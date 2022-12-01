package buildconfig

import (
	"context"
	"fmt"
	"strings"

	"github.com/mitchellh/hashstructure"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	kmmbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const dtkBuildArg = "DTK_AUTO"

//go:generate mockgen -source=maker.go -package=buildconfig -destination=mock_maker.go

type Maker interface {
	MakeBuildTemplate(ctx context.Context, mod kmmv1beta1.Module, mapping kmmv1beta1.KernelMapping, targetKernel, containerImage string, pushImage bool) (*buildv1.Build, error)
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
	mod kmmv1beta1.Module,
	mapping kmmv1beta1.KernelMapping,
	targetKernel,
	containerImage string,
	pushImage bool,
) (*buildv1.Build, error) {

	kmmBuild := m.helper.GetRelevantBuild(mod, mapping)

	overrides := []kmmv1beta1.BuildArg{
		{
			Name:  "KERNEL_VERSION",
			Value: targetKernel,
		},
	}

	dockerfileData, err := m.getDockerfileData(ctx, kmmBuild, mod.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get dockerfile data from configmap: %v", err)
	}

	if strings.Contains(dockerfileData, dtkBuildArg) {

		dtkImage, err := m.kernelOsDtkMapping.GetImage(targetKernel)
		if err != nil {
			return nil, fmt.Errorf("could not get DTK image for kernel %v: %v", targetKernel, err)
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
		PushSecret: mod.Spec.ImageRepoSecret,
	}
	if !pushImage {
		buildTarget = buildv1.BuildOutput{}
	}

	sourceConfig := buildv1.BuildSource{
		Dockerfile: &dockerfileData,
		Type:       buildv1.BuildSourceDockerfile,
	}

	sourceConfigHash, err := hashstructure.Hash(sourceConfig, nil)
	if err != nil {
		return nil, fmt.Errorf("could not hash Build's Buildsource template: %v", err)
	}

	bc := buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mod.Name + "-",
			Namespace:    mod.Namespace,
			Labels:       kmmbuild.GetBuildLabels(mod, targetKernel),
			Annotations:  map[string]string{buildHashAnnotation: fmt.Sprintf("%d", sourceConfigHash)},
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
				NodeSelector:   mod.Spec.Selector,
				MountTrustedCA: pointer.Bool(true),
			},
		},
	}

	if err := controllerutil.SetControllerReference(&mod, &bc, m.scheme); err != nil {
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
