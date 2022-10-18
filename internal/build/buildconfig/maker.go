package buildconfig

import (
	"fmt"

	"github.com/mitchellh/hashstructure"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//go:generate mockgen -source=maker.go -package=buildconfig -destination=mock_maker.go

type Maker interface {
	MakeBuildConfigTemplate(mod kmmv1beta1.Module, mapping kmmv1beta1.KernelMapping, targetKernel, containerImage string, pushImage bool) (*buildv1.BuildConfig, error)
}

type maker struct {
	helper build.Helper
	scheme *runtime.Scheme
}

func NewMaker(helper build.Helper, scheme *runtime.Scheme) Maker {
	return &maker{
		helper: helper,
		scheme: scheme,
	}
}

func (m *maker) MakeBuildConfigTemplate(mod kmmv1beta1.Module, mapping kmmv1beta1.KernelMapping, targetKernel, containerImage string, pushImage bool) (*buildv1.BuildConfig, error) {
	kmmBuild := m.helper.GetRelevantBuild(mod, mapping)

	buildArgs := m.helper.ApplyBuildArgOverrides(
		kmmBuild.BuildArgs,
		kmmv1beta1.BuildArg{Name: "KERNEL_VERSION", Value: targetKernel},
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
		Dockerfile: &kmmBuild.Dockerfile,
		Type:       buildv1.BuildSourceDockerfile,
	}

	sourceConfigHash, err := hashstructure.Hash(sourceConfig, nil)
	if err != nil {
		return nil, fmt.Errorf("could not hash BuildConfig's Buildsource template: %v", err)
	}

	bc := buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mod.Name + "-",
			Namespace:    mod.Namespace,
			Labels:       build.GetBuildLabels(mod, targetKernel),
			Annotations:  map[string]string{buildConfigHashAnnotation: fmt.Sprintf("%d", sourceConfigHash)},
		},
		Spec: buildv1.BuildConfigSpec{
			Triggers: []buildv1.BuildTriggerPolicy{
				{Type: buildv1.ConfigChangeBuildTriggerType},
			},
			RunPolicy: buildv1.BuildRunPolicySerialLatestOnly,
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
