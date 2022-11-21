package buildconfig

import (
	"context"
	"errors"
	"fmt"

	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const buildConfigHashAnnotation = "kmm.node.kubernetes.io/last-hash"

var (
	ErrNoMatchingBuildConfig = errors.New("no matching BuildConfig")
	errNoMatchingBuild       = errors.New("no matching Build")
)

type buildConfigManager struct {
	client          client.Client
	maker           Maker
	ocpBuildsHelper OpenShiftBuildsHelper
}

func NewManager(client client.Client, maker Maker, ocpBuildsHelper OpenShiftBuildsHelper) *buildConfigManager {
	return &buildConfigManager{
		client:          client,
		maker:           maker,
		ocpBuildsHelper: ocpBuildsHelper,
	}
}

func (bcm *buildConfigManager) Sync(ctx context.Context, mod kmmv1beta1.Module, m kmmv1beta1.KernelMapping,
	targetKernel, targetImage string, pushImage bool, kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping) (build.Result, error) {

	logger := log.FromContext(ctx)

	buildConfigTemplate, err := bcm.maker.MakeBuildConfigTemplate(mod, m, targetKernel, targetImage, pushImage, kernelOsDtkMapping)
	if err != nil {
		return build.Result{}, fmt.Errorf("could not make BuildConfig template: %v", err)
	}

	buildConfig, err := bcm.ocpBuildsHelper.GetBuildConfig(ctx, mod, targetKernel)
	if err != nil {
		if !errors.Is(err, ErrNoMatchingBuildConfig) {
			return build.Result{}, fmt.Errorf("error getting the build: %v", err)
		}

		logger.Info("Creating BuildConfig")

		if err = bcm.client.Create(ctx, buildConfigTemplate); err != nil {
			return build.Result{}, fmt.Errorf("could not create BuildConfig: %v", err)
		}

		return build.Result{Status: build.StatusCreated, Requeue: true}, nil
	}

	changed, err := bcm.isBuildConfigChanged(buildConfig, buildConfigTemplate)
	if err != nil {
		return build.Result{}, fmt.Errorf("could not determine if BuildConfig has changed: %v", err)
	}

	if changed {
		logger.Info("The module's build spec has been changed, deleting the current BuildConfig so a new one can be created", "name", buildConfig.Name)
		err = bcm.client.Delete(ctx, buildConfig)
		if err != nil {
			logger.Info(utils.WarnString(fmt.Sprintf("failed to delete BuildConfig %s: %v", buildConfig.Name, err)))
		}
		return build.Result{Status: build.StatusInProgress, Requeue: true}, nil
	}

	b, err := bcm.ocpBuildsHelper.GetLatestBuild(ctx, mod.Namespace, buildConfig.Name)
	if err != nil {
		return build.Result{}, fmt.Errorf("could not find the latest build: %v", err)
	}

	switch b.Status.Phase {
	case buildv1.BuildPhaseComplete:
		return build.Result{Status: build.StatusCompleted}, nil
	case buildv1.BuildPhaseNew, buildv1.BuildPhasePending, buildv1.BuildPhaseRunning:
		return build.Result{Status: build.StatusInProgress, Requeue: true}, nil
	case buildv1.BuildPhaseFailed:
		return build.Result{}, fmt.Errorf("buildConfig failed: %v", b.Status.LogSnippet)
	default:
		return build.Result{}, fmt.Errorf("unknown status: %v", buildConfig.Status)
	}
}

func (bcm *buildConfigManager) isBuildConfigChanged(existingBuildConfig *buildv1.BuildConfig, newBuildConfig *buildv1.BuildConfig) (bool, error) {
	existingAnnotations := existingBuildConfig.GetAnnotations()
	newAnnotations := newBuildConfig.GetAnnotations()
	if existingAnnotations == nil {
		return false, fmt.Errorf("annotations are not present in the existing BuildConfig %s", existingBuildConfig.Name)
	}
	if existingAnnotations[buildConfigHashAnnotation] == newAnnotations[buildConfigHashAnnotation] {
		return false, nil
	}
	return true, nil
}

//go:generate mockgen -source=manager.go -package=buildconfig -destination=mock_manager.go

type OpenShiftBuildsHelper interface {
	GetBuildConfig(ctx context.Context, mod kmmv1beta1.Module, targetKernel string) (*buildv1.BuildConfig, error)
	GetLatestBuild(ctx context.Context, namespace, buildConfigName string) (*buildv1.Build, error)
}

type openShiftBuildsHelper struct {
	client client.Client
}

func NewOpenShiftBuildsHelper(client client.Client) OpenShiftBuildsHelper {
	return &openShiftBuildsHelper{client: client}
}

func (osbh *openShiftBuildsHelper) GetBuildConfig(ctx context.Context, mod kmmv1beta1.Module, targetKernel string) (*buildv1.BuildConfig, error) {
	buildConfigList := buildv1.BuildConfigList{}

	opts := []client.ListOption{
		client.MatchingLabels(build.GetBuildLabels(mod, targetKernel)),
		client.InNamespace(mod.Namespace),
	}

	if err := osbh.client.List(ctx, &buildConfigList, opts...); err != nil {
		return nil, fmt.Errorf("could not list BuildConfigs: %v", err)
	}

	if n := len(buildConfigList.Items); n == 0 {
		return nil, ErrNoMatchingBuildConfig
	} else if n > 1 {
		return nil, fmt.Errorf("expected 0 or 1 BuildConfigs, got %d", n)
	}

	return &buildConfigList.Items[0], nil
}

func (osbh *openShiftBuildsHelper) GetLatestBuild(ctx context.Context, namespace, buildConfigName string) (*buildv1.Build, error) {
	builds := buildv1.BuildList{}

	opts := []client.ListOption{
		client.MatchingLabels(map[string]string{"openshift.io/build-config.name": buildConfigName}),
		client.InNamespace(namespace),
	}

	if err := osbh.client.List(ctx, &builds, opts...); err != nil {
		return nil, fmt.Errorf("could not list builds: %v", err)
	}

	if len(builds.Items) == 0 {
		return nil, errNoMatchingBuild
	}

	latest := buildv1.Build{}

	for _, b := range builds.Items {
		if b.CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = b
		}
	}

	return &latest, nil
}
