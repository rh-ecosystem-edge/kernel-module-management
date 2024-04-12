package controllers

import (
	"context"
	"time"

	buildv1 "github.com/openshift/api/build/v1"
	buildocpbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build/ocpbuild"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	signocpbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/sign/ocpbuild"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const JobGCReconcilerName = "JobGCReconciler"

// JobGCReconciler removes the GC finalizer from deleted build & signing builds, after the optional GC delay has passed
// or if the build has failed.
type JobGCReconciler struct {
	client client.Client
	delay  time.Duration
}

func NewJobGCReconciler(client client.Client, delay time.Duration) *JobGCReconciler {
	return &JobGCReconciler{
		client: client,
		delay:  delay,
	}
}

func (r *JobGCReconciler) Reconcile(ctx context.Context, build *buildv1.Build) (reconcile.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	releaseAt := build.DeletionTimestamp.Add(r.delay)
	now := time.Now()

	// Only delay the deletion of successful builds.
	if build.Status.Phase != buildv1.BuildPhaseComplete || now.After(releaseAt) {
		logger.Info("Releasing finalizer")

		buildCopy := build.DeepCopy()

		controllerutil.RemoveFinalizer(build, constants.GCDelayFinalizer)

		return reconcile.Result{}, r.client.Patch(ctx, build, client.MergeFrom(buildCopy))
	}

	requeueAfter := releaseAt.Sub(now)

	logger.Info("Not yet removing finalizer", "requeue after", requeueAfter)

	return reconcile.Result{RequeueAfter: requeueAfter}, nil
}

func (r *JobGCReconciler) SetupWithManager(mgr manager.Manager) error {
	podTypes := sets.New(buildocpbuild.BuildType, signocpbuild.BuildType)

	p := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return podTypes.Has(
			object.GetLabels()[constants.BuildTypeLabel],
		) &&
			controllerutil.ContainsFinalizer(object, constants.GCDelayFinalizer) &&
			object.GetDeletionTimestamp() != nil
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(
			&buildv1.Build{},
			builder.WithPredicates(p),
		).
		Named(JobGCReconcilerName).
		Complete(
			reconcile.AsReconciler[*buildv1.Build](r.client, r),
		)
}
