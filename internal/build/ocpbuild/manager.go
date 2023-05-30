package ocpbuild

import (
	"context"
	"errors"
	"fmt"

	buildv1 "github.com/openshift/api/build/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	ocpbuildutils "github.com/rh-ecosystem-edge/kernel-module-management/internal/utils/ocpbuild"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils"
)

type manager struct {
	client          client.Client
	maker           Maker
	ocpBuildsHelper ocpbuildutils.OCPBuildsHelper
	authFactory     auth.RegistryAuthGetterFactory
	registry        registry.Registry
}

func NewManager(
	client client.Client,
	maker Maker,
	ocpBuildsHelper ocpbuildutils.OCPBuildsHelper,
	authFactory auth.RegistryAuthGetterFactory,
	registry registry.Registry) build.Manager {
	return &manager{
		client:          client,
		maker:           maker,
		ocpBuildsHelper: ocpBuildsHelper,
		authFactory:     authFactory,
		registry:        registry,
	}
}

func (m *manager) GarbageCollect(ctx context.Context, modName, namespace string, owner metav1.Object) ([]string, error) {
	moduleBuilds, err := m.ocpBuildsHelper.GetModuleOCPBuilds(ctx, modName, namespace, owner)
	if err != nil {
		return nil, fmt.Errorf("failed to get OCP builds for module's builds %s: %v", modName, err)
	}

	deleteNames := make([]string, 0, len(moduleBuilds))
	for _, moduleBuild := range moduleBuilds {
		if moduleBuild.Status.Phase == buildv1.BuildPhaseComplete {
			err = m.ocpBuildsHelper.DeleteOCPBuild(ctx, &moduleBuild)
			if err != nil {
				return nil, fmt.Errorf("failed to delete OCP build %s: %v", moduleBuild.Name, err)
			}
			deleteNames = append(deleteNames, moduleBuild.Name)
		}
	}
	return deleteNames, nil
}

func (m *manager) ShouldSync(ctx context.Context, mld *api.ModuleLoaderData) (bool, error) {
	// if there is no build specified skip
	if !module.ShouldBeBuilt(mld) {
		return false, nil
	}

	targetImage := mld.ContainerImage

	// if build AND sign are specified, then we will build an intermediate image
	// and let sign produce the one specified in targetImage
	if module.ShouldBeSigned(mld) {
		targetImage = module.IntermediateImageName(mld.Name, mld.Namespace, targetImage)
	}

	// build is specified and targetImage is either the final image or the intermediate image
	// tag, depending on whether sign is specified or not. Either way, if targetImage exists
	// we can skip building it
	exists, err := module.ImageExists(ctx, m.authFactory, m.registry, mld, targetImage)
	if err != nil {
		return false, fmt.Errorf("failed to check existence of image %s: %w", targetImage, err)
	}

	return !exists, nil
}

func (m *manager) Sync(
	ctx context.Context,
	mld *api.ModuleLoaderData,
	pushImage bool,
	owner metav1.Object,
) (ocpbuildutils.Status, error) {

	logger := log.FromContext(ctx)

	buildTemplate, err := m.maker.MakeBuildTemplate(ctx, mld, pushImage, owner)
	if err != nil {
		return "", fmt.Errorf("could not make Build template: %v", err)
	}

	build, err := m.ocpBuildsHelper.GetModuleOCPBuildByKernel(ctx, mld, owner)
	if err != nil {
		if !errors.Is(err, ocpbuildutils.ErrNoMatchingBuild) {
			return "", fmt.Errorf("error getting the build: %v", err)
		}

		logger.Info("Creating Build")

		if err = m.client.Create(ctx, buildTemplate); err != nil {
			return "", fmt.Errorf("could not create Build: %v", err)
		}

		return ocpbuildutils.StatusCreated, nil
	}

	changed, err := ocpbuildutils.IsOCPBuildChanged(build, buildTemplate)
	if err != nil {
		return "", fmt.Errorf("could not determine if Build has changed: %v", err)
	}

	if changed {
		logger.Info("The module's build spec has been changed, deleting the current Build so a new one can be created", "name", build.Name)
		err = m.ocpBuildsHelper.DeleteOCPBuild(ctx, build)
		if err != nil {
			logger.Info(utils.WarnString(fmt.Sprintf("failed to delete Build %s: %v", build.Name, err)))
		}
		return ocpbuildutils.StatusInProgress, nil
	}

	switch build.Status.Phase {
	case buildv1.BuildPhaseComplete:
		return ocpbuildutils.StatusCompleted, nil
	case buildv1.BuildPhaseNew, buildv1.BuildPhasePending, buildv1.BuildPhaseRunning:
		return ocpbuildutils.StatusInProgress, nil
	case buildv1.BuildPhaseFailed:
		return ocpbuildutils.StatusFailed, fmt.Errorf("build failed: %v", build.Status.LogSnippet)
	default:
		return "", fmt.Errorf("unknown status: %v", build.Status)
	}
}
