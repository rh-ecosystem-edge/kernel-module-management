package buildconfig

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/hashstructure"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("Maker_MakeBuildConfigTemplate", func() {
	It("should work as expected", func() {
		const (
			containerImage = "container-image"
			dockerFile     = "FROM some-image"
			moduleName     = "some-name"
			namespace      = "some-namespace"
			targetKernel   = "target-kernel"
		)

		nodeSelector := map[string]string{"label-key": "label-value"}

		buildArgs := []kmmv1beta1.BuildArg{
			{
				Name:  "arg-1",
				Value: "value-1",
			},
			{
				Name:  "arg-2",
				Value: "value-2",
			},
		}

		buildSecrets := []v1.LocalObjectReference{
			{Name: "secret-1"},
			{Name: "secret-2"},
		}

		irs := v1.LocalObjectReference{Name: "push-secret"}

		mapping := kmmv1beta1.KernelMapping{
			ContainerImage: containerImage,
			Build: &kmmv1beta1.Build{
				BuildArgs:  buildArgs,
				Dockerfile: dockerFile,
				Secrets:    buildSecrets,
			},
		}

		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      moduleName,
				Namespace: namespace,
			},
			Spec: kmmv1beta1.ModuleSpec{
				ModuleLoader: kmmv1beta1.ModuleLoaderSpec{
					Container: kmmv1beta1.ModuleLoaderContainerSpec{
						KernelMappings: []kmmv1beta1.KernelMapping{mapping},
					},
				},
				ImageRepoSecret: &irs,
				Selector:        nodeSelector,
			},
		}

		expected := buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: moduleName + "-",
				Namespace:    namespace,
				Labels: map[string]string{
					constants.ModuleNameLabel:    moduleName,
					constants.TargetKernelTarget: targetKernel,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "kmm.sigs.k8s.io/v1beta1",
						Kind:               "Module",
						Name:               moduleName,
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				},
			},
			Spec: buildv1.BuildConfigSpec{
				Triggers: []buildv1.BuildTriggerPolicy{
					{Type: buildv1.ConfigChangeBuildTriggerType},
				},
				RunPolicy: buildv1.BuildRunPolicySerialLatestOnly,
				CommonSpec: buildv1.CommonSpec{
					ServiceAccount: "builder",
					Source: buildv1.BuildSource{
						Dockerfile: pointer.String(dockerFile),
						Type:       buildv1.BuildSourceDockerfile,
					},
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.DockerBuildStrategyType,
						DockerStrategy: &buildv1.DockerBuildStrategy{
							BuildArgs: append(
								envVarsFromKMMBuildArgs(buildArgs),
								v1.EnvVar{Name: "KERNEL_VERSION", Value: targetKernel},
							),
							Volumes: buildVolumesFromBuildSecrets(buildSecrets),
						},
					},
					Output: buildv1.BuildOutput{
						To: &v1.ObjectReference{
							Kind: "DockerImage",
							Name: containerImage,
						},
						PushSecret: &irs,
					},
					NodeSelector:   nodeSelector,
					MountTrustedCA: pointer.Bool(true),
				},
			},
		}

		hash, err := hashstructure.Hash(expected.Spec.CommonSpec.Source, nil)
		Expect(err).NotTo(HaveOccurred())
		annotations := map[string]string{buildConfigHashAnnotation: fmt.Sprintf("%d", hash)}
		expected.SetAnnotations(annotations)

		bc, err := NewMaker(build.NewHelper(), scheme).MakeBuildConfigTemplate(mod, mapping, targetKernel, containerImage, true)
		Expect(err).NotTo(HaveOccurred())

		Expect(
			cmp.Diff(&expected, bc),
		).To(
			BeEmpty(),
		)
	})
})

var _ = Describe("envVarsFromKMMBuildArgs", func() {
	It("should return nil if args is nil", func() {
		Expect(envVarsFromKMMBuildArgs(nil)).To(BeNil())
	})

	It("should work as expected", func() {
		args := []kmmv1beta1.BuildArg{
			{Name: "arg1", Value: "value1"},
			{Name: "arg2", Value: "value2"},
		}

		expected := []v1.EnvVar{
			{Name: "arg1", Value: "value1"},
			{Name: "arg2", Value: "value2"},
		}

		Expect(envVarsFromKMMBuildArgs(args)).To(Equal(expected))
	})
})

var _ = Describe("buildVolumesFromBuildSecrets", func() {
	It("should return nil if secrets is nil", func() {
		Expect(buildVolumesFromBuildSecrets(nil)).To(BeNil())
	})

	It("should work as expected", func() {
		secrets := []v1.LocalObjectReference{
			{Name: "secret-1"},
			{Name: "secret-2"},
		}

		expectedVolumes := []buildv1.BuildVolume{
			{
				Name: "secret-secret-1",
				Source: buildv1.BuildVolumeSource{
					Type: buildv1.BuildVolumeSourceTypeSecret,
					Secret: &v1.SecretVolumeSource{
						SecretName: "secret-1",
						Optional:   pointer.Bool(false),
					},
				},
				Mounts: []buildv1.BuildVolumeMount{
					{DestinationPath: "/run/secrets/secret-1"},
				},
			},
			{
				Name: "secret-secret-2",
				Source: buildv1.BuildVolumeSource{
					Type: buildv1.BuildVolumeSourceTypeSecret,
					Secret: &v1.SecretVolumeSource{
						SecretName: "secret-2",
						Optional:   pointer.Bool(false),
					},
				},
				Mounts: []buildv1.BuildVolumeMount{
					{DestinationPath: "/run/secrets/secret-2"},
				},
			},
		}

		Expect(buildVolumesFromBuildSecrets(secrets)).To(Equal(expectedVolumes))
	})
})
