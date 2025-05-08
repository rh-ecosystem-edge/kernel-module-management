package ocpbuild

import (
	"context"
	"errors"
	"fmt"

	buildv1 "github.com/openshift/api/build/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/buildsign"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/kernel"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils"
)

type manager struct {
	client          client.Client
	signer          signer
	ocpbuildManager ocpbuildManager
}

func NewManager(client client.Client,
	combiner module.Combiner,
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping,
	signImage string,
	scheme *runtime.Scheme) buildsign.Manager {
	ocpbuildManager := newOCPBuildManager(client, combiner, kernelOsDtkMapping, scheme)
	signer := newSigner(client, signImage, ocpbuildManager, scheme)
	return &manager{
		client:          client,
		signer:          signer,
		ocpbuildManager: ocpbuildManager,
	}
}

func (m *manager) GetStatus(ctx context.Context, name, namespace, kernelVersion string,
	action kmmv1beta1.BuildOrSignAction, owner metav1.Object) (kmmv1beta1.BuildOrSignStatus, error) {
	ocpbuildType := ocpbuildTypeBuild
	if action == kmmv1beta1.SignImage {
		ocpbuildType = ocpbuildTypeSign
	}
	normalizedKernel := kernel.NormalizeVersion(kernelVersion)
	foundOCPBuild, err := m.ocpbuildManager.getModuleOCPBuildByKernel(ctx, name, namespace, normalizedKernel, ocpbuildType, owner)
	if err != nil {
		if !errors.Is(err, ErrNoMatchingBuild) {
			return kmmv1beta1.BuildOrSignStatus(""), fmt.Errorf("failed to get ocpbuild %s/%s, action %s: %v", namespace, name, action, err)
		}
		return kmmv1beta1.BuildOrSignStatus(""), nil
	}
	status, err := m.ocpbuildManager.getOCPBuildStatus(foundOCPBuild)
	if err != nil {
		return kmmv1beta1.BuildOrSignStatus(""), fmt.Errorf("failed to get status from the ocpbuild %s/%s, action %s: %v",
			foundOCPBuild.Namespace, foundOCPBuild.Name, action, err)
	}
	switch status {
	case StatusCompleted:
		return kmmv1beta1.ActionSuccess, nil
	case StatusFailed:
		return kmmv1beta1.ActionFailure, nil
	}

	// any other status means the pod is still not finished, returning empty status
	return kmmv1beta1.BuildOrSignStatus(""), nil
}

func (m *manager) Sync(ctx context.Context, mld *api.ModuleLoaderData, pushImage bool, action kmmv1beta1.BuildOrSignAction, owner metav1.Object) error {
	logger := log.FromContext(ctx)
	var (
		ocpbuildType     string
		ocpbuildTemplate *buildv1.Build
		err              error
	)
	switch action {
	case kmmv1beta1.BuildImage:
		logger.Info("Building in-cluster")
		ocpbuildType = ocpbuildTypeBuild
		ocpbuildTemplate, err = m.ocpbuildManager.makeOCPBuildTemplate(ctx, mld, pushImage, owner)
	case kmmv1beta1.SignImage:
		logger.Info("Signing in-cluster")
		ocpbuildType = ocpbuildTypeSign
		ocpbuildTemplate, err = m.signer.makeBuildTemplate(ctx, mld, pushImage, owner)
	default:
		return fmt.Errorf("invalid action %s", action)
	}

	if err != nil {
		return fmt.Errorf("could not make Pod template: %v", err)
	}

	b, err := m.ocpbuildManager.getModuleOCPBuildByKernel(ctx, mld.Name, mld.Namespace,
		mld.KernelNormalizedVersion, ocpbuildType, owner)

	if err != nil {
		if !errors.Is(err, ErrNoMatchingBuild) {
			return fmt.Errorf("error getting the %s ocpbuild: %v", ocpbuildType, err)
		}

		logger.Info("Creating build")
		err = m.ocpbuildManager.createOCPBuild(ctx, ocpbuildTemplate)
		if err != nil {
			return fmt.Errorf("could not create Build: %v", err)
		}

		return nil
	}

	changed, err := m.ocpbuildManager.isOCPBuildChanged(b, ocpbuildTemplate)
	if err != nil {
		return fmt.Errorf("could not determine if ocpbuild has changed: %v", err)
	}

	if changed {
		logger.Info("The module's spec has been changed, deleting the current ocpbuild so a new one can be created", "name", b.Name, "action", action)
		err = m.ocpbuildManager.deleteOCPBuild(ctx, b)
		if err != nil {
			logger.Info(utils.WarnString(fmt.Sprintf("failed to delete %s ocpbuild %s: %v", ocpbuildType, b.Name, err)))
		}
	}

	return nil
}

func (m *manager) GarbageCollect(ctx context.Context, name, namespace string, action kmmv1beta1.BuildOrSignAction, owner metav1.Object) ([]string, error) {
	ocpbuildType := ocpbuildTypeBuild
	if action == kmmv1beta1.SignImage {
		ocpbuildType = ocpbuildTypeSign
	}
	builds, err := m.ocpbuildManager.getModuleOCPBuilds(ctx, name, namespace, ocpbuildType, owner)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s buils for mbsc %s/%s: %v", ocpbuildType, namespace, name, err)
	}

	logger := log.FromContext(ctx)
	errs := make([]error, 0, len(builds))
	deleteBuildsNames := make([]string, 0, len(builds))
	for _, build := range builds {
		if build.Status.Phase == buildv1.BuildPhaseComplete {
			err = m.ocpbuildManager.deleteOCPBuild(ctx, &build)
			errs = append(errs, err)
			if err != nil {
				logger.Info(utils.WarnString("failed to delete %s build %s in garbage collection: %v"), ocpbuildType, build.Name, err)
				continue
			}
			deleteBuildsNames = append(deleteBuildsNames, build.Name)
		}
	}
	return deleteBuildsNames, errors.Join(errs...)
}
