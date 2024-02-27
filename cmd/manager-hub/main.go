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
	"time"

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/config"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/textlogger"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/rh-ecosystem-edge/kernel-module-management/api-hub/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	buildocpbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build/ocpbuild"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/cache"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/cluster"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/cmd"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/controllers"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/controllers/hub"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/manifestwork"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/metrics"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/nmc"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/sign"
	signocpbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/sign/ocpbuild"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/statusupdater"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	ocpbuildutils "github.com/rh-ecosystem-edge/kernel-module-management/internal/utils/ocpbuild"
	//+kubebuilder:scaffold:imports
)

var (
	GitCommit = "undefined"
	Version   = "undefined"

	scheme = runtime.NewScheme()
)

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
	logConfig := textlogger.NewConfig()
	logConfig.AddFlags(flag.CommandLine)

	var configFile string

	flag.StringVar(&configFile, "config", "", "The path to the configuration file.")

	flag.Parse()

	logger := textlogger.NewLogger(logConfig).WithName("kmm-hub")

	ctrl.SetLogger(logger)

	setupLogger := logger.WithName("setup")

	setupLogger.Info("Creating manager", "version", Version, "git commit", GitCommit)
	setupLogger.Info("Parsing configuration file", "path", configFile)

	cfg, err := config.ParseFile(configFile)
	if err != nil {
		cmd.FatalError(setupLogger, err, "could not parse the configuration file", "path", configFile)
	}

	options := cfg.ManagerOptions(setupLogger)
	options.Scheme = scheme

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), *options)
	if err != nil {
		cmd.FatalError(setupLogger, err, "unable to create manager")
	}

	operatorNamespace := cmd.GetEnvOrFatalError(constants.OperatorNamespaceEnvVar, setupLogger)

	client := mgr.GetClient()

	nmcHelper := nmc.NewHelper(client)
	filterAPI := filter.New(client, nmcHelper)
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
		ocpbuildutils.NewOCPBuildsHelper(client, buildocpbuild.BuildType),
		authFactory,
		registryAPI,
	)

	signAPI := signocpbuild.NewManager(
		client,
		signocpbuild.NewMaker(client, cmd.GetEnvOrFatalError("RELATED_IMAGE_SIGN", setupLogger), scheme),
		ocpbuildutils.NewOCPBuildsHelper(client, signocpbuild.BuildType),
		authFactory,
		registryAPI,
	)

	kernelAPI := module.NewKernelMapper(buildHelperAPI, sign.NewSignerHelper())

	ctrlLogger := setupLogger.WithValues("name", hub.ManagedClusterModuleReconcilerName)
	ctrlLogger.Info("Adding controller")

	cache := cache.New[string](10 * time.Minute)
	ctx := ctrl.SetupSignalHandler()
	cache.StartCollecting(ctx, 10*time.Minute)

	mcmr := hub.NewManagedClusterModuleReconciler(
		client,
		manifestwork.NewCreator(client, scheme, kernelAPI, registryAPI, authFactory, cache, operatorNamespace),
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

	if err = controllers.NewKernelDTKReconciler(client, kernelOsDtkMapping).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.KernelDTKReconcilerName)
	}

	eventRecorder := mgr.GetEventRecorderFor("kmm-hub")
	jobEventReconcilerHelper := controllers.NewJobEventReconcilerHelper(client)

	if err = controllers.NewBuildSignEventsReconciler(client, jobEventReconcilerHelper, eventRecorder).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.BuildSignEventsReconcilerName)
	}

	//+kubebuilder:scaffold:builder

	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		cmd.FatalError(setupLogger, err, "unable to set up health check")
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		cmd.FatalError(setupLogger, err, "unable to set up ready check")
	}

	setupLogger.Info("starting manager")
	if err = mgr.Start(ctx); err != nil {
		cmd.FatalError(setupLogger, err, "problem running manager")
	}

	cache.WaitForTermination()
}
