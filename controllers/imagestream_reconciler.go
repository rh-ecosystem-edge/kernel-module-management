package controllers

import (
	"context"
	"fmt"

	imagev1 "github.com/openshift/api/image/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

//+kubebuilder:rbac:groups="image.openshift.io",resources=imagestreams,verbs=get;list;watch

type ImageStreamReconciler struct {
	client             client.Client
	filter             *filter.Filter
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping
}

func NewImageStreamReconciler(client client.Client, filter *filter.Filter,
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping) *ImageStreamReconciler {

	return &ImageStreamReconciler{
		client:             client,
		filter:             filter,
		kernelOsDtkMapping: kernelOsDtkMapping,
	}
}

func (r *ImageStreamReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	logger := log.FromContext(ctx)

	is := imagev1.ImageStream{}

	if err := r.client.Get(ctx, req.NamespacedName, &is); err != nil {
		return ctrl.Result{}, fmt.Errorf("could not get imagestream %v: %v", req.Name, err)
	}

	for _, t := range is.Spec.Tags {
		if tag := t.Name; tag != "latest" {
			r.kernelOsDtkMapping.SetImageStreamInfo(tag, t.From.Name)
			logger.Info("registered imagestream info mapping", "osImageVersion", tag, "dtkImage", t.From.Name)
		}
	}

	return ctrl.Result{}, nil
}

func (r *ImageStreamReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.
		NewControllerManagedBy(mgr).
		Named("imagestream").
		For(&imagev1.ImageStream{}).
		WithEventFilter(r.filter.ImageStreamReconcilerPredicate()).
		Complete(r)
}
