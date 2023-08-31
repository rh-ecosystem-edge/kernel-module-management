package controllers

import (
	"context"
	"fmt"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/ocp/ca"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//+kubebuilder:rbac:groups="core",resources=configmaps,verbs=create;get;list;patch;watch

const ModuleCAReconcilerName = "ModuleCAReconciler"

type ModuleCAReconciler struct {
	caHelper          ca.Helper
	client            client.Client
	operatorNamespace string
}

func NewModuleCAReconciler(client client.Client, caHelper ca.Helper, operatorNamespace string) *ModuleCAReconciler {
	return &ModuleCAReconciler{
		caHelper:          caHelper,
		client:            client,
		operatorNamespace: operatorNamespace,
	}
}

func (r *ModuleCAReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	mod := kmmv1beta1.Module{}

	if err := r.client.Get(ctx, req.NamespacedName, &mod); err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, fmt.Errorf("could not get Module %s: %v", req.NamespacedName, err)
	}

	logger := ctrl.LoggerFrom(ctx)

	if req.Namespace == r.operatorNamespace {
		logger.Info("Module is in the operator namespace; not syncing CA ConfigMaps")
		return reconcile.Result{}, nil
	}

	logger.Info("Syncing CA ConfigMaps", "namespace", req.Namespace)

	if err := r.caHelper.Sync(ctx, req.Namespace, &mod); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to synchronize CA ConfigMaps: %v", err)
	}

	return reconcile.Result{}, nil
}

func (r *ModuleCAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.
		NewControllerManagedBy(mgr).
		For(&kmmv1beta1.Module{}).
		Owns(&v1.ConfigMap{}).
		Named(ModuleCAReconcilerName).
		Complete(r)
}
