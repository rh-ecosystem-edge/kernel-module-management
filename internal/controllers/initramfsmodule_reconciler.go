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
	"fmt"
	"sort"
	"strings"

	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/mcfg"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/mic"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/node"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const InitramfsModuleReconcilerName = "InitramfsModuleReconciler"

const workerNodeLabelKey = "node-role.kubernetes.io/worker"

// +kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=initramfsmodules,verbs=get;list;patch;update;watch
// +kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=initramfsmodules/status,verbs=get;patch;update
// +kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=initramfsmodules/finalizers,verbs=update
// +kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=moduleimagesconfigs,verbs=create;get;list;patch;watch
// +kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigpools,verbs=get;list;watch;create;update;patch;delete

type initramfsModuleReconciler struct {
	helper initramfsModuleReconcilerHelper
}

func NewInitramfsModuleReconciler(client client.Client, micAPI mic.MIC, nodeAPI node.Node, mcfgAPI mcfg.MCFG) *initramfsModuleReconciler {
	return &initramfsModuleReconciler{
		helper: newInitramfsModuleReconcilerHelper(client, micAPI, nodeAPI, mcfgAPI),
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *initramfsModuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kmmv1beta1.InitramfsModule{}).
		Owns(&kmmv1beta1.ModuleImagesConfig{}).
		Named(InitramfsModuleReconcilerName).
		Complete(reconcile.AsReconciler[*kmmv1beta1.InitramfsModule](mgr.GetClient(), r))
}

func (r *initramfsModuleReconciler) Reconcile(ctx context.Context, irm *kmmv1beta1.InitramfsModule) (ctrl.Result, error) {
	res := ctrl.Result{}
	if irm.GetDeletionTimestamp() != nil {
		return res, r.helper.finalize(ctx, irm)
	}

	if err := r.helper.checkUnstampedPoolAndMachineConfig(ctx, irm); err != nil {
		return res, err
	}

	nodes, err := r.helper.listSelectedNodes(ctx, irm)
	if err != nil {
		return res, err
	}

	if err = r.helper.handleMIC(ctx, irm, nodes); err != nil {
		return res, err
	}
	parsed, err := r.helper.handleParseJob(ctx, irm)
	if err != nil {
		return res, err
	}
	if err = r.helper.handleMachineConfigPool(ctx, irm); err != nil {
		return res, err
	}
	if err = r.helper.handleMachineConfig(ctx, irm, parsed); err != nil {
		return res, err
	}
	return res, nil
}

//go:generate mockgen -source=initramfsmodule_reconciler.go -package=controllers -destination=mock_initramfsmodule_reconciler.go initramfsModuleReconcilerHelper

type initramfsModuleReconcilerHelper interface {
	listSelectedNodes(ctx context.Context, irm *kmmv1beta1.InitramfsModule) ([]v1.Node, error)
	handleMIC(ctx context.Context, irm *kmmv1beta1.InitramfsModule, nodes []v1.Node) error
	finalize(ctx context.Context, irm *kmmv1beta1.InitramfsModule) error
	checkUnstampedPoolAndMachineConfig(ctx context.Context, irm *kmmv1beta1.InitramfsModule) error
	handleParseJob(ctx context.Context, irm *kmmv1beta1.InitramfsModule) (initramfsParseParams, error)
	handleMachineConfigPool(ctx context.Context, irm *kmmv1beta1.InitramfsModule) error
	handleMachineConfig(ctx context.Context, irm *kmmv1beta1.InitramfsModule, parsed initramfsParseParams) error
}

type initramfsModuleReconcilerHelperImpl struct {
	client  client.Client
	micAPI  mic.MIC
	nodeAPI node.Node
	mcfgAPI mcfg.MCFG
}

func newInitramfsModuleReconcilerHelper(client client.Client, micAPI mic.MIC, nodeAPI node.Node, mcfgAPI mcfg.MCFG) initramfsModuleReconcilerHelper {
	return &initramfsModuleReconcilerHelperImpl{
		client:  client,
		micAPI:  micAPI,
		nodeAPI: nodeAPI,
		mcfgAPI: mcfgAPI,
	}
}

func (h *initramfsModuleReconcilerHelperImpl) listSelectedNodes(ctx context.Context, irm *kmmv1beta1.InitramfsModule) ([]v1.Node, error) {
	selector := irm.Spec.Selector
	if len(selector) == 0 {
		selector = map[string]string{workerNodeLabelKey: ""}
	}
	return h.nodeAPI.GetAllNodesBySelector(ctx, selector)
}

func (h *initramfsModuleReconcilerHelperImpl) handleMIC(ctx context.Context, irm *kmmv1beta1.InitramfsModule, nodes []v1.Node) error {
	seen := make(map[string]struct{}, len(nodes))
	kernels := make([]string, 0, len(nodes))
	for i := range nodes {
		kernel := strings.ReplaceAll(nodes[i].Status.NodeInfo.KernelVersion, "+", "-")
		if kernel == "" {
			continue
		}
		if _, ok := seen[kernel]; ok {
			continue
		}
		seen[kernel] = struct{}{}
		kernels = append(kernels, kernel)
	}
	if len(kernels) == 0 {
		return nil
	}
	sort.Strings(kernels)

	images := make([]kmmv1beta1.ModuleImageSpec, 0, len(kernels))
	for _, kernel := range kernels {
		images = append(images, kmmv1beta1.ModuleImageSpec{
			Image:         irm.Spec.ContainerImage + "-" + kernel,
			KernelVersion: kernel,
			Build:         irm.Spec.Build,
			Sign:          irm.Spec.Sign,
			DirName:       irm.Spec.DirName,
		})
	}
	return h.micAPI.CreateOrPatch(
		ctx,
		"irm-"+irm.Name,
		irm.Namespace,
		images,
		irm.Spec.ImageRepoSecret,
		"",
		true,
		nil,
		nil,
		irm,
	)
}

func (h *initramfsModuleReconcilerHelperImpl) finalize(context.Context, *kmmv1beta1.InitramfsModule) error {
	return nil
}

func (h *initramfsModuleReconcilerHelperImpl) handleMachineConfigPool(ctx context.Context, irm *kmmv1beta1.InitramfsModule) error {
	pool := &mcfgv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: mcfg.InitramfsPoolName}}
	_, err := controllerutil.CreateOrPatch(ctx, h.client, pool, func() error {
		h.mcfgAPI.UpdateInitramfsPool(pool, irm)
		return nil
	})
	return err
}

func (h *initramfsModuleReconcilerHelperImpl) handleMachineConfig(ctx context.Context, irm *kmmv1beta1.InitramfsModule, parsed initramfsParseParams) error {
	mc := &mcfgv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: mcfg.InitramfsMachineConfigName}}
	_, err := controllerutil.CreateOrPatch(ctx, h.client, mc, func() error {
		return h.mcfgAPI.UpdateInitramfsMachineConfig(mc, irm, parsed.Kernels)
	})
	return err
}

type initramfsParseParams struct {
	Kernels map[string]mcfg.KernelInspectLists
}

func (h *initramfsModuleReconcilerHelperImpl) handleParseJob(context.Context, *kmmv1beta1.InitramfsModule) (initramfsParseParams, error) {
	return initramfsParseParams{}, nil
}

func (h *initramfsModuleReconcilerHelperImpl) checkUnstampedPoolAndMachineConfig(ctx context.Context, irm *kmmv1beta1.InitramfsModule) error {
	pool, mc, err := h.lookupPoolAndMachineConfig(ctx)
	if err != nil {
		return err
	}
	return h.refuseUnstamped(pool, mc, irm)
}

func (h *initramfsModuleReconcilerHelperImpl) lookupPoolAndMachineConfig(ctx context.Context) (*mcfgv1.MachineConfigPool, *mcfgv1.MachineConfig, error) {
	pool := &mcfgv1.MachineConfigPool{}
	found, err := getIfExists(ctx, h.client, mcfg.InitramfsPoolName, pool)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		pool = nil
	}
	mc := &mcfgv1.MachineConfig{}
	found, err = getIfExists(ctx, h.client, mcfg.InitramfsMachineConfigName, mc)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		mc = nil
	}
	return pool, mc, nil
}

func getIfExists(ctx context.Context, c client.Client, name string, obj client.Object) (bool, error) {
	err := c.Get(ctx, client.ObjectKey{Name: name}, obj)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (h *initramfsModuleReconcilerHelperImpl) refuseUnstamped(pool *mcfgv1.MachineConfigPool, mc *mcfgv1.MachineConfig, irm *kmmv1beta1.InitramfsModule) error {
	if pool != nil && !h.mcfgAPI.StampedForInitramfsModule(pool, irm) {
		return fmt.Errorf("MachineConfigPool %s already exists and is not stamped for this CR", mcfg.InitramfsPoolName)
	}
	if mc != nil && !h.mcfgAPI.StampedForInitramfsModule(mc, irm) {
		return fmt.Errorf("MachineConfig %s already exists and is not stamped for this CR", mcfg.InitramfsMachineConfigName)
	}
	return nil
}
