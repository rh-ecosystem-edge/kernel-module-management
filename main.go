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
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build/buildconfig"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/daemonset"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/metrics"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/preflight"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/rbac"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/statusupdater"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2/klogr"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	//+kubebuilder:scaffold:imports
)

const (
	NFDKernelLabelingMethod  = "nfd"
	KMMOKernelLabelingMethod = "kmmo"

	KernelLabelingMethodEnvVar = "KERNEL_LABELING_METHOD"
)

var (
	scheme               = runtime.NewScheme()
	validLabelingMethods = sets.NewString(KMMOKernelLabelingMethod, NFDKernelLabelingMethod)
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(kmmv1beta1.AddToScheme(scheme))
	utilruntime.Must(buildv1.Install(scheme))
	utilruntime.Must(imagev1.Install(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		configFile           string
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	flag.StringVar(&configFile, "config", "", "The path to the configuration file.")

	klog.InitFlags(flag.CommandLine)

	flag.Parse()

	logger := klogr.New()

	ctrl.SetLogger(logger)

	setupLogger := logger.WithName("setup")

	commit, err := gitCommit()
	if err != nil {
		setupLogger.Error(err, "Could not get the git commit; using <undefined>")
		commit = "<undefined>"
	}

	setupLogger.Info("Creating manager", "git commit", commit)

	restConfig := ctrl.GetConfigOrDie()

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "c5baf8af.sigs.k8s.io",
	})
	if err != nil {
		setupLogger.Error(err, "unable to start manager")
		os.Exit(1)
	}

	client := mgr.GetClient()

	var (
		kernelLabel          string
		kernelLabelingMethod = GetEnvWithDefault(KernelLabelingMethodEnvVar, KMMOKernelLabelingMethod)
	)

	setupLogger.V(1).Info("Determining kernel labeling method", KernelLabelingMethodEnvVar, kernelLabelingMethod)

	filter := filter.New(client, mgr.GetLogger())
	kernelOsDtkMapping := syncronizedmap.NewKernelOsDtkMapping()

	switch kernelLabelingMethod {
	case KMMOKernelLabelingMethod:
		kernelLabel = "kmm.node.kubernetes.io/kernel-version.full"

		nodeKernelReconciler := controllers.NewNodeKernelReconciler(client, kernelLabel, filter, kernelOsDtkMapping)

		if err = nodeKernelReconciler.SetupWithManager(mgr); err != nil {
			setupLogger.Error(err, "unable to create controller", "controller", "NodeKernel")
			os.Exit(1)
		}
	case NFDKernelLabelingMethod:
		kernelLabel = "feature.node.kubernetes.io/kernel-version.full"
	default:
		setupLogger.Error(
			fmt.Errorf("%q is not in %v", kernelLabelingMethod, validLabelingMethods.List()),
			"Invalid kernel labeling method",
		)

		os.Exit(1)
	}

	setupLogger.V(1).Info("Using kernel label", "label", kernelLabel)

	metricsAPI := metrics.New()
	metricsAPI.Register()
	helperAPI := build.NewHelper()
	buildAPI := buildconfig.NewManager(
		client,
		buildconfig.NewMaker(helperAPI, scheme),
		buildconfig.NewOpenShiftBuildsHelper(client),
	)
	rbacAPI := rbac.NewCreator(client, scheme)
	daemonAPI := daemonset.NewCreator(client, kernelLabel, scheme)
	kernelAPI := module.NewKernelMapper()
	moduleStatusUpdaterAPI := statusupdater.NewModuleStatusUpdater(client, daemonAPI, metricsAPI)
	preflightStatusUpdaterAPI := statusupdater.NewPreflightStatusUpdater(client)
	registryAPI := registry.NewRegistry()
	authFactory := auth.NewRegistryAuthGetterFactory(client, kubernetes.NewForConfigOrDie(restConfig))
	preflightAPI := preflight.NewPreflightAPI(client, registryAPI, kernelAPI, authFactory)

	mc := controllers.NewModuleReconciler(
		client,
		buildAPI,
		rbacAPI,
		daemonAPI,
		kernelAPI,
		metricsAPI,
		filter,
		registryAPI,
		authFactory,
		moduleStatusUpdaterAPI,
		kernelOsDtkMapping,
	)

	if err = mc.SetupWithManager(mgr, kernelLabel); err != nil {
		setupLogger.Error(err, "unable to create controller", "controller", "Module")
		os.Exit(1)
	}

	if err = controllers.NewPodNodeModuleReconciler(client, daemonAPI).SetupWithManager(mgr); err != nil {
		setupLogger.Error(err, "unable to create controller", "controller", "PodNodeModule")
		os.Exit(1)
	}

	if err = controllers.NewPreflightValidationReconciler(client, filter, preflightStatusUpdaterAPI, preflightAPI).SetupWithManager(mgr); err != nil {
		setupLogger.Error(err, "unable to create controller", "controller", "Preflight")
		os.Exit(1)
	}

	if err = controllers.NewImageStreamReconciler(client, filter, kernelOsDtkMapping).SetupWithManager(mgr); err != nil {
		setupLogger.Error(err, "unable to create controller", "controller", "ImageStream")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLogger.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLogger.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLogger.Info("starting manager")
	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLogger.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func GetEnvWithDefault(key, def string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		v = def
	}

	return v
}

func gitCommit() (string, error) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", errors.New("build info is not available")
	}

	const vcsRevisionKey = "vcs.revision"

	for _, s := range bi.Settings {
		if s.Key == vcsRevisionKey {
			return s.Value, nil
		}
	}

	return "", fmt.Errorf("%s not found in build info settings", vcsRevisionKey)
}
