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
	"fmt"
	"os"
	"strconv"

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/ca"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/controllers"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build/buildconfig"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/cmd"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/daemonset"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/metrics"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/preflight"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/rbac"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/sign"
	signjob "github.com/rh-ecosystem-edge/kernel-module-management/internal/sign/job"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/statusupdater"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils"
	//+kubebuilder:scaffold:imports
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kmmv1beta1.AddToScheme(scheme))
	utilruntime.Must(buildv1.Install(scheme))
	utilruntime.Must(imagev1.Install(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	logger := klogr.New().WithName("kmm")

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

	managed, err := GetBoolEnv("KMM_MANAGED")
	if err != nil {
		setupLogger.Error(err, "could not determine if we are running as managed; disabling")
		managed = false
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

	client := mgr.GetClient()

	filterAPI := filter.New(client, mgr.GetLogger())
	kernelOsDtkMapping := syncronizedmap.NewKernelOsDtkMapping()

	metricsAPI := metrics.New()
	metricsAPI.Register()
	helperAPI := build.NewHelper()
	registryAPI := registry.NewRegistry()
	authFactory := auth.NewRegistryAuthGetterFactory(
		client,
		kubernetes.NewForConfigOrDie(
			ctrl.GetConfigOrDie(),
		),
	)

	buildAPI := buildconfig.NewManager(
		client,
		buildconfig.NewMaker(client, helperAPI, scheme, kernelOsDtkMapping),
		buildconfig.NewOpenShiftBuildsHelper(client),
		authFactory,
		registryAPI,
	)

	jobHelperAPI := utils.NewJobHelper(client)
	caHelper := ca.NewHelper(client, scheme)

	signAPI := signjob.NewSignJobManager(
		signjob.NewSigner(client, scheme, sign.NewSignerHelper(), jobHelperAPI, caHelper),
		jobHelperAPI,
		authFactory,
		registryAPI,
	)

	daemonAPI := daemonset.NewCreator(client, constants.KernelLabel, scheme)
	kernelAPI := module.NewKernelMapper()

	mc := controllers.NewModuleReconciler(
		client,
		buildAPI,
		signAPI,
		rbac.NewCreator(client, scheme),
		daemonAPI,
		kernelAPI,
		metricsAPI,
		filterAPI,
		statusupdater.NewModuleStatusUpdater(client, metricsAPI),
		caHelper,
	)

	if err = mc.SetupWithManager(mgr, constants.KernelLabel); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.ModuleReconcilerName)
	}

	nodeKernelReconciler := controllers.NewNodeKernelReconciler(client, constants.KernelLabel, filterAPI, kernelOsDtkMapping)

	if err = nodeKernelReconciler.SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.NodeKernelReconcilerName)
	}

	if err = controllers.NewPodNodeModuleReconciler(client, daemonAPI).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.PodNodeModuleReconcilerName)
	}

	preflightStatusUpdaterAPI := statusupdater.NewPreflightStatusUpdater(client)
	preflightOCPStatusUpdaterAPI := statusupdater.NewPreflightOCPStatusUpdater(client)
	preflightAPI := preflight.NewPreflightAPI(client, buildAPI, signAPI, registryAPI, kernelAPI, preflightStatusUpdaterAPI, authFactory)

	if err = controllers.NewPreflightValidationReconciler(client, filterAPI, preflightStatusUpdaterAPI, preflightAPI).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.PreflightValidationReconcilerName)
	}

	if managed {
		setupLogger.Info("Starting as managed")

		if err = clusterv1alpha1.Install(scheme); err != nil {
			cmd.FatalError(setupLogger, err, "could not add the Cluster API to the scheme")
		}

		if err = controllers.NewNodeKernelClusterClaimReconciler(client).SetupWithManager(mgr); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.NodeKernelClusterClaimReconcilerName)
		}
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

	if err = controllers.NewPreflightValidationOCPReconciler(client,
		filterAPI,
		registryAPI,
		authFactory,
		kernelOsDtkMapping,
		preflightOCPStatusUpdaterAPI,
		scheme).SetupWithManager(mgr); err != nil {
		setupLogger.Error(err, "unable to create controller", "controller", "PreflightOCP")
		os.Exit(1)
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

func GetBoolEnv(s string) (bool, error) {
	envValue := os.Getenv(s)

	if envValue == "" {
		return false, nil
	}

	managed, err := strconv.ParseBool(envValue)
	if err != nil {
		return false, fmt.Errorf("%q: invalid value for %s", envValue, s)
	}

	return managed, nil
}
