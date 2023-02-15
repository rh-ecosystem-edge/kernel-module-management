package buildconfig

import (
	"context"
	"errors"
	"fmt"

	buildv1 "github.com/openshift/api/build/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	kmmbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils"
)

const buildHashAnnotation = "kmm.node.kubernetes.io/last-hash"

var (
	errNoMatchingBuild = errors.New("no matching Build")
)

type buildManager struct {
	client          client.Client
	maker           Maker
	ocpBuildsHelper OpenShiftBuildsHelper
	authFactory     auth.RegistryAuthGetterFactory
	registry        registry.Registry
}

func NewManager(
	client client.Client,
	maker Maker,
	ocpBuildsHelper OpenShiftBuildsHelper,
	authFactory auth.RegistryAuthGetterFactory,
	registry registry.Registry) *buildManager {
	return &buildManager{
		client:          client,
		maker:           maker,
		ocpBuildsHelper: ocpBuildsHelper,
		authFactory:     authFactory,
		registry:        registry,
	}
}

func (bcm *buildManager) GarbageCollect(ctx context.Context, modName, namespace string, owner metav1.Object) ([]string, error) {

	//Garbage Collection noti (yet) implemented for Build
	return nil, nil
}

func (bcm *buildManager) ShouldSync(ctx context.Context, mod kmmv1beta1.Module, m kmmv1beta1.KernelMapping) (bool, error) {
	// if there is no build specified skip
	if !module.ShouldBeBuilt(m) {
		return false, nil
	}

	targetImage := m.ContainerImage

	// if build AND sign are specified, then we will build an intermediate image
	// and let sign produce the one specified in targetImage
	if module.ShouldBeSigned(m) {
		targetImage = module.IntermediateImageName(mod.Name, mod.Namespace, targetImage)
	}

	// build is specified and targetImage is either the final image or the intermediate image
	// tag, depending on whether sign is specified or not. Either way, if targetImage exists
	// we can skip building it
	exists, err := module.ImageExists(ctx, bcm.authFactory, bcm.registry, mod, m, targetImage)
	if err != nil {
		return false, fmt.Errorf("failed to check existence of image %s: %w", targetImage, err)
	}

	return !exists, nil
}

func (bcm *buildManager) Sync(
	ctx context.Context,
	mod kmmv1beta1.Module,
	m kmmv1beta1.KernelMapping,
	targetKernel string,
	pushImage bool,
	owner metav1.Object,
) (utils.Status, error) {

	logger := log.FromContext(ctx)

	buildTemplate, err := bcm.maker.MakeBuildTemplate(ctx, mod, m, targetKernel, pushImage, owner)
	if err != nil {
		return "", fmt.Errorf("could not make Build template: %v", err)
	}

	build, err := bcm.ocpBuildsHelper.GetBuild(ctx, mod, targetKernel)
	if err != nil {
		if !errors.Is(err, errNoMatchingBuild) {
			return "", fmt.Errorf("error getting the build: %v", err)
		}

		logger.Info("Creating Build")

		if err = bcm.client.Create(ctx, buildTemplate); err != nil {
			return "", fmt.Errorf("could not create Build: %v", err)
		}

		return utils.StatusCreated, nil
	}

	changed, err := bcm.isBuildChanged(build, buildTemplate)
	if err != nil {
		return "", fmt.Errorf("could not determine if Build has changed: %v", err)
	}

	if changed {
		logger.Info("The module's build spec has been changed, deleting the current Build so a new one can be created", "name", build.Name)
		opts := []client.DeleteOption{
			client.PropagationPolicy(metav1.DeletePropagationBackground),
		}
		err = bcm.client.Delete(ctx, build, opts...)
		if err != nil {
			logger.Info(utils.WarnString(fmt.Sprintf("failed to delete Build %s: %v", build.Name, err)))
		}
		return utils.StatusInProgress, nil
	}

	switch build.Status.Phase {
	case buildv1.BuildPhaseComplete:
		return utils.StatusCompleted, nil
	case buildv1.BuildPhaseNew, buildv1.BuildPhasePending, buildv1.BuildPhaseRunning:
		return utils.StatusInProgress, nil
	case buildv1.BuildPhaseFailed:
		return utils.StatusFailed, fmt.Errorf("build failed: %v", build.Status.LogSnippet)
	default:
		return "", fmt.Errorf("unknown status: %v", build.Status)
	}
}

func (bcm *buildManager) isBuildChanged(existingBuild *buildv1.Build, newBuild *buildv1.Build) (bool, error) {
	existingAnnotations := existingBuild.GetAnnotations()
	newAnnotations := newBuild.GetAnnotations()
	if existingAnnotations == nil {
		return false, fmt.Errorf("annotations are not present in the existing Build %s", existingBuild.Name)
	}
	if existingAnnotations[buildHashAnnotation] == newAnnotations[buildHashAnnotation] {
		return false, nil
	}
	return true, nil
}

//go:generate mockgen -source=manager.go -package=buildconfig -destination=mock_manager.go

type OpenShiftBuildsHelper interface {
	GetBuild(ctx context.Context, mod kmmv1beta1.Module, targetKernel string) (*buildv1.Build, error)
}

type openShiftBuildsHelper struct {
	client client.Client
}

func NewOpenShiftBuildsHelper(client client.Client) OpenShiftBuildsHelper {
	return &openShiftBuildsHelper{client: client}
}

func (osbh *openShiftBuildsHelper) GetBuild(ctx context.Context, mod kmmv1beta1.Module, targetKernel string) (*buildv1.Build, error) {
	buildList := buildv1.BuildList{}

	opts := []client.ListOption{
		client.MatchingLabels(kmmbuild.GetBuildLabels(mod, targetKernel)),
		client.InNamespace(mod.Namespace),
	}

	if err := osbh.client.List(ctx, &buildList, opts...); err != nil {
		return nil, fmt.Errorf("could not list Build: %v", err)
	}

	if n := len(buildList.Items); n == 0 {
		return nil, errNoMatchingBuild
	} else if n > 1 {
		return nil, fmt.Errorf("expected 0 or 1 Builds, got %d", n)
	}

	return &buildList.Items[0], nil
}
