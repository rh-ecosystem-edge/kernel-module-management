package controllers

import (
	"context"
	"fmt"

	apimcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	apioperatorv1 "github.com/openshift/api/operator/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/mcfg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const BMCReconcilerName = "BootModuleConfigReconciler"

// BMC reconciler handle the changes/creation of the MachineConfig that can load day1 kernel module
type bmcReconciler struct {
	reconHelper bmcReconcilerHelperAPI
}

func NewBMCReconciler(client client.Client, mcfgAPI mcfg.MCFG, scheme *runtime.Scheme) *bmcReconciler {
	helper := newBMCReconcilerHelper(client, mcfgAPI, scheme)
	return &bmcReconciler{
		reconHelper: helper,
	}
}

func (r *bmcReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kmmv1beta1.BootModuleConfig{}).
		Named(BMCReconcilerName).
		Owns(&apimcfgv1.MachineConfig{}).
		Complete(
			reconcile.AsReconciler[*kmmv1beta1.BootModuleConfig](mgr.GetClient(), r),
		)
}

func (r *bmcReconciler) Reconcile(ctx context.Context, bmc *kmmv1beta1.BootModuleConfig) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Starting BootModuleConfig reconciliation")

	if bmc.GetDeletionTimestamp() != nil {
		// bootmoduleconfig is being deleted
		err := r.reconHelper.finalizeBMC(ctx, bmc)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to finalize BootModuleConfig %s/%s: %v", bmc.Namespace, bmc.Name, err)
		}
		return ctrl.Result{}, nil
	}

	err := r.reconHelper.setFinalizer(ctx, bmc)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set finalize on BootModuleConfig %s/%s: %v", bmc.Namespace, bmc.Name, err)
	}

	// handling MachineConfiguration must come before handling MachineConfig in order to avoid reboots
	err = r.reconHelper.handleMachineConfiguration(ctx, bmc)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to handle MachineConfiguration for BootModuleConfig %s/%s: %v", bmc.Namespace, bmc.Name, err)
	}

	err = r.reconHelper.handleMachineConfig(ctx, bmc)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to handle MachineConfig for BootModuleConfig %s/%s: %v", bmc.Namespace, bmc.Name, err)
	}

	return ctrl.Result{}, nil
}

//go:generate mockgen -source=bmc_reconciler.go -package=controllers -destination=mock_bmc_reconciler.go bmcReconcilerHelperAPI

type bmcReconcilerHelperAPI interface {
	finalizeBMC(ctx context.Context, bmc *kmmv1beta1.BootModuleConfig) error
	setFinalizer(ctx context.Context, bmc *kmmv1beta1.BootModuleConfig) error
	handleMachineConfiguration(ctx context.Context, bmc *kmmv1beta1.BootModuleConfig) error
	handleMachineConfig(ctx context.Context, bmc *kmmv1beta1.BootModuleConfig) error
}

type bmcReconcilerHelper struct {
	client  client.Client
	mcfgAPI mcfg.MCFG
	scheme  *runtime.Scheme
}

func newBMCReconcilerHelper(client client.Client, mcfgAPI mcfg.MCFG, scheme *runtime.Scheme) bmcReconcilerHelperAPI {
	return &bmcReconcilerHelper{
		client:  client,
		mcfgAPI: mcfgAPI,
		scheme:  scheme,
	}
}

func (brh *bmcReconcilerHelper) setFinalizer(ctx context.Context, bmc *kmmv1beta1.BootModuleConfig) error {
	if controllerutil.ContainsFinalizer(bmc, constants.BMCFinalizer) {
		return nil
	}

	bmcCopy := bmc.DeepCopy()
	controllerutil.AddFinalizer(bmcCopy, constants.BMCFinalizer)
	return brh.client.Patch(ctx, bmc, client.MergeFrom(bmcCopy))
}

func (brh *bmcReconcilerHelper) finalizeBMC(ctx context.Context, bmc *kmmv1beta1.BootModuleConfig) error {
	// [TODO] determine whether MC was created by user, or by BMC. This can be done by looking at MC labels.
	// if MC is created by BMC - delete it, otherwise  - ignore
	return nil
}

func (brh *bmcReconcilerHelper) handleMachineConfiguration(ctx context.Context, bmc *kmmv1beta1.BootModuleConfig) error {
	logger := log.FromContext(ctx).WithValues("bmc", bmc.GetName())
	mc := &apioperatorv1.MachineConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
	opRes, err := controllerutil.CreateOrPatch(ctx, brh.client, mc, func() error {
		brh.mcfgAPI.UpdateDisruptionPolicies(mc, bmc)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create/patch MachineConfiguration cluster: %v", err)
	}
	logger.Info("handleMachineConfiguration successfull", "opRes", opRes)
	return nil
}

func (brh *bmcReconcilerHelper) handleMachineConfig(ctx context.Context, bmc *kmmv1beta1.BootModuleConfig) error {
	logger := log.FromContext(ctx).WithValues("bmc", bmc.GetName())
	mc := &apimcfgv1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: bmc.Spec.MachineConfigName,
		},
	}

	opRes, err := controllerutil.CreateOrPatch(ctx, brh.client, mc, func() error {
		return brh.mcfgAPI.UpdateMachineConfig(mc, bmc)
	})
	if err != nil {
		return fmt.Errorf("failed to create/patch MachineConfig %s: %v", bmc.Spec.MachineConfigName, err)
	}
	logger.Info("handleMachineConfig successfull", "opRes", opRes)

	return nil
}
