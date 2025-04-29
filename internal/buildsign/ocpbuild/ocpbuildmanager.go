package ocpbuild

import (
	"context"
	"errors"
	"fmt"

	buildv1 "github.com/openshift/api/build/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
)

type Status string

const (
	ocpbuildTypeBuild = "build"
	ocpbuildTypeSign  = "sign"

	StatusCompleted  Status = "completed"
	StatusCreated    Status = "created"
	StatusInProgress Status = "in progress"
	StatusFailed     Status = "failed"

	hashAnnotation = "kmm.node.kubernetes.io/last-hash"
)

var ErrNoMatchingBuild = errors.New("no matching Build")

//go:generate mockgen -source=ocpbuildmanager.go -package=ocpbuild -destination=mock_ocbuildmanager.go

type ocpbuildManager interface {
	isOCPBuildChanged(existingBuild *buildv1.Build, newBuild *buildv1.Build) (bool, error)
	getModuleOCPBuildByKernel(ctx context.Context, modName, namespace, kernelVersion, ocbuildType string, owner metav1.Object) (*buildv1.Build, error)
	getModuleOCPBuilds(ctx context.Context, modName, modNamespace, ocbuildType string, owner metav1.Object) ([]buildv1.Build, error)
	getOCPBuildStatus(build *buildv1.Build) (Status, error)
	createOCPBuild(ctx context.Context, build *buildv1.Build) error
	deleteOCPBuild(ctx context.Context, build *buildv1.Build) error
	ocpbuildLabels(modName, kernelVersion, ocpbuildType string) map[string]string
	ocpbuildAnnotations(hash uint64) map[string]string
}

type ocpbuildManagerImpl struct {
	client client.Client
}

func newOCPBuildManager(client client.Client) ocpbuildManager {
	return &ocpbuildManagerImpl{
		client: client,
	}
}

func (omi *ocpbuildManagerImpl) isOCPBuildChanged(existingBuild *buildv1.Build, newBuild *buildv1.Build) (bool, error) {
	existingAnnotations := existingBuild.GetAnnotations()
	newAnnotations := newBuild.GetAnnotations()
	if existingAnnotations == nil {
		return false, fmt.Errorf("annotations are not present in the existing build %s", existingBuild.Name)
	}
	if existingAnnotations[hashAnnotation] == newAnnotations[hashAnnotation] {
		return false, nil
	}
	return true, nil
}

func (omi *ocpbuildManagerImpl) getModuleOCPBuildByKernel(ctx context.Context,
	modName,
	modNamespace,
	kernelVersion,
	ocbuildType string,
	owner metav1.Object) (*buildv1.Build, error) {
	buildList := buildv1.BuildList{}

	opts := []client.ListOption{
		client.MatchingLabels(omi.ocpbuildLabels(modName, kernelVersion, ocbuildType)),
		client.InNamespace(modNamespace),
	}

	if err := omi.client.List(ctx, &buildList, opts...); err != nil {
		return nil, fmt.Errorf("could not list build: %v", err)
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

func (omi *ocpbuildManagerImpl) getModuleOCPBuilds(ctx context.Context, modName, modNamespace, ocpbuildType string, owner metav1.Object) ([]buildv1.Build, error) {
	buildList := buildv1.BuildList{}

	opts := []client.ListOption{
		client.MatchingLabels(moduleLabels(modName, ocpbuildType)),
		client.InNamespace(modNamespace),
	}

	if err := omi.client.List(ctx, &buildList, opts...); err != nil {
		return nil, fmt.Errorf("could not list build: %v", err)
	}

	// filter OCP builds by owner, since they could have been created by the preflight
	// when checking that specific module
	moduleOwnedBuilds := filterOCPBuildsByOwner(buildList.Items, owner)

	return moduleOwnedBuilds, nil
}

func (omi *ocpbuildManagerImpl) getOCPBuildStatus(build *buildv1.Build) (Status, error) {
	switch build.Status.Phase {
	case buildv1.BuildPhaseComplete:
		return StatusCompleted, nil
	case buildv1.BuildPhaseRunning, buildv1.BuildPhasePending:
		return StatusInProgress, nil
	case buildv1.BuildPhaseFailed, buildv1.BuildPhaseCancelled, buildv1.BuildPhaseError:
		return StatusFailed, nil
	default:
		return "", fmt.Errorf("unknown status: %v", build.Status.Phase)
	}
}

func (omi *ocpbuildManagerImpl) createOCPBuild(ctx context.Context, build *buildv1.Build) error {
	err := omi.client.Create(ctx, build)
	if err != nil {
		return err
	}
	return nil
}

func (omi *ocpbuildManagerImpl) deleteOCPBuild(ctx context.Context, build *buildv1.Build) error {
	opts := []client.DeleteOption{
		client.PropagationPolicy(metav1.DeletePropagationBackground),
	}
	return omi.client.Delete(ctx, build, opts...)
}

func (omi *ocpbuildManagerImpl) ocpbuildLabels(modName, kernelVersion, ocpbuildType string) map[string]string {
	labels := moduleKernelLabels(modName, kernelVersion, ocpbuildType)

	labels["app.kubernetes.io/name"] = "kmm"
	labels["app.kubernetes.io/component"] = ocpbuildType
	labels["app.kubernetes.io/part-of"] = "kmm"

	return labels
}

func (omi *ocpbuildManagerImpl) ocpbuildAnnotations(hash uint64) map[string]string {
	return map[string]string{hashAnnotation: fmt.Sprintf("%d", hash)}
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

func moduleKernelLabels(modName, kernelVersion, ocpbuildType string) map[string]string {
	labels := moduleLabels(modName, ocpbuildType)
	labels[constants.TargetKernelTarget] = kernelVersion
	return labels
}

func moduleLabels(modName, ocpbuildType string) map[string]string {
	return map[string]string{
		constants.ModuleNameLabel: modName,
		constants.BuildTypeLabel:  ocpbuildType,
	}
}
