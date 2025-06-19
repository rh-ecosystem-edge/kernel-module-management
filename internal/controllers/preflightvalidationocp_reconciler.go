/*


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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta2"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
)

const (
	preflightOSVersionValue = "preflightOSVersion"

	PreflightValidationOCPReconcilerName = "PreflightValidationOCP"
)

// PreflightValidationOCPReconciler reconciles a PreflightValidationOCP object
type preflightValidationOCPReconciler struct {
	filter *filter.Filter
	helper preflightOCPReconcilerHelper
}

func NewPreflightValidationOCPReconciler(
	client client.Client,
	filter *filter.Filter,
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping,
	scheme *runtime.Scheme) *preflightValidationOCPReconciler {
	helper := newPreflightOCPReconcilerHelper(client, kernelOsDtkMapping, scheme)
	return &preflightValidationOCPReconciler{
		filter: filter,
		helper: helper,
	}
}

func (r *preflightValidationOCPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(PreflightValidationOCPReconcilerName).
		For(&v1beta2.PreflightValidationOCP{}, builder.WithPredicates(filter.PreflightOCPReconcilerUpdatePredicate())).
		Owns(&v1beta2.PreflightValidation{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(
			reconcile.AsReconciler[*v1beta2.PreflightValidationOCP](mgr.GetClient(), r),
		)
}

// Reconcile Reconiliation entry point
func (r *preflightValidationOCPReconciler) Reconcile(ctx context.Context, pvo *v1beta2.PreflightValidationOCP) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Start PreflightValidationOCP Reconciliation")

	if pvo.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	r.helper.setDTKMapping(pvo)

	err := r.helper.preparePreflightValidation(ctx, pvo)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update PreflightValidation: %v", err)
	}

	err = r.helper.updateStatus(ctx, pvo)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status of PreflightValidationOCP: %v", err)
	}
	return ctrl.Result{}, nil
}

//go:generate mockgen -source=preflightvalidationocp_reconciler.go -package=controllers -destination=mock_preflightvalidationocp_reconciler.go preflightOCPReconcilerHelper
type preflightOCPReconcilerHelper interface {
	setDTKMapping(pvo *v1beta2.PreflightValidationOCP)
	preparePreflightValidation(ctx context.Context, pvo *v1beta2.PreflightValidationOCP) error
	updateStatus(ctx context.Context, pvo *v1beta2.PreflightValidationOCP) error
}

type preflightOCPReconcilerHelperImpl struct {
	client             client.Client
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping
	scheme             *runtime.Scheme
}

func newPreflightOCPReconcilerHelper(client client.Client,
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping,
	scheme *runtime.Scheme) preflightOCPReconcilerHelper {

	return &preflightOCPReconcilerHelperImpl{
		client:             client,
		kernelOsDtkMapping: kernelOsDtkMapping,
		scheme:             scheme,
	}
}

func (porhi *preflightOCPReconcilerHelperImpl) setDTKMapping(pvo *v1beta2.PreflightValidationOCP) {
	if pvo.Spec.DTKImage == "" {
		return
	}

	// set the DTK image mapping, only if it was not set previously
	_, err := porhi.kernelOsDtkMapping.GetImage(pvo.Spec.KernelVersion)
	if err != nil {
		porhi.kernelOsDtkMapping.SetNodeInfo(pvo.Spec.KernelVersion, preflightOSVersionValue)
		porhi.kernelOsDtkMapping.SetImageStreamInfo(preflightOSVersionValue, pvo.Spec.DTKImage)
	}
}

func (porhi *preflightOCPReconcilerHelperImpl) preparePreflightValidation(ctx context.Context, pvo *v1beta2.PreflightValidationOCP) error {
	pvObj := v1beta2.PreflightValidation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvo.Name,
			Namespace: pvo.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, porhi.client, &pvObj, func() error {
		pvObj.Spec.KernelVersion = pvo.Spec.KernelVersion
		pvObj.Spec.PushBuiltImage = pvo.Spec.PushBuiltImage

		return controllerutil.SetControllerReference(pvo, &pvObj, porhi.scheme)
	})

	return err
}

func (porhi *preflightOCPReconcilerHelperImpl) updateStatus(ctx context.Context, pvo *v1beta2.PreflightValidationOCP) error {
	pvObj := v1beta2.PreflightValidation{}
	err := porhi.client.Get(ctx, types.NamespacedName{Namespace: pvo.Namespace, Name: pvo.Name}, &pvObj)
	if err != nil {
		return fmt.Errorf("failed to get PreflightValidation %s/%s: %v", pvo.Namespace, pvo.Name, err)
	}

	patchFrom := client.MergeFrom(pvo.DeepCopy())
	pvo.Status = pvObj.Status
	return porhi.client.Status().Patch(ctx, pvo, patchFrom)
}
