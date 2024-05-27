package controllers

import (
	"context"
	"fmt"

	imagev1 "github.com/openshift/api/image/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

//+kubebuilder:rbac:groups="image.openshift.io",resources=imagestreams,verbs=get;list;watch

const ImageStreamReconcilerName = "ImageStream"

type ImageStreamReconciler struct {
	client             client.Client
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping
	nsn                types.NamespacedName
}

func NewImageStreamReconciler(
	client client.Client,
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping,
	nsn types.NamespacedName,
) *ImageStreamReconciler {
	return &ImageStreamReconciler{
		client:             client,
		kernelOsDtkMapping: kernelOsDtkMapping,
		nsn:                nsn,
	}
}

func (r *ImageStreamReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	is := imagev1.ImageStream{}

	if err := r.client.Get(ctx, r.nsn, &is); err != nil {
		return ctrl.Result{}, fmt.Errorf("could not get imagestream %v: %v", r.nsn, err)
	}

	for _, t := range is.Spec.Tags {
		if tag := t.Name; tag != "latest" {
			r.kernelOsDtkMapping.SetImageStreamInfo(tag, t.From.Name)
			logger.Info("registered imagestream info mapping", "osImageVersion", tag, "dtkImage", t.From.Name)
		}
	}

	return ctrl.Result{}, nil
}

func (r *ImageStreamReconciler) SetupWithManager(mgr ctrl.Manager, f *filter.Filter) error {
	dtkPredicates := predicate.And(
		filter.MatchesNamespacedNamePredicate(r.nsn),
		f.ImageStreamReconcilerPredicate(),
	)

	return ctrl.
		NewControllerManagedBy(mgr).
		Named(ImageStreamReconcilerName).
		For(
			&imagev1.ImageStream{},
			builder.WithPredicates(dtkPredicates),
		).
		Complete(r)
}
