package ocpbuild

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/hashstructure/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/buildsign"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
)

var _ = Describe("maker_makeBuildTemplate", func() {
	const (
		containerImage = "container-image"
		dockerFile     = "FROM some-image"
		moduleName     = "some-name"
		namespace      = "some-namespace"
		targetKernel   = "target-kernel"
	)

	var (
		ctrl                   *gomock.Controller
		clnt                   *client.MockClient
		maker                  maker
		mockBuildSignHelper    *buildsign.MockHelper
		mockKernelOSDTKMapping *syncronizedmap.MockKernelOsDtkMapping
		mockOCPBuildManager    *MockocpbuildManager
		ctx                    context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		mockBuildSignHelper = buildsign.NewMockHelper(ctrl)
		mockKernelOSDTKMapping = syncronizedmap.NewMockKernelOsDtkMapping(ctrl)
		mockOCPBuildManager = NewMockocpbuildManager(ctrl)
		maker = newMaker(clnt, mockBuildSignHelper, mockOCPBuildManager, mockKernelOSDTKMapping, scheme)
		ctx = context.Background()
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	dockerfileConfigMap := v1.LocalObjectReference{Name: "configMapName"}
	dockerfileCMData := map[string]string{constants.DockerfileCMKey: dockerFile}

	DescribeTable("should set fields correctly", func(
		buildSecrets []v1.LocalObjectReference,
		imagePullSecret *v1.LocalObjectReference,
		useBuildSelector bool) {
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

		irs := v1.LocalObjectReference{Name: "push-secret"}

		mld := api.ModuleLoaderData{
			Name:           moduleName,
			Namespace:      namespace,
			ContainerImage: containerImage,
			Build: &kmmv1beta1.Build{
				BuildArgs:           buildArgs,
				DockerfileConfigMap: &dockerfileConfigMap,
				Secrets:             buildSecrets,
			},
			ImageRepoSecret:         &irs,
			Selector:                nodeSelector,
			KernelVersion:           targetKernel,
			KernelNormalizedVersion: targetKernel,
			Owner: &kmmv1beta1.Module{
				ObjectMeta: metav1.ObjectMeta{
					Name:      moduleName,
					Namespace: namespace,
				},
			},
		}

		if useBuildSelector {
			mld.Selector = nil
			mld.Build.Selector = nodeSelector
		}

		overrides := []kmmv1beta1.BuildArg{
			{Name: "KERNEL_VERSION", Value: targetKernel},
			{Name: "KERNEL_FULL_VERSION", Value: targetKernel},
			{Name: "MOD_NAME", Value: moduleName},
			{Name: "MOD_NAMESPACE", Value: namespace},
		}

		expected := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: moduleName + "-build-",
				Namespace:    namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "kmm.sigs.x-k8s.io/v1beta1",
						Kind:               "Module",
						Name:               moduleName,
						Controller:         ptr.To(true),
						BlockOwnerDeletion: ptr.To(true),
					},
				},
				Finalizers: []string{constants.GCDelayFinalizer, constants.JobEventFinalizer},
			},
			Spec: buildv1.BuildSpec{
				CommonSpec: buildv1.CommonSpec{
					ServiceAccount: "builder",
					Source: buildv1.BuildSource{
						Dockerfile: ptr.To(dockerFile),
						Type:       buildv1.BuildSourceDockerfile,
					},
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.DockerBuildStrategyType,
						DockerStrategy: &buildv1.DockerBuildStrategy{
							BuildArgs: append(
								envVarsFromKMMBuildArgs(buildArgs),
								v1.EnvVar{Name: "KERNEL_VERSION", Value: targetKernel},
								v1.EnvVar{Name: "KERNEL_FULL_VERSION", Value: targetKernel},
								v1.EnvVar{Name: "MOD_NAME", Value: moduleName},
								v1.EnvVar{Name: "MOD_NAMESPACE", Value: namespace},
							),
							Volumes:    buildVolumesFromBuildSecrets(buildSecrets),
							PullSecret: &irs,
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
					MountTrustedCA: ptr.To(true),
				},
			},
		}

		if imagePullSecret != nil {
			mld.ImageRepoSecret = imagePullSecret
			expected.Spec.CommonSpec.Output.PushSecret = imagePullSecret
			expected.Spec.Strategy.DockerStrategy.PullSecret = imagePullSecret
		}

		if len(buildSecrets) > 0 {

			mld.Build.Secrets = buildSecrets

			expected.Spec.CommonSpec.Strategy.DockerStrategy.Volumes = buildVolumesFromBuildSecrets(buildSecrets)
		}

		hash, err := hashstructure.Hash(expected.Spec.CommonSpec.Source, hashstructure.FormatV2, nil)
		Expect(err).NotTo(HaveOccurred())
		annotations := map[string]string{hashAnnotation: fmt.Sprintf("%d", hash)}
		expected.SetAnnotations(annotations)
		labels := map[string]string{"some label": "some value"}
		expected.SetLabels(labels)

		gomock.InOrder(
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: dockerfileConfigMap.Name, Namespace: mld.Namespace}, gomock.Any()).DoAndReturn(
				func(_ interface{}, _ interface{}, cm *v1.ConfigMap, _ ...ctrlclient.GetOption) error {
					cm.Data = dockerfileCMData
					return nil
				},
			),
			mockBuildSignHelper.EXPECT().ApplyBuildArgOverrides(buildArgs, overrides).Return(
				append(buildArgs,
					kmmv1beta1.BuildArg{Name: "KERNEL_VERSION", Value: targetKernel},
					kmmv1beta1.BuildArg{Name: "KERNEL_FULL_VERSION", Value: targetKernel},
					kmmv1beta1.BuildArg{Name: "MOD_NAME", Value: moduleName},
					kmmv1beta1.BuildArg{Name: "MOD_NAMESPACE", Value: namespace}),
			),
			mockOCPBuildManager.EXPECT().ocpbuildLabels(moduleName, targetKernel, ocpbuildTypeBuild).Return(labels),
			mockOCPBuildManager.EXPECT().ocpbuildAnnotations(hash).Return(annotations),
		)

		bc, err := maker.makeBuildTemplate(ctx, &mld, true, mld.Owner)
		Expect(err).NotTo(HaveOccurred())

		Expect(
			cmp.Diff(&expected, bc),
		).To(
			BeEmpty(),
		)
	},
		Entry(
			"no secrets at all",
			[]v1.LocalObjectReference{},
			nil,
			false,
		),
		Entry(
			"no secrets at all with build.Selector property",
			[]v1.LocalObjectReference{},
			nil,
			true,
		),
		Entry(
			"only buildSecrets",
			[]v1.LocalObjectReference{{Name: "s1"}},
			nil,
			false,
		),
		Entry(
			"only imagePullSecrets",
			[]v1.LocalObjectReference{},
			&v1.LocalObjectReference{Name: "pull-push-secret"},
			false,
		),
		Entry(
			"buildSecrets and imagePullSecrets",
			[]v1.LocalObjectReference{{Name: "s1"}},
			&v1.LocalObjectReference{Name: "pull-push-secret"},
			false,
		),
	)

	Context(fmt.Sprintf("using %s", dtkBuildArg), func() {
		It("should fail if we couldn't get the DTK image", func() {

			gomock.InOrder(
				clnt.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ interface{}, _ interface{}, cm *v1.ConfigMap, _ ...ctrlclient.GetOption) error {
						dockerfileData := fmt.Sprintf("FROM %s", dtkBuildArg)
						cm.Data = map[string]string{constants.DockerfileCMKey: dockerfileData}
						return nil
					},
				),
				mockKernelOSDTKMapping.EXPECT().GetImage(gomock.Any()).Return("", errors.New("random error")),
			)

			mld := api.ModuleLoaderData{
				Build: &kmmv1beta1.Build{
					DockerfileConfigMap: &dockerfileConfigMap,
				},
			}
			_, err := maker.makeBuildTemplate(ctx, &mld, false, mld.Owner)
			Expect(err).To(HaveOccurred())
		})

		It(fmt.Sprintf("should add a build arg if %s is used in the Dockerfile", dtkBuildArg), func() {

			const dtkImage = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:111"

			buildArgs := []kmmv1beta1.BuildArg{
				{
					Name:  dtkBuildArg,
					Value: dtkImage,
				},
			}

			gomock.InOrder(
				clnt.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ interface{}, _ interface{}, cm *v1.ConfigMap, _ ...ctrlclient.GetOption) error {
						dockerfileData := fmt.Sprintf("FROM %s", dtkBuildArg)
						cm.Data = map[string]string{constants.DockerfileCMKey: dockerfileData}
						return nil
					},
				),
				mockKernelOSDTKMapping.EXPECT().GetImage(gomock.Any()).Return(dtkImage, nil),
				mockBuildSignHelper.EXPECT().ApplyBuildArgOverrides(gomock.Any(), gomock.Any()).Return(buildArgs),
				mockOCPBuildManager.EXPECT().ocpbuildLabels(gomock.Any(), gomock.Any(), ocpbuildTypeBuild),
				mockOCPBuildManager.EXPECT().ocpbuildAnnotations(gomock.Any()),
			)

			mld := api.ModuleLoaderData{
				Build: &kmmv1beta1.Build{
					DockerfileConfigMap: &dockerfileConfigMap,
				},
				Owner: &kmmv1beta1.Module{},
			}
			bct, err := maker.makeBuildTemplate(ctx, &mld, false, mld.Owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(bct.Spec.CommonSpec.Strategy.DockerStrategy.BuildArgs)).To(Equal(1))
			Expect(bct.Spec.CommonSpec.Strategy.DockerStrategy.BuildArgs[0].Name).To(Equal(buildArgs[0].Name))
			Expect(bct.Spec.CommonSpec.Strategy.DockerStrategy.BuildArgs[0].Value).To(Equal(buildArgs[0].Value))
		})
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
						Optional:   ptr.To(false),
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
						Optional:   ptr.To(false),
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
