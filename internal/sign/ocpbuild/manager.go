package ocpbuild

import (
	"context"
	"errors"
	"fmt"
	"time"

	buildv1 "github.com/openshift/api/build/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/sign"
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
	registry registry.Registry) sign.SignManager {
	return &manager{
		client:          client,
		maker:           maker,
		ocpBuildsHelper: ocpBuildsHelper,
		authFactory:     authFactory,
		registry:        registry,
	}
}

func (m *manager) GarbageCollect(ctx context.Context, modName, namespace string, owner metav1.Object, delay time.Duration) ([]string, error) {
	moduleSigns, err := m.ocpBuildsHelper.GetModuleOCPBuilds(ctx, modName, namespace, owner)
	if err != nil {
		return nil, fmt.Errorf("failed to get OCP builds for module's signs %s: %v", modName, err)
	}

	deleteNames := make([]string, 0, len(moduleSigns))
	for _, moduleSign := range moduleSigns {
		if moduleSign.Status.Phase == buildv1.BuildPhaseComplete {
			if moduleSign.DeletionTimestamp == nil {
				if err = m.ocpBuildsHelper.DeleteOCPBuild(ctx, &moduleSign); err != nil {
					return nil, fmt.Errorf("failed to delete signing pod %s: %v", moduleSign.Name, err)
				}
			}

			if moduleSign.DeletionTimestamp.Add(delay).Before(time.Now()) {
				if err = m.ocpBuildsHelper.RemoveFinalizer(ctx, &moduleSign, constants.GCDelayFinalizer); err != nil {
					return nil, fmt.Errorf("could not remove the GC delay finalizer from pod %s/%s: %v", moduleSign.Namespace, moduleSign.Name, err)
				}

				deleteNames = append(deleteNames, moduleSign.Name)
			}
		}
	}
	return deleteNames, nil
}

func (m *manager) ShouldSync(ctx context.Context, mld *api.ModuleLoaderData) (bool, error) {
	// if there is no sign specified skip
	if !module.ShouldBeSigned(mld) {
		return false, nil
	}

	exists, err := module.ImageExists(ctx, m.authFactory, m.registry, mld, mld.ContainerImage)
	if err != nil {
		return false, fmt.Errorf("failed to check existence of image %s: %w", mld.ContainerImage, err)
	}

	return !exists, nil
}

func (m *manager) Sync(
	ctx context.Context,
	mld *api.ModuleLoaderData,
	imageToSign string,
	pushImage bool,
	owner metav1.Object,
) (ocpbuildutils.Status, error) {

	logger := log.FromContext(ctx)

	buildTemplate, err := m.maker.MakeBuildTemplate(ctx, mld, imageToSign, pushImage, owner)
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
		opts := []client.DeleteOption{
			client.PropagationPolicy(metav1.DeletePropagationBackground),
		}
		err = m.client.Delete(ctx, build, opts...)
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
