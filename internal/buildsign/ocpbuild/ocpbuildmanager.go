package ocpbuild

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	buildv1 "github.com/openshift/api/build/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
)

type Status string

const (
	ocpbuildTypeBuild = "build"
	ocpbuildTypeSign  = "sign"

	StatusCompleted  Status = "completed"
	StatusCreated    Status = "created"
	StatusInProgress Status = "in progress"
	StatusFailed     Status = "failed"
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
	makeOcpbuildBuildTemplate(ctx context.Context, mld *api.ModuleLoaderData, pushImage bool, owner metav1.Object) (*buildv1.Build, error)
	makeOcpbuildSignTemplate(ctx context.Context, mld *api.ModuleLoaderData, pushImage bool, owner metav1.Object) (*buildv1.Build, error)
}

type ocpbuildManagerImpl struct {
	client             client.Client
	combiner           module.Combiner
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping
	signImage          string
	scheme             *runtime.Scheme
}

func newOCPBuildManager(client client.Client, combiner module.Combiner, kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping,
	signImage string, scheme *runtime.Scheme) ocpbuildManager {
	return &ocpbuildManagerImpl{
		client:             client,
		combiner:           combiner,
		kernelOsDtkMapping: kernelOsDtkMapping,
		signImage:          signImage,
		scheme:             scheme,
	}
}

func (omi *ocpbuildManagerImpl) isOCPBuildChanged(existingBuild *buildv1.Build, newBuild *buildv1.Build) (bool, error) {
	existingAnnotations := existingBuild.GetAnnotations()
	newAnnotations := newBuild.GetAnnotations()
	if existingAnnotations == nil {
		return false, fmt.Errorf("annotations are not present in the existing build %s", existingBuild.Name)
	}
	if existingAnnotations[constants.HashAnnotation] == newAnnotations[constants.HashAnnotation] {
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

func (omi *ocpbuildManagerImpl) makeOcpbuildBuildTemplate(ctx context.Context, mld *api.ModuleLoaderData, pushImage bool,
	owner metav1.Object) (*buildv1.Build, error) {

	containerImage := mld.ContainerImage
	// if build AND sign are specified, then we will build an intermediate image
	// and let sign produce the final image specified in spec.moduleLoader.container.km.containerImage
	if module.ShouldBeSigned(mld) {
		containerImage = module.IntermediateImageName(mld.Name, mld.Namespace, containerImage)
	}

	dockerfileData, err := omi.getDockerfileData(ctx, mld.Build, mld.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get dockerfile data from configmap: %v", err)
	}

	buildSpec, err := omi.buildSpec(mld, dockerfileData, containerImage, pushImage)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Build spec: %v", err)
	}
	sourceConfigHash, err := omi.getBuildHashAnnotationValue(ctx, dockerfileData)
	if err != nil {
		return nil, fmt.Errorf("failed to get build annotation value: %v", err)
	}

	bc := buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mld.Name + "-build-",
			Namespace:    mld.Namespace,
			Labels:       omi.ocpbuildLabels(mld.Name, mld.KernelNormalizedVersion, ocpbuildTypeBuild),
			Annotations:  omi.ocpbuildAnnotations(sourceConfigHash),
			Finalizers:   []string{constants.GCDelayFinalizer, constants.JobEventFinalizer},
		},
		Spec: *buildSpec,
	}

	if err := controllerutil.SetControllerReference(owner, &bc, omi.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return &bc, nil
}

func (omi *ocpbuildManagerImpl) makeOcpbuildSignTemplate(ctx context.Context, mld *api.ModuleLoaderData, pushImage bool,
	owner metav1.Object) (*buildv1.Build, error) {

	signConfig := mld.Sign

	var buf bytes.Buffer

	td := TemplateData{
		FilesToSign: mld.Sign.FilesToSign,
		SignImage:   omi.signImage,
	}

	imageToSign := ""
	if module.ShouldBeBuilt(mld) {
		imageToSign = module.IntermediateImageName(mld.Name, mld.Namespace, mld.ContainerImage)
	}

	if imageToSign != "" {
		td.UnsignedImage = imageToSign
	} else if signConfig.UnsignedImage != "" {
		td.UnsignedImage = signConfig.UnsignedImage
	} else {
		return nil, fmt.Errorf("no image to sign given")
	}

	if err := tmpl.Execute(&buf, td); err != nil {
		return nil, fmt.Errorf("could not render Dockerfile: %v", err)
	}
	dockerfileData := buf.String()

	spec := omi.signSpec(mld, dockerfileData, pushImage)
	buildSpecHash, err := omi.getSignHashAnnotationValue(ctx, signConfig.KeySecret.Name, signConfig.CertSecret.Name, mld.Namespace, &spec)
	if err != nil {
		return nil, fmt.Errorf("could not hash Build spec: %v", err)
	}

	build := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mld.Name + "-sign-",
			Namespace:    mld.Namespace,
			Labels:       omi.ocpbuildLabels(mld.Name, mld.KernelNormalizedVersion, ocpbuildTypeSign),
			Annotations:  omi.ocpbuildAnnotations(buildSpecHash),
			Finalizers:   []string{constants.GCDelayFinalizer, constants.JobEventFinalizer},
		},
		Spec: spec,
	}

	if err := controllerutil.SetControllerReference(owner, build, omi.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return build, nil
}
