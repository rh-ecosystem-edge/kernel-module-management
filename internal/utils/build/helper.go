package build

import (
	"context"
	"errors"
	"fmt"

	buildv1 "github.com/openshift/api/build/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrNoMatchingBuild = errors.New("no matching Build")

//go:generate mockgen -source=helper.go -package=build -destination=mock_helper.go

type OpenShiftBuildsHelper interface {
	GetModuleBuildByKernel(ctx context.Context, mld *api.ModuleLoaderData) (*buildv1.Build, error)
	GetModuleBuilds(ctx context.Context, moduleName, moduleNamespace string) ([]buildv1.Build, error)
	DeleteBuild(ctx context.Context, build *buildv1.Build) error
}

type openShiftBuildsHelper struct {
	client    client.Client
	buildType string
}

func NewOpenShiftBuildsHelper(client client.Client, buildType string) OpenShiftBuildsHelper {
	return &openShiftBuildsHelper{
		buildType: buildType,
		client:    client,
	}
}

func (o *openShiftBuildsHelper) GetModuleBuildByKernel(ctx context.Context, mld *api.ModuleLoaderData) (*buildv1.Build, error) {
	buildList := buildv1.BuildList{}

	opts := []client.ListOption{
		client.MatchingLabels(GetBuildLabels(mld, o.buildType)),
		client.InNamespace(mld.Namespace),
	}

	if err := o.client.List(ctx, &buildList, opts...); err != nil {
		return nil, fmt.Errorf("could not list Build: %v", err)
	}

	if n := len(buildList.Items); n == 0 {
		return nil, ErrNoMatchingBuild
	} else if n > 1 {
		return nil, fmt.Errorf("expected 0 or 1 Builds, got %d", n)
	}

	return &buildList.Items[0], nil
}

func (o *openShiftBuildsHelper) GetModuleBuilds(ctx context.Context, moduleName, moduleNamespace string) ([]buildv1.Build, error) {
	buildList := buildv1.BuildList{}

	opts := []client.ListOption{
		client.MatchingLabels(moduleLabels(moduleName, o.buildType)),
		client.InNamespace(moduleNamespace),
	}

	if err := o.client.List(ctx, &buildList, opts...); err != nil {
		return nil, fmt.Errorf("could not list Build: %v", err)
	}

	return buildList.Items, nil
}

func (o *openShiftBuildsHelper) DeleteBuild(ctx context.Context, build *buildv1.Build) error {
	opts := []client.DeleteOption{
		client.PropagationPolicy(metav1.DeletePropagationBackground),
	}
	return o.client.Delete(ctx, build, opts...)
}
