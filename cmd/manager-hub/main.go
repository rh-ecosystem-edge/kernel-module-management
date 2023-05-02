/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	buildocpbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build/ocpbuild"
	signocpbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/sign/ocpbuild"
	buildutils "github.com/rh-ecosystem-edge/kernel-module-management/internal/utils/build"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/rh-ecosystem-edge/kernel-module-management/api-hub/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/controllers"
	"github.com/rh-ecosystem-edge/kernel-module-management/controllers/hub"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/cluster"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/cmd"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/manifestwork"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/metrics"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/sign"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/statusupdater"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	//+kubebuilder:scaffold:imports
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1beta1.AddToScheme(scheme))
	utilruntime.Must(buildv1.Install(scheme))
	utilruntime.Must(clusterv1.Install(scheme))
	utilruntime.Must(imagev1.Install(scheme))
	utilruntime.Must(workv1.Install(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	logger := klogr.New().WithName("kmm-hub")

	ctrl.SetLogger(logger)

	setupLogger := logger.WithName("setup")

	var configFile string

	flag.StringVar(&configFile, "config", "", "The path to the configuration file.")

	klog.InitFlags(flag.CommandLine)

	flag.Parse()

	commit, err := cmd.GitCommit()
	if err != nil {
		setupLogger.Error(err, "Could not get the git commit; using <undefined>")
		commit = "<undefined>"
	}

	setupLogger.Info("Creating manager", "git commit", commit)

	options := ctrl.Options{Scheme: scheme}

	options, err = options.AndFrom(ctrl.ConfigFile().AtPath(configFile))
	if err != nil {
		cmd.FatalError(setupLogger, err, "unable to load the config file")
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		cmd.FatalError(setupLogger, err, "unable to create manager")
	}

	operatorNamespace := cmd.GetEnvOrFatalError(constants.OperatorNamespaceEnvVar, setupLogger)

	client := mgr.GetClient()

	filterAPI := filter.New(client, mgr.GetLogger())
	kernelOsDtkMapping := syncronizedmap.NewKernelOsDtkMapping()

	metricsAPI := metrics.New()
	metricsAPI.Register()

	buildHelperAPI := build.NewHelper()
	registryAPI := registry.NewRegistry()

	authFactory := auth.NewRegistryAuthGetterFactory(
		client,
		kubernetes.NewForConfigOrDie(
			ctrl.GetConfigOrDie(),
		),
	)

	buildAPI := buildocpbuild.NewManager(
		client,
		buildocpbuild.NewMaker(client, buildHelperAPI, scheme, kernelOsDtkMapping),
		buildutils.NewOpenShiftBuildsHelper(client, buildocpbuild.BuildType),
		authFactory,
		registryAPI,
	)

	signAPI := signocpbuild.NewManager(
		client,
		signocpbuild.NewMaker(client, cmd.GetEnvOrFatalError("RELATED_IMAGES_SIGN", setupLogger), scheme),
		buildutils.NewOpenShiftBuildsHelper(client, signocpbuild.BuildType),
		authFactory,
		registryAPI,
	)

	ctrlLogger := setupLogger.WithValues("name", hub.ManagedClusterModuleReconcilerName)
	ctrlLogger.Info("Adding controller")

	mcmr := hub.NewManagedClusterModuleReconciler(
		client,
		manifestwork.NewCreator(client, scheme),
		cluster.NewClusterAPI(client, module.NewKernelMapper(buildHelperAPI, sign.NewSignerHelper()), buildAPI, signAPI, operatorNamespace),
		statusupdater.NewManagedClusterModuleStatusUpdater(client),
		filterAPI,
		operatorNamespace,
	)

	if err = mcmr.SetupWithManager(mgr); err != nil {
		cmd.FatalError(ctrlLogger, err, "unable to create controller")
	}

	dtkNSN := types.NamespacedName{
		Namespace: constants.DTKImageStreamNamespace,
		Name:      "driver-toolkit",
	}

	dtkClient := ctrlclient.NewNamespacedClient(client, constants.DTKImageStreamNamespace)

	if err = controllers.NewImageStreamReconciler(dtkClient, kernelOsDtkMapping, dtkNSN).SetupWithManager(mgr, filterAPI); err != nil {
		setupLogger.Error(err, "unable to create controller", "controller", "ImageStream")
		os.Exit(1)
	}

	if err = (&v1beta1.ManagedClusterModule{}).SetupWebhookWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create webhook", "webhook", "ManagedClusterModule")
	}

	//+kubebuilder:scaffold:builder

	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		cmd.FatalError(setupLogger, err, "unable to set up health check")
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		cmd.FatalError(setupLogger, err, "unable to set up ready check")
	}

	setupLogger.Info("starting manager")
	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		cmd.FatalError(setupLogger, err, "problem running manager")
	}
}
