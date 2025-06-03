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

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/buildsign"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/mbsc"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/mic"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/node"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/pod"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/preflight"

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta2"
	buildsignresource "github.com/rh-ecosystem-edge/kernel-module-management/internal/buildsign/resource"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/cmd"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/config"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/controllers"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/metrics"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/nmc"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/ocp/ca"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/textlogger"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
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
	utilruntime.Must(v1beta2.AddToScheme(scheme))
	utilruntime.Must(buildv1.Install(scheme))
	utilruntime.Must(imagev1.Install(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	logConfig := textlogger.NewConfig()
	logConfig.AddFlags(flag.CommandLine)

	var userConfigMapName string

	flag.StringVar(&userConfigMapName, "config", "", "Name of the ConfigMap containing user config.")

	flag.Parse()

	logger := textlogger.NewLogger(logConfig).WithName("kmm")

	ctrl.SetLogger(logger)

	setupLogger := logger.WithName("setup")
	operatorNamespace := cmd.GetEnvOrFatalError(constants.OperatorNamespaceEnvVar, setupLogger)

	setupLogger.Info("Creating manager", "version", Version, "git commit", GitCommit)

	workerImage := cmd.GetEnvOrFatalError("RELATED_IMAGE_WORKER", setupLogger)

	managed, err := GetBoolEnv("KMM_MANAGED")
	if err != nil {
		setupLogger.Error(err, "could not determine if we are running as managed; disabling")
		managed = false
	}

	ctx := ctrl.SetupSignalHandler()
	cg := config.NewConfigGetter(setupLogger)

	cfg, err := cg.GetConfig(ctx, userConfigMapName, operatorNamespace, false)
	if err != nil {
		cmd.FatalError(setupLogger, err, "failed to get kmm config")
	}

	options := cg.GetManagerOptionsFromConfig(cfg, scheme)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		cmd.FatalError(setupLogger, err, "unable to create manager")
	}

	client := mgr.GetClient()

	nmcHelper := nmc.NewHelper(client)
	filterAPI := filter.New(client, nmcHelper)
	kernelOsDtkMapping := syncronizedmap.NewKernelOsDtkMapping()

	metricsAPI := metrics.New()
	metricsAPI.Register()

	buildArgOverriderAPI := module.NewBuildArgOverrider()
	resourceManager := buildsignresource.NewResourceManager(client, buildArgOverriderAPI, kernelOsDtkMapping, scheme)
	nodeAPI := node.NewNode(client)
	kernelAPI := module.NewKernelMapper(buildArgOverriderAPI)
	micAPI := mic.New(client, scheme)
	mbscAPI := mbsc.New(client, scheme)
	imagePullerAPI := pod.NewImagePuller(client, scheme)

	dpc := controllers.NewDevicePluginReconciler(
		client,
		metricsAPI,
		filterAPI,
		nodeAPI,
		scheme,
		operatorNamespace)
	if err = dpc.SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.DevicePluginReconcilerName)
	}

	caHelper := ca.NewHelper(client, scheme)

	if err = controllers.NewModuleCAReconciler(client, caHelper, operatorNamespace).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.ModuleCAReconcilerName)
	}

	mnc := controllers.NewModuleReconciler(
		client,
		kernelAPI,
		micAPI,
		nmcHelper,
		filterAPI,
		nodeAPI,
		operatorNamespace,
		scheme,
	)
	if err = mnc.SetupWithManager(mgr, !managed); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.ModuleReconcilerName)
	}

	eventRecorder := mgr.GetEventRecorderFor("kmm")

	workerPodManagerAPI := pod.NewWorkerPodManager(client, workerImage, scheme, &cfg.Worker)
	if err = controllers.NewNMCReconciler(client, scheme, workerImage, caHelper, &cfg.Worker, eventRecorder, nodeAPI,
		workerPodManagerAPI).SetupWithManager(ctx, mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.NodeModulesConfigReconcilerName)
	}

	nodeKernelReconciler := controllers.NewKernelDTKReconciler(client, kernelOsDtkMapping)

	if err = nodeKernelReconciler.SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.KernelDTKReconcilerName)
	}

	if err = controllers.NewDevicePluginPodReconciler(client).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.DevicePluginPodReconcilerName)
	}

	if err = controllers.NewNodeLabelModuleVersionReconciler(client).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.NodeLabelModuleVersionReconcilerName)
	}

	if err = controllers.NewMICReconciler(client, micAPI, mbscAPI, imagePullerAPI, scheme).SetupWithManager(mgr); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.MICReconcilerName)
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
		builSignAPI := buildsign.NewManager(client, resourceManager, scheme)

		mbscr := controllers.NewMBSCReconciler(client, builSignAPI, mbscAPI)
		if err = mbscr.SetupWithManager(mgr); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.MBSCReconcilerName)
		}

		helper := controllers.NewJobEventReconcilerHelper(client)

		if err = controllers.NewBuildSignEventsReconciler(client, helper, eventRecorder).SetupWithManager(mgr); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.BuildSignEventsReconcilerName)
		}

		if err = controllers.NewJobGCReconciler(client, cfg.Job.GCDelay).SetupWithManager(mgr); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.JobGCReconcilerName)
		}

		preflightAPI := preflight.NewPreflightAPI()

		if err = controllers.NewPreflightValidationReconciler(client, filterAPI, metricsAPI, micAPI, kernelAPI, preflightAPI).SetupWithManager(mgr); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create controller", "name", controllers.PreflightValidationReconcilerName)
		}
	}

	dtkNSN := types.NamespacedName{
		Namespace: constants.DTKImageStreamNamespace,
		Name:      "driver-toolkit",
	}

	dtkClient := ctrlclient.NewNamespacedClient(client, constants.DTKImageStreamNamespace)

	if err = controllers.NewImageStreamReconciler(dtkClient, kernelOsDtkMapping, dtkNSN).SetupWithManager(mgr, filterAPI); err != nil {
		cmd.FatalError(setupLogger, err, "unable to create controller", "controller", controllers.ImageStreamReconcilerName)
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
