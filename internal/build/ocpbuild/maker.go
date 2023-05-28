package ocpbuild

import (
	"context"
	"fmt"
	"strings"

	"github.com/mitchellh/hashstructure/v2"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	kmmbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	ocpbuildutils "github.com/rh-ecosystem-edge/kernel-module-management/internal/utils/ocpbuild"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	BuildType = "build"

	dtkBuildArg = "DTK_AUTO"
)

//go:generate mockgen -source=maker.go -package=ocpbuild -destination=mock_maker.go Maker

type Maker interface {
	MakeBuildTemplate(
		ctx context.Context,
		mld *api.ModuleLoaderData,
		pushImage bool,
		owner metav1.Object,
	) (*buildv1.Build, error)
}

type maker struct {
	client             client.Client
	helper             kmmbuild.Helper
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping
	scheme             *runtime.Scheme
}

func NewMaker(client client.Client, helper kmmbuild.Helper, scheme *runtime.Scheme, kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping) Maker {
	return &maker{
		client:             client,
		helper:             helper,
		kernelOsDtkMapping: kernelOsDtkMapping,
		scheme:             scheme,
	}
}

func (m *maker) MakeBuildTemplate(
	ctx context.Context,
	mld *api.ModuleLoaderData,
	pushImage bool,
	owner metav1.Object,
) (*buildv1.Build, error) {

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
			Name:  "MOD_NAME",
			Value: mld.Name,
		},
		{
			Name:  "MOD_NAMESPACE",
			Value: mld.Namespace,
		},
	}

	dockerfileData, err := m.getDockerfileData(ctx, kmmBuild, mld.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get dockerfile data from configmap: %v", err)
	}

	if strings.Contains(dockerfileData, dtkBuildArg) {

		dtkImage, err := m.kernelOsDtkMapping.GetImage(kernelVersion)
		if err != nil {
			return nil, fmt.Errorf("could not get DTK image for kernel %v: %v", kernelVersion, err)
		}
		overrides = append(overrides, kmmv1beta1.BuildArg{Name: dtkBuildArg, Value: dtkImage})
	}

	buildArgs := m.helper.ApplyBuildArgOverrides(
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
			Labels:       ocpbuildutils.GetOCPBuildLabels(mld, BuildType),
			Annotations:  ocpbuildutils.GetOCPBuildAnnotations(sourceConfigHash),
		},
		Spec: buildv1.BuildSpec{
			CommonSpec: buildv1.CommonSpec{
				ServiceAccount: constants.OCPBuilderServiceAccountName,
				Source:         sourceConfig,
				Strategy: buildv1.BuildStrategy{
					Type: buildv1.DockerBuildStrategyType,
					DockerStrategy: &buildv1.DockerBuildStrategy{
						BuildArgs: envVarsFromKMMBuildArgs(buildArgs),
						Volumes:   buildVolumesFromBuildSecrets(kmmBuild.Secrets),
					},
				},
				Output:         buildTarget,
				NodeSelector:   selector,
				MountTrustedCA: pointer.Bool(true),
			},
		},
	}

	if err := controllerutil.SetControllerReference(owner, &bc, m.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return &bc, nil
}

func (m *maker) getDockerfileData(ctx context.Context, buildConfig *kmmv1beta1.Build, namespace string) (string, error) {
	dockerfileCM := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{Name: buildConfig.DockerfileConfigMap.Name, Namespace: namespace}
	err := m.client.Get(ctx, namespacedName, dockerfileCM)
	if err != nil {
		return "", fmt.Errorf("failed to get dockerfile ConfigMap %s: %v", namespacedName, err)
	}
	data, ok := dockerfileCM.Data[constants.DockerfileCMKey]
	if !ok {
		return "", fmt.Errorf("invalid Dockerfile ConfigMap %s format, %s key is missing", namespacedName, constants.DockerfileCMKey)
	}
	return data, nil
}

func envVarsFromKMMBuildArgs(args []kmmv1beta1.BuildArg) []v1.EnvVar {
	if args == nil {
		return nil
	}

	ev := make([]v1.EnvVar, 0, len(args))

	for _, ba := range args {
		ev = append(ev, v1.EnvVar{Name: ba.Name, Value: ba.Value})
	}

	return ev
}

func buildVolumesFromBuildSecrets(secrets []v1.LocalObjectReference) []buildv1.BuildVolume {
	if secrets == nil {
		return nil
	}

	vols := make([]buildv1.BuildVolume, 0, len(secrets))

	for _, s := range secrets {
		bv := buildv1.BuildVolume{
			Name: "secret-" + s.Name,
			Source: buildv1.BuildVolumeSource{
				Type: buildv1.BuildVolumeSourceTypeSecret,
				Secret: &v1.SecretVolumeSource{
					SecretName: s.Name,
					Optional:   pointer.Bool(false),
				},
			},
			Mounts: []buildv1.BuildVolumeMount{
				{DestinationPath: "/run/secrets/" + s.Name},
			},
		}

		vols = append(vols, bv)
	}

	return vols
}
