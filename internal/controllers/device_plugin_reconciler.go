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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/metrics"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/node"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	DevicePluginReconcilerName     = "DevicePluginReconciler"
	kubeletDevicePluginsVolumeName = "kubelet-device-plugins"
	kubeletDevicePluginsPath       = "/var/lib/kubelet/device-plugins"
)

// ModuleReconciler reconciles a Module object
type DevicePluginReconciler struct {
	client         client.Client
	filter         *filter.Filter
	reconHelperAPI devicePluginReconcilerHelperAPI
}

func NewDevicePluginReconciler(
	client client.Client,
	metricsAPI metrics.Metrics,
	filter *filter.Filter,
	nodeAPI node.Node,
	scheme *runtime.Scheme,
	operatorNamespace string,
) *DevicePluginReconciler {
	reconHelperAPI := newDevicePluginReconcilerHelper(client, metricsAPI, nodeAPI, scheme, operatorNamespace)
	return &DevicePluginReconciler{
		client:         client,
		reconHelperAPI: reconHelperAPI,
		filter:         filter,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DevicePluginReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kmmv1beta1.Module{}).
		Owns(&appsv1.DaemonSet{}).
		Named(DevicePluginReconcilerName).
		Complete(
			reconcile.AsReconciler[*kmmv1beta1.Module](r.client, r),
		)
}

func (r *DevicePluginReconciler) Reconcile(ctx context.Context, mod *kmmv1beta1.Module) (ctrl.Result, error) {
	res := ctrl.Result{}

	logger := log.FromContext(ctx)

	existingDevicePluginDS, err := r.reconHelperAPI.getModuleDevicePluginDaemonSets(ctx, mod.Name, mod.Namespace)
	if err != nil {
		return res, fmt.Errorf("could not get DaemonSets for module %s, namespace %s: %v", mod.Name, mod.Namespace, err)
	}

	if mod.GetDeletionTimestamp() != nil {
		err = r.reconHelperAPI.handleModuleDeletion(ctx, existingDevicePluginDS)
		return ctrl.Result{}, err
	}

	r.reconHelperAPI.setKMMOMetrics(ctx)

	err = r.reconHelperAPI.handleDevicePlugin(ctx, mod, existingDevicePluginDS)
	if err != nil {
		return res, fmt.Errorf("could handle device plugin: %w", err)
	}

	logger.Info("Run garbage collection")
	err = r.reconHelperAPI.garbageCollect(ctx, mod, existingDevicePluginDS)
	if err != nil {
		return res, fmt.Errorf("failed to run garbage collection: %v", err)
	}

	err = r.reconHelperAPI.moduleUpdateDevicePluginStatus(ctx, mod, existingDevicePluginDS)
	if err != nil {
		return res, fmt.Errorf("failed to update device-plugin status of the module: %w", err)
	}

	logger.Info("Reconcile loop finished successfully")

	return res, nil
}

//go:generate mockgen -source=device_plugin_reconciler.go -package=controllers -destination=mock_device_plugin_reconciler.go devicePluginReconcilerHelperAPI

type devicePluginReconcilerHelperAPI interface {
	setKMMOMetrics(ctx context.Context)
	handleDevicePlugin(ctx context.Context, mod *kmmv1beta1.Module, existingDevicePluginDS []appsv1.DaemonSet) error
	garbageCollect(ctx context.Context, mod *kmmv1beta1.Module, existingDS []appsv1.DaemonSet) error
	handleModuleDeletion(ctx context.Context, existingDevicePluginDS []appsv1.DaemonSet) error
	moduleUpdateDevicePluginStatus(ctx context.Context, mod *kmmv1beta1.Module, existingDevicePluginDS []appsv1.DaemonSet) error
	getModuleDevicePluginDaemonSets(ctx context.Context, name, namespace string) ([]appsv1.DaemonSet, error)
}

type devicePluginReconcilerHelper struct {
	client          client.Client
	metricsAPI      metrics.Metrics
	daemonSetHelper daemonSetCreator
	nodeAPI         node.Node
}

func newDevicePluginReconcilerHelper(client client.Client,
	metricsAPI metrics.Metrics,
	nodeAPI node.Node,
	scheme *runtime.Scheme,
	operatorNamespace string) devicePluginReconcilerHelperAPI {
	daemonSetHelper := newDaemonSetCreator(scheme, operatorNamespace)
	return &devicePluginReconcilerHelper{
		client:          client,
		metricsAPI:      metricsAPI,
		daemonSetHelper: daemonSetHelper,
		nodeAPI:         nodeAPI,
	}
}

func (dprh *devicePluginReconcilerHelper) getModuleDevicePluginDaemonSets(ctx context.Context, name, namespace string) ([]appsv1.DaemonSet, error) {
	dsList := appsv1.DaemonSetList{}
	opts := []client.ListOption{
		client.MatchingLabels(map[string]string{constants.ModuleNameLabel: name}),
		client.InNamespace(namespace),
	}
	if err := dprh.client.List(ctx, &dsList, opts...); err != nil {
		return nil, fmt.Errorf("could not list DaemonSets: %v", err)
	}

	devicePluginsList := make([]appsv1.DaemonSet, 0, len(dsList.Items))
	// remove the older version module loader daemonsets
	for _, ds := range dsList.Items {
		if _, ok := ds.GetLabels()[constants.KernelLabel]; !ok {
			devicePluginsList = append(devicePluginsList, ds)
		}
	}

	return devicePluginsList, nil
}

func (dprh *devicePluginReconcilerHelper) handleDevicePlugin(ctx context.Context, mod *kmmv1beta1.Module, existingDevicePluginDS []appsv1.DaemonSet) error {
	if mod.Spec.DevicePlugin == nil {
		return nil
	}

	logger := log.FromContext(ctx)

	ds, version := getExistingDSFromVersion(existingDevicePluginDS, mod.Namespace, mod.Name, mod.Spec.ModuleLoader)
	if ds == nil {
		logger.Info("creating new device plugin DS", "version", version)
		ds = &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Namespace: mod.Namespace, GenerateName: mod.Name + "-device-plugin-"},
		}
	}
	opRes, err := controllerutil.CreateOrPatch(ctx, dprh.client, ds, func() error {
		return dprh.daemonSetHelper.setDevicePluginAsDesired(ctx, ds, mod)
	})

	if err == nil {
		logger.Info("Reconciled Device Plugin", "name", ds.Name, "result", opRes)
	}

	return err
}

func (dprh *devicePluginReconcilerHelper) garbageCollect(ctx context.Context,
	mod *kmmv1beta1.Module,
	existingDS []appsv1.DaemonSet) error {
	if mod.Spec.ModuleLoader == nil {
		// If ModuleLoader is not exist, Ordered Upgrade is not relevant
		// so there is no need to delete older-versioned Deamonsets
		return nil
	}

	logger := log.FromContext(ctx)
	deleted := make([]string, 0)
	for _, ds := range existingDS {
		if isOlderVersionUnusedDevicePluginDaemonset(&ds, mod.Spec.ModuleLoader.Container.Version) {
			deleted = append(deleted, ds.Name)
			if err := dprh.client.Delete(ctx, &ds); err != nil {
				return fmt.Errorf("could not delete device plugin DaemonSet %s: %v", ds.Name, err)
			}
		}
	}

	logger.Info("garbage-collected device plugin's DaemonSets", "names", deleted)
	return nil
}

func (dprh *devicePluginReconcilerHelper) handleModuleDeletion(ctx context.Context, existingDevicePluginDS []appsv1.DaemonSet) error {
	// delete all the Device Plugin Daemonset, in order to allow worker pods to delete kernel modules
	for _, ds := range existingDevicePluginDS {
		err := dprh.client.Delete(ctx, &ds)
		if err != nil {
			return fmt.Errorf("failed to delete device-plugin Daemonset %s/%s: %v", ds.Namespace, ds.Name, err)
		}
	}
	return nil
}

func (dprh *devicePluginReconcilerHelper) setKMMOMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)

	mods := kmmv1beta1.ModuleList{}
	err := dprh.client.List(ctx, &mods)
	if err != nil {
		logger.V(1).Info("failed to list KMMomodules for metrics", "error", err)
		return
	}

	numModules := len(mods.Items)
	numModulesWithBuild := 0
	numModulesWithSign := 0
	numModulesWithDevicePlugin := 0
	for _, mod := range mods.Items {
		if mod.Spec.DevicePlugin != nil {
			numModulesWithDevicePlugin += 1
		}
		if mod.Spec.ModuleLoader != nil {
			buildCapable, signCapable := isModuleBuildAndSignCapable(&mod)
			if buildCapable {
				numModulesWithBuild += 1
			}
			if signCapable {
				numModulesWithSign += 1
			}
			if mod.Spec.ModuleLoader.Container.Modprobe.Args != nil {
				modprobeArgs := strings.Join(mod.Spec.ModuleLoader.Container.Modprobe.Args.Load, ",")
				dprh.metricsAPI.SetKMMModprobeArgs(mod.Name, mod.Namespace, modprobeArgs)
			}
			if mod.Spec.ModuleLoader.Container.Modprobe.RawArgs != nil {
				modprobeRawArgs := strings.Join(mod.Spec.ModuleLoader.Container.Modprobe.RawArgs.Load, ",")
				dprh.metricsAPI.SetKMMModprobeRawArgs(mod.Name, mod.Namespace, modprobeRawArgs)
			}
		}
	}
	dprh.metricsAPI.SetKMMModulesNum(numModules)
	dprh.metricsAPI.SetKMMInClusterBuildNum(numModulesWithBuild)
	dprh.metricsAPI.SetKMMInClusterSignNum(numModulesWithSign)
	dprh.metricsAPI.SetKMMDevicePluginNum(numModulesWithDevicePlugin)
}

func (dprh *devicePluginReconcilerHelper) moduleUpdateDevicePluginStatus(ctx context.Context,
	mod *kmmv1beta1.Module,
	existingDevicePluginDS []appsv1.DaemonSet) error {

	if mod.Spec.DevicePlugin == nil {
		return nil
	}

	// get the number of nodes targeted by selector (which also relevant for device plugin)
	numTargetedNodes, err := dprh.nodeAPI.GetNumTargetedNodes(ctx, mod.Spec.Selector, mod.Spec.Tolerations)
	if err != nil {
		return fmt.Errorf("failed to determine the number of nodes that should be targeted by Module's %s/%s selector: %v", mod.Namespace, mod.Name, err)
	}

	// number of available consists of sum of available pods for both (in case of ordered upgrade)
	// device plugin DaemonSets
	numAvailable := int32(0)
	for _, ds := range existingDevicePluginDS {
		numAvailable += ds.Status.NumberAvailable
	}

	unmodifiedMod := mod.DeepCopy()

	mod.Status.DevicePlugin.NodesMatchingSelectorNumber = int32(numTargetedNodes)
	mod.Status.DevicePlugin.DesiredNumber = int32(numTargetedNodes)
	mod.Status.DevicePlugin.AvailableNumber = numAvailable

	return dprh.client.Status().Patch(ctx, mod, client.MergeFrom(unmodifiedMod))
}

//go:generate mockgen -source=device_plugin_reconciler.go -package=controllers -destination=mock_device_plugin_reconciler.go daemonSetCreator

type daemonSetCreator interface {
	setDevicePluginAsDesired(ctx context.Context, ds *appsv1.DaemonSet, mod *kmmv1beta1.Module) error
}

type daemonSetCreatorImpl struct {
	scheme            *runtime.Scheme
	operatorNamespace string
}

func newDaemonSetCreator(scheme *runtime.Scheme, operatorNamespace string) daemonSetCreator {
	return &daemonSetCreatorImpl{
		scheme:            scheme,
		operatorNamespace: operatorNamespace,
	}
}

func (dsci *daemonSetCreatorImpl) setDevicePluginAsDesired(
	ctx context.Context,
	ds *appsv1.DaemonSet,
	mod *kmmv1beta1.Module,
) error {
	if ds == nil {
		return errors.New("ds cannot be nil")
	}

	if mod.Spec.DevicePlugin == nil {
		return errors.New("device plugin in module should not be nil")
	}

	containerVolumeMounts := []v1.VolumeMount{
		{
			Name:      kubeletDevicePluginsVolumeName,
			MountPath: kubeletDevicePluginsPath,
		},
	}

	hostPathDirectory := v1.HostPathDirectory

	devicePluginVolume := v1.Volume{
		Name: kubeletDevicePluginsVolumeName,
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: kubeletDevicePluginsPath,
				Type: &hostPathDirectory,
			},
		},
	}

	standardLabels, nodeSelector := generateDevicePluginLabelsAndSelector(mod)

	ds.SetLabels(
		overrideLabels(ds.GetLabels(), standardLabels),
	)

	serviceAccountName := mod.Spec.DevicePlugin.ServiceAccountName
	if serviceAccountName == "" {
		if mod.Namespace == dsci.operatorNamespace {
			serviceAccountName = "kmm-operator-device-plugin"
		} else {
			log.FromContext(ctx).Info(utils.WarnString("No ServiceAccount set for the device plugin DaemonSet"))
		}
	}

	ds.Spec = appsv1.DaemonSetSpec{
		Selector: &metav1.LabelSelector{MatchLabels: standardLabels},
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:     standardLabels,
				Finalizers: []string{constants.NodeLabelerFinalizer},
			},
			Spec: v1.PodSpec{
				InitContainers:     generatePodContainerSpec(mod.Spec.DevicePlugin.InitContainer, "device-plugin-init", nil),
				Containers:         generatePodContainerSpec(&mod.Spec.DevicePlugin.Container, "device-plugin", containerVolumeMounts),
				PriorityClassName:  "system-node-critical",
				ImagePullSecrets:   getPodPullSecrets(mod.Spec.ImageRepoSecret),
				NodeSelector:       nodeSelector,
				ServiceAccountName: serviceAccountName,
				Volumes:            append([]v1.Volume{devicePluginVolume}, mod.Spec.DevicePlugin.Volumes...),
				Tolerations:        mod.Spec.Tolerations,
			},
		},
	}

	return controllerutil.SetControllerReference(mod, ds, dsci.scheme)
}

func generatePodContainerSpec(containerSpec *kmmv1beta1.DevicePluginContainerSpec, containerName string, presetVolumeMounts []v1.VolumeMount) []v1.Container {
	if containerSpec == nil {
		return nil
	}
	return []v1.Container{
		{
			Args:            containerSpec.Args,
			Command:         containerSpec.Command,
			Env:             containerSpec.Env,
			Name:            containerName,
			Image:           containerSpec.Image,
			ImagePullPolicy: containerSpec.ImagePullPolicy,
			Resources:       containerSpec.Resources,
			SecurityContext: &v1.SecurityContext{Privileged: ptr.To(true)},
			VolumeMounts:    append(containerSpec.VolumeMounts, presetVolumeMounts...),
		},
	}
}

func generateDevicePluginLabelsAndSelector(mod *kmmv1beta1.Module) (map[string]string, map[string]string) {
	labels := map[string]string{constants.ModuleNameLabel: mod.Name}
	nodeSelector := map[string]string{utils.GetKernelModuleReadyNodeLabel(mod.Namespace, mod.Name): ""}

	if mod.Spec.ModuleLoader != nil && mod.Spec.ModuleLoader.Container.Version != "" {
		versionLabel := utils.GetDevicePluginVersionLabelName(mod.Namespace, mod.Name)
		labels[versionLabel] = mod.Spec.ModuleLoader.Container.Version
		nodeSelector[versionLabel] = mod.Spec.ModuleLoader.Container.Version
	} else if mod.Spec.ModuleLoader == nil {
		nodeSelector = mod.Spec.Selector
	}

	return labels, nodeSelector
}

func isModuleBuildAndSignCapable(mod *kmmv1beta1.Module) (bool, bool) {
	buildCapable := mod.Spec.ModuleLoader.Container.Build != nil
	signCapable := mod.Spec.ModuleLoader.Container.Sign != nil
	if buildCapable && signCapable {
		return true, true
	}
	for _, mapping := range mod.Spec.ModuleLoader.Container.KernelMappings {
		if mapping.Sign != nil {
			signCapable = true
		}
		if mapping.Build != nil {
			buildCapable = true
		}
	}
	return buildCapable, signCapable
}

func getExistingDSFromVersion(existingDS []appsv1.DaemonSet,
	moduleNamespace string,
	moduleName string,
	moduleLoader *kmmv1beta1.ModuleLoaderSpec) (*appsv1.DaemonSet, string) {
	version := ""
	if moduleLoader != nil {
		version = moduleLoader.Container.Version
	}

	versionLabel := utils.GetDevicePluginVersionLabelName(moduleNamespace, moduleName)
	for _, ds := range existingDS {
		dsModuleVersion := ds.GetLabels()[versionLabel]
		if dsModuleVersion == version {
			return &ds, version
		}
	}
	return nil, version
}

func isOlderVersionUnusedDevicePluginDaemonset(ds *appsv1.DaemonSet, moduleVersion string) bool {
	moduleName := ds.Labels[constants.ModuleNameLabel]
	moduleNamespace := ds.Namespace
	versionLabel := utils.GetDevicePluginVersionLabelName(moduleNamespace, moduleName)
	return ds.Labels[versionLabel] != moduleVersion && ds.Status.DesiredNumberScheduled == 0
}

func overrideLabels(labels, overrides map[string]string) map[string]string {
	if labels == nil {
		labels = make(map[string]string, len(overrides))
	}

	for k, v := range overrides {
		labels[k] = v
	}

	return labels
}

func getPodPullSecrets(secret *v1.LocalObjectReference) []v1.LocalObjectReference {
	if secret == nil {
		return nil
	}

	return []v1.LocalObjectReference{*secret}
}
