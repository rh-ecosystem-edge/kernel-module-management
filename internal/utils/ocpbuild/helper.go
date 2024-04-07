package ocpbuild

import (
	"context"
	"errors"
	"fmt"

	buildv1 "github.com/openshift/api/build/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var ErrNoMatchingBuild = errors.New("no matching Build")

//go:generate mockgen -source=helper.go -package=ocpbuild -destination=mock_helper.go

type OCPBuildsHelper interface {
	GetModuleOCPBuildByKernel(ctx context.Context, mld *api.ModuleLoaderData, owner metav1.Object) (*buildv1.Build, error)
	GetModuleOCPBuilds(ctx context.Context, moduleName, moduleNamespace string, owner metav1.Object) ([]buildv1.Build, error)
	DeleteOCPBuild(ctx context.Context, build *buildv1.Build) error
	RemoveFinalizer(ctx context.Context, build *buildv1.Build, finalizer string) error
}

type ocpBuildsHelper struct {
	client    client.Client
	buildType string
}

func NewOCPBuildsHelper(client client.Client, buildType string) OCPBuildsHelper {
	return &ocpBuildsHelper{
		buildType: buildType,
		client:    client,
	}
}

func (o *ocpBuildsHelper) GetModuleOCPBuildByKernel(ctx context.Context, mld *api.ModuleLoaderData, owner metav1.Object) (*buildv1.Build, error) {
	buildList := buildv1.BuildList{}

	opts := []client.ListOption{
		client.MatchingLabels(GetOCPBuildLabels(mld, o.buildType)),
		client.InNamespace(mld.Namespace),
	}

	if err := o.client.List(ctx, &buildList, opts...); err != nil {
		return nil, fmt.Errorf("could not list Build: %v", err)
	}

	// filter OCP builds by owner, since they could have been created by the preflight
	// when checking that specific module
	moduleOwnedOCPBuilds := filterOCPBuildsByOwner(buildList.Items, owner)

	if n := len(moduleOwnedOCPBuilds); n == 0 {
		return nil, ErrNoMatchingBuild
	} else if n > 1 {
		return nil, fmt.Errorf("expected 0 or 1 Builds, got %d", n)
	}

	return &moduleOwnedOCPBuilds[0], nil
}

func (o *ocpBuildsHelper) GetModuleOCPBuilds(ctx context.Context, moduleName, moduleNamespace string, owner metav1.Object) ([]buildv1.Build, error) {
	buildList := buildv1.BuildList{}

	opts := []client.ListOption{
		client.MatchingLabels(moduleLabels(moduleName, o.buildType)),
		client.InNamespace(moduleNamespace),
	}

	if err := o.client.List(ctx, &buildList, opts...); err != nil {
		return nil, fmt.Errorf("could not list Build: %v", err)
	}

	// filter OCP builds by owner, since they could have been created by the preflight
	// when checking that specific module
	moduleOwnedBuilds := filterOCPBuildsByOwner(buildList.Items, owner)

	return moduleOwnedBuilds, nil
}

func (o *ocpBuildsHelper) DeleteOCPBuild(ctx context.Context, build *buildv1.Build) error {
	opts := []client.DeleteOption{
		client.PropagationPolicy(metav1.DeletePropagationBackground),
	}
	return o.client.Delete(ctx, build, opts...)
}

func (o *ocpBuildsHelper) RemoveFinalizer(ctx context.Context, build *buildv1.Build, finalizer string) error {
	if !controllerutil.RemoveFinalizer(build, finalizer) {
		return nil
	}

	podCopy := build.DeepCopy()

	controllerutil.RemoveFinalizer(build, finalizer)

	return o.client.Patch(ctx, build, client.MergeFrom(podCopy))
}

func filterOCPBuildsByOwner(builds []buildv1.Build, owner metav1.Object) []buildv1.Build {
	ownedBuilds := []buildv1.Build{}
	for _, build := range builds {
		if metav1.IsControlledBy(&build, owner) {
			ownedBuilds = append(ownedBuilds, build)
		}
	}
	return ownedBuilds
}
