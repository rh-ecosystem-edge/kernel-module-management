package ocpbuild

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mitchellh/hashstructure/v2"
	buildv1 "github.com/openshift/api/build/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
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
func (omi *ocpbuildManagerImpl) makeOcpbuildBuildTemplate(ctx context.Context, mld *api.ModuleLoaderData, pushImage bool,
	owner metav1.Object) (*buildv1.Build, error) {

	kmmBuild := mld.Build
	containerImage := mld.ContainerImage
	kernelVersion := mld.KernelVersion

	// if build AND sign are specified, then we will build an intermediate image
	// and let sign produce the final image specified in spec.moduleLoader.container.km.containerImage
	if module.ShouldBeSigned(mld) {
		containerImage = module.IntermediateImageName(mld.Name, mld.Namespace, containerImage)
	}

	overrides := []kmmv1beta1.BuildArg{
		{
			Name:  "KERNEL_VERSION",
			Value: kernelVersion,
		},
		{
			Name:  "KERNEL_FULL_VERSION",
			Value: kernelVersion,
		},
		{
			Name:  "MOD_NAME",
			Value: mld.Name,
		},
		{
			Name:  "MOD_NAMESPACE",
			Value: mld.Namespace,
		},
	}

	dockerfileData, err := omi.getDockerfileData(ctx, kmmBuild, mld.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get dockerfile data from configmap: %v", err)
	}

	if strings.Contains(dockerfileData, dtkBuildArg) {

		dtkImage, err := omi.kernelOsDtkMapping.GetImage(kernelVersion)
		if err != nil {
			return nil, fmt.Errorf("could not get DTK image for kernel %v: %v", kernelVersion, err)
		}
		overrides = append(overrides, kmmv1beta1.BuildArg{Name: dtkBuildArg, Value: dtkImage})
	}

	buildArgs := omi.combiner.ApplyBuildArgOverrides(
		kmmBuild.BuildArgs,
		overrides...,
	)

	buildTarget := buildv1.BuildOutput{
		To: &v1.ObjectReference{
			Kind: "DockerImage",
			Name: containerImage,
		},
		PushSecret: mld.ImageRepoSecret,
	}
	if !pushImage {
		buildTarget = buildv1.BuildOutput{}
	}

	sourceConfig := buildv1.BuildSource{
		Dockerfile: &dockerfileData,
		Type:       buildv1.BuildSourceDockerfile,
	}

	sourceConfigHash, err := hashstructure.Hash(sourceConfig, hashstructure.FormatV2, nil)
	if err != nil {
		return nil, fmt.Errorf("could not hash Build's Buildsource template: %v", err)
	}

	selector := mld.Selector
	if len(mld.Build.Selector) != 0 {
		selector = mld.Build.Selector
	}

	bc := buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mld.Name + "-build-",
			Namespace:    mld.Namespace,
			Labels:       omi.ocpbuildLabels(mld.Name, mld.KernelNormalizedVersion, ocpbuildTypeBuild),
			Annotations:  omi.ocpbuildAnnotations(sourceConfigHash),
			Finalizers:   []string{constants.GCDelayFinalizer, constants.JobEventFinalizer},
		},
		Spec: buildv1.BuildSpec{
			CommonSpec: buildv1.CommonSpec{
				ServiceAccount: constants.OCPBuilderServiceAccountName,
				Source:         sourceConfig,
				Strategy: buildv1.BuildStrategy{
					Type: buildv1.DockerBuildStrategyType,
					DockerStrategy: &buildv1.DockerBuildStrategy{
						BuildArgs:  envVarsFromKMMBuildArgs(buildArgs),
						Volumes:    buildVolumesFromBuildSecrets(kmmBuild.Secrets),
						PullSecret: mld.ImageRepoSecret,
					},
				},
				Output:         buildTarget,
				NodeSelector:   selector,
				MountTrustedCA: ptr.To(true),
			},
		},
	}

	if err := controllerutil.SetControllerReference(owner, &bc, omi.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return &bc, nil
}

func (omi *ocpbuildManagerImpl) getDockerfileData(ctx context.Context, buildConfig *kmmv1beta1.Build, namespace string) (string, error) {
	dockerfileCM := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{Name: buildConfig.DockerfileConfigMap.Name, Namespace: namespace}
	err := omi.client.Get(ctx, namespacedName, dockerfileCM)
	if err != nil {
		return "", fmt.Errorf("failed to get dockerfile ConfigMap %s: %v", namespacedName, err)
	}
	data, ok := dockerfileCM.Data[constants.DockerfileCMKey]
	if !ok {
		return "", fmt.Errorf("invalid Dockerfile ConfigMap %s format, %s key is missing", namespacedName, constants.DockerfileCMKey)
	}
	return data, nil
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

	dockerfile := buf.String()

	buildTarget := buildv1.BuildOutput{
		To: &v1.ObjectReference{
			Kind: "DockerImage",
			Name: mld.ContainerImage,
		},
		PushSecret: mld.ImageRepoSecret,
	}
	if !pushImage {
		buildTarget = buildv1.BuildOutput{}
	}

	sourceConfig := buildv1.BuildSource{
		Dockerfile: &dockerfile,
		Type:       buildv1.BuildSourceDockerfile,
	}

	spec := buildv1.BuildSpec{
		CommonSpec: buildv1.CommonSpec{
			ServiceAccount: constants.OCPBuilderServiceAccountName,
			Source:         sourceConfig,
			Strategy: buildv1.BuildStrategy{
				Type: buildv1.DockerBuildStrategyType,
				DockerStrategy: &buildv1.DockerBuildStrategy{
					Volumes: []buildv1.BuildVolume{
						{
							Name: "key",
							Source: buildv1.BuildVolumeSource{
								Type: buildv1.BuildVolumeSourceTypeSecret,
								Secret: &v1.SecretVolumeSource{
									SecretName: signConfig.KeySecret.Name,
									Optional:   ptr.To(false),
								},
							},
							Mounts: []buildv1.BuildVolumeMount{
								{DestinationPath: "/run/secrets/key"},
							},
						},
						{
							Name: "cert",
							Source: buildv1.BuildVolumeSource{
								Type: buildv1.BuildVolumeSourceTypeSecret,
								Secret: &v1.SecretVolumeSource{
									SecretName: signConfig.CertSecret.Name,
									Optional:   ptr.To(false),
								},
							},
							Mounts: []buildv1.BuildVolumeMount{
								{DestinationPath: "/run/secrets/cert"},
							},
						},
					},
				},
			},
			Output:         buildTarget,
			NodeSelector:   mld.Selector,
			MountTrustedCA: ptr.To(true),
		},
	}

	hash, err := omi.hash(ctx, &spec, mld.Namespace, signConfig.KeySecret.Name, signConfig.CertSecret.Name)
	if err != nil {
		return nil, fmt.Errorf("could not hash Build spec: %v", err)
	}

	build := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mld.Name + "-sign-",
			Namespace:    mld.Namespace,
			Labels:       omi.ocpbuildLabels(mld.Name, mld.KernelNormalizedVersion, ocpbuildTypeSign),
			Annotations:  omi.ocpbuildAnnotations(hash),
			Finalizers:   []string{constants.GCDelayFinalizer, constants.JobEventFinalizer},
		},
		Spec: spec,
	}

	if err := controllerutil.SetControllerReference(owner, build, omi.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return build, nil
}
