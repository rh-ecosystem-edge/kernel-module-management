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

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/ocp/ca"
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

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	buildocpbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build/ocpbuild"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/cmd"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/config"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/controllers"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/daemonset"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/metrics"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/nmc"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/preflight"
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

	setupLogger.Info("Creating manager", "version", Version, "git commit", GitCommit)

	operatorNamespace := cmd.GetEnvOrFatalError(constants.OperatorNamespaceEnvVar, setupLogger)
	workerImage := cmd.GetEnvOrFatalError("RELATED_IMAGES_WORKER", setupLogger)

	managed, err := GetBoolEnv("KMM_MANAGED")
	if err != nil {
		setupLogger.Error(err, "could not determine if we are running as managed; disabling")
		managed = false
	}

	setupLogger.Info("Parsing configuration file", "path", configFile)

	cfg, err := config.ParseFile(configFile)
	if err != nil {
		cmd.FatalError(setupLogger, err, "could not parse the configuration file", "path", configFile)
	}

	options := cfg.ManagerOptions()
	options.Scheme = scheme

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), *options)
	if err != nil {
		cmd.FatalError(setupLogger, err, "unable to create manager")
	}

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
		signocpbuild.NewMaker(client, cmd.GetEnvOrFatalError("RELATED_IMAGES_SIGN", setupLogger), scheme),
		ocpbuildutils.NewOCPBuildsHelper(client, signocpbuild.BuildType),
		authFactory,
		registryAPI,
	)

	daemonAPI := daemonset.NewCreator(client, constants.KernelLabel, scheme)
	kernelAPI := module.NewKernelMapper(buildHelperAPI, sign.NewSignerHelper())

	mc := controllers.NewModuleReconciler(
		client,
		buildAPI,
		signAPI,
		daemonAPI,
		kernelAPI,
		metricsAPI,
		filterAPI,
		statusupdater.NewModuleStatusUpdater(client),
		operatorNamespace,
	)
	if err = mc.SetupWithManager(mgr, constants.KernelLabel); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.ModuleReconcilerName)
	}

	caHelper := ca.NewHelper(client, scheme)

	if err = controllers.NewModuleCAReconciler(client, caHelper, operatorNamespace).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.ModuleCAReconcilerName)
	}

	mnc := controllers.NewModuleNMCReconciler(
		client,
		kernelAPI,
		registryAPI,
		nmcHelper,
		filterAPI,
		authFactory,
		scheme,
	)
	if err = mnc.SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.ModuleNMCReconcilerName)
	}

	ctx := ctrl.SetupSignalHandler()

	if err = controllers.NewNMCReconciler(client, scheme, workerImage, caHelper, &cfg.Worker).SetupWithManager(ctx, mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.NodeModulesConfigReconcilerName)
	}

	nodeKernelReconciler := controllers.NewKernelDTKReconciler(client, kernelOsDtkMapping)

	if err = nodeKernelReconciler.SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.KernelDTKReconcilerName)
	}

	if err = controllers.NewPodNodeModuleReconciler(client, daemonAPI).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.PodNodeModuleReconcilerName)
	}

	if err = controllers.NewNodeLabelModuleVersionReconciler(client).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.NodeLabelModuleVersionReconcilerName)
	}

	preflightStatusUpdaterAPI := statusupdater.NewPreflightStatusUpdater(client)
	preflightOCPStatusUpdaterAPI := statusupdater.NewPreflightOCPStatusUpdater(client)
	preflightAPI := preflight.NewPreflightAPI(client, buildAPI, signAPI, registryAPI, kernelAPI, preflightStatusUpdaterAPI, authFactory)

	if err = controllers.NewPreflightValidationReconciler(client, filterAPI, metricsAPI, preflightStatusUpdaterAPI, preflightAPI).SetupWithManager(mgr); err != nil {
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
	} else {
		eventRecorder := mgr.GetEventRecorderFor("kmm-hub")

		if err = controllers.NewBuildSignEventsReconciler(client, eventRecorder).SetupWithManager(mgr); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.BuildSignEventsReconcilerName)
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

	if err = (&kmmv1beta1.Module{}).SetupWebhookWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create webhook", "webhook", "Module")
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
