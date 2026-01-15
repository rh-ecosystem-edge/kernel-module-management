package resource

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/hashstructure/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("makeBuildTemplate", func() {
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
		mbao                   *module.MockBuildArgOverrider
		mockKernelOSDTKMapping *syncronizedmap.MockKernelOsDtkMapping
		ctx                    context.Context
		rm                     *resourceManager
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		mbao = module.NewMockBuildArgOverrider(ctrl)
		mockKernelOSDTKMapping = syncronizedmap.NewMockKernelOsDtkMapping(ctrl)
		ctx = context.Background()
		rm = &resourceManager{
			client:             clnt,
			buildArgOverrider:  mbao,
			kernelOsDtkMapping: mockKernelOSDTKMapping,
			scheme:             scheme,
		}
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
				Labels:     resourceLabels(mld.Name, mld.KernelNormalizedVersion, kmmv1beta1.BuildImage),
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
							Volumes:    makeBuildResourceVolumes(mld.Build),
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

			expected.Spec.CommonSpec.Strategy.DockerStrategy.Volumes = makeBuildResourceVolumes(mld.Build)
		}

		hash, err := hashstructure.Hash(expected.Spec.CommonSpec.Source, hashstructure.FormatV2, nil)
		Expect(err).NotTo(HaveOccurred())

		annotations := map[string]string{constants.ResourceHashAnnotation: fmt.Sprintf("%d", hash)}
		expected.SetAnnotations(annotations)

		gomock.InOrder(
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: dockerfileConfigMap.Name, Namespace: mld.Namespace}, gomock.Any()).DoAndReturn(
				func(_ interface{}, _ interface{}, cm *v1.ConfigMap, _ ...ctrlclient.GetOption) error {
					cm.Data = dockerfileCMData
					return nil
				},
			),
			mbao.EXPECT().ApplyBuildArgOverrides(buildArgs, overrides).Return(
				append(buildArgs,
					kmmv1beta1.BuildArg{Name: "KERNEL_VERSION", Value: targetKernel},
					kmmv1beta1.BuildArg{Name: "KERNEL_FULL_VERSION", Value: targetKernel},
					kmmv1beta1.BuildArg{Name: "MOD_NAME", Value: moduleName},
					kmmv1beta1.BuildArg{Name: "MOD_NAMESPACE", Value: namespace}),
			),
		)

		bc, err := rm.makeBuildTemplate(ctx, &mld, mld.Owner, true)
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
			"only buidSecrets",
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
			_, err := rm.makeBuildTemplate(ctx, &mld, mld.Owner, false)
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
				mbao.EXPECT().ApplyBuildArgOverrides(gomock.Any(), gomock.Any()).Return(buildArgs),
			)

			mld := api.ModuleLoaderData{
				Build: &kmmv1beta1.Build{
					DockerfileConfigMap: &dockerfileConfigMap,
				},
				Owner: &kmmv1beta1.Module{},
			}
			buildObj, err := rm.makeBuildTemplate(ctx, &mld, mld.Owner, false)
			Expect(err).NotTo(HaveOccurred())
			bct, ok := buildObj.(*buildv1.Build)
			Expect(ok).To(BeTrue())
			Expect(len(bct.Spec.CommonSpec.Strategy.DockerStrategy.BuildArgs)).To(Equal(1))
			Expect(bct.Spec.CommonSpec.Strategy.DockerStrategy.BuildArgs[0].Name).To(Equal(buildArgs[0].Name))
			Expect(bct.Spec.CommonSpec.Strategy.DockerStrategy.BuildArgs[0].Value).To(Equal(buildArgs[0].Value))
		})
	})
})

var _ = Describe("makeSignTemplate", func() {
	const (
		unsignedImage  = "my.registry/my/image"
		signedImage    = "my.registry/my/image-signed"
		signerImage    = "some-signer-image:some-tag"
		certSecretName = "cert-secret"
		filesToSign    = "/modules/simple-kmod.ko:/modules/simple-procfs-kmod.ko"
		kernelVersion  = "1.2.3"
		keySecretName  = "key-secret"
		moduleName     = "module-name"
		namespace      = "some-namespace"
		privateKey     = "some private key"
		publicKey      = "some public key"
	)

	var (
		ctrl *gomock.Controller
		clnt *client.MockClient
		rm   *resourceManager
		mld  api.ModuleLoaderData
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		rm = &resourceManager{
			client: clnt,
			scheme: scheme,
		}
		mld = api.ModuleLoaderData{
			Name:      moduleName,
			Namespace: namespace,
			Owner: &kmmv1beta1.Module{
				ObjectMeta: metav1.ObjectMeta{
					Name:      moduleName,
					Namespace: namespace,
				},
			},
			KernelVersion:           kernelVersion,
			KernelNormalizedVersion: kernelVersion,
			Sign: &kmmv1beta1.Sign{
				UnsignedImage: unsignedImage,
				KeySecret:     &v1.LocalObjectReference{Name: keySecretName},
				CertSecret:    &v1.LocalObjectReference{Name: certSecretName},
				FilesToSign:   strings.Split(filesToSign, ","),
			},
			ContainerImage: signedImage,
			RegistryTLS:    &kmmv1beta1.TLSOptions{},
			Modprobe: kmmv1beta1.ModprobeSpec{
				DirName: "/modules",
			},
		}

	})

	publicSignData := map[string][]byte{constants.PublicSignDataKey: []byte(publicKey)}
	privateSignData := map[string][]byte{constants.PrivateSignDataKey: []byte(privateKey)}

	DescribeTable("should set fields correctly", func(imagePullSecret *v1.LocalObjectReference) {

		GinkgoT().Setenv("RELATED_IMAGE_SIGN", "some-signer-image:some-tag")

		ctx := context.Background()
		nodeSelector := map[string]string{"arch": "x64"}

		const dockerfile = `FROM my.registry/my/image as source

FROM some-signer-image:some-tag AS signimage

USER 0

COPY --from=source /modules /opt/modules
RUN for file in /opt/modules/simple-kmod.ko:/modules/simple-procfs-kmod.ko; do \
      [ -e "${file}" ] && /usr/local/bin/sign-file sha256 /run/secrets/key/key /run/secrets/cert/cert "${file}"; \
    done

FROM source
COPY --from=signimage /opt/modules /modules
`
		expected := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: moduleName + "-sign-",
				Labels:       resourceLabels(moduleName, kernelVersion, kmmv1beta1.SignImage),
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
					ServiceAccount: constants.OCPBuilderServiceAccountName,
					Source: buildv1.BuildSource{
						Dockerfile: ptr.To(dockerfile),
						Type:       buildv1.BuildSourceDockerfile,
					},
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.DockerBuildStrategyType,
						DockerStrategy: &buildv1.DockerBuildStrategy{
							Volumes: []buildv1.BuildVolume{
								{
									Name: "key",
									Source: buildv1.BuildVolumeSource{
										Type: buildv1.BuildVolumeSourceTypeSecret,
										Secret: &v1.SecretVolumeSource{
											SecretName: keySecretName,
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
											SecretName: certSecretName,
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
					Output: buildv1.BuildOutput{
						To: &v1.ObjectReference{
							Kind: "DockerImage",
							Name: signedImage,
						},
					},
					NodeSelector:   nodeSelector,
					MountTrustedCA: ptr.To(true),
				},
			},
			Status: buildv1.BuildStatus{},
		}

		gomock.InOrder(
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.KeySecret.Name, Namespace: mld.Namespace}, gomock.Any()).
				DoAndReturn(
					func(_ interface{}, _ interface{}, secret *v1.Secret, _ ...ctrlclient.GetOption) error {
						secret.Data = privateSignData
						return nil
					},
				),
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.CertSecret.Name, Namespace: mld.Namespace}, gomock.Any()).
				DoAndReturn(
					func(_ interface{}, _ interface{}, secret *v1.Secret, _ ...ctrlclient.GetOption) error {
						secret.Data = publicSignData
						return nil
					},
				),
		)

		hash, err := rm.getSignHashAnnotationValue(ctx, mld.Sign.KeySecret.Name, mld.Sign.CertSecret.Name, mld.Namespace, &expected.Spec)
		Expect(err).NotTo(HaveOccurred())
		annotations := map[string]string{
			constants.ResourceHashAnnotation: fmt.Sprintf("%d", hash),
		}
		expected.SetAnnotations(annotations)

		mld.Selector = nodeSelector

		gomock.InOrder(
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.KeySecret.Name, Namespace: mld.Namespace}, gomock.Any()).
				DoAndReturn(
					func(_ interface{}, _ interface{}, secret *v1.Secret, _ ...ctrlclient.GetOption) error {
						secret.Data = privateSignData
						return nil
					},
				),
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.CertSecret.Name, Namespace: mld.Namespace}, gomock.Any()).
				DoAndReturn(
					func(_ interface{}, _ interface{}, secret *v1.Secret, _ ...ctrlclient.GetOption) error {
						secret.Data = publicSignData
						return nil
					},
				),
		)

		actual, err := rm.makeSignTemplate(ctx, &mld, mld.Owner, true)
		Expect(err).NotTo(HaveOccurred())

		Expect(
			cmp.Diff(expected, actual),
		).To(
			BeEmpty(),
		)
	},
		Entry(
			"no secrets at all",
			nil,
		),
		Entry(
			"only imagePullSecrets",
			&v1.LocalObjectReference{Name: "pull-push-secret"},
		),
	)

	It("should leave the build output empty if no push is configured", func() {
		ctx := context.Background()
		mld.Sign = &kmmv1beta1.Sign{
			UnsignedImage: signedImage,
			KeySecret:     &v1.LocalObjectReference{Name: "securebootkey"},
			CertSecret:    &v1.LocalObjectReference{Name: "securebootcert"},
		}
		mld.ContainerImage = unsignedImage
		mld.RegistryTLS = &kmmv1beta1.TLSOptions{}

		gomock.InOrder(
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.KeySecret.Name, Namespace: mld.Namespace}, gomock.Any()).
				DoAndReturn(
					func(_ interface{}, _ interface{}, secret *v1.Secret, _ ...ctrlclient.GetOption) error {
						secret.Data = privateSignData
						return nil
					},
				),
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.CertSecret.Name, Namespace: mld.Namespace}, gomock.Any()).
				DoAndReturn(
					func(_ interface{}, _ interface{}, secret *v1.Secret, _ ...ctrlclient.GetOption) error {
						secret.Data = publicSignData
						return nil
					},
				),
		)

		signObj, err := rm.makeSignTemplate(ctx, &mld, mld.Owner, false)
		Expect(err).NotTo(HaveOccurred())
		actual, ok := signObj.(*buildv1.Build)
		Expect(ok).To(BeTrue())
		Expect(actual.Spec.Output).To(BeZero())
	})

	DescribeTable("should generate correct Dockerfile for signing",
		func(filesToSign []string, expectedSubstrings []string, unexpectedSubstrings []string) {
			GinkgoT().Setenv("RELATED_IMAGE_SIGN", "some-sign-image:some-tag")

			ctx := context.Background()
			mld.Sign = &kmmv1beta1.Sign{
				UnsignedImage: unsignedImage,
				KeySecret:     &v1.LocalObjectReference{Name: "securebootkey"},
				CertSecret:    &v1.LocalObjectReference{Name: "securebootcert"},
				FilesToSign:   filesToSign,
			}
			mld.ContainerImage = signedImage
			mld.RegistryTLS = &kmmv1beta1.TLSOptions{}

			gomock.InOrder(
				clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.KeySecret.Name, Namespace: mld.Namespace}, gomock.Any()).DoAndReturn(
					func(_ interface{}, _ interface{}, secret *v1.Secret, _ ...ctrlclient.GetOption) error {
						secret.Data = privateSignData
						return nil
					},
				),
				clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.CertSecret.Name, Namespace: mld.Namespace}, gomock.Any()).DoAndReturn(
					func(_ interface{}, _ interface{}, secret *v1.Secret, _ ...ctrlclient.GetOption) error {
						secret.Data = publicSignData
						return nil
					},
				),
			)

			actual, err := rm.makeSignTemplate(ctx, &mld, mld.Owner, true)
			Expect(err).NotTo(HaveOccurred())
			actualBuild, ok := actual.(*buildv1.Build)
			Expect(ok).To(BeTrue())

			dockerfile := *actualBuild.Spec.CommonSpec.Source.Dockerfile
			for _, expected := range expectedSubstrings {
				Expect(dockerfile).To(ContainSubstring(expected))
			}
			for _, unexpected := range unexpectedSubstrings {
				Expect(dockerfile).NotTo(ContainSubstring(unexpected))
			}
		},
		Entry(
			"sign explicit paths",
			[]string{"/modules/test.ko"},
			[]string{
				"COPY --from=source /modules /opt/modules",
				"for file in /opt/modules/test.ko; do",
				"/usr/local/bin/sign-file sha256",
				"COPY --from=signimage /opt/modules /modules",
			},
			[]string{"source-extract", "find /tmp/source"},
		),
		Entry(
			"sign multiple paths",
			[]string{"/modules/a.ko", "/modules/b.ko"},
			[]string{
				"COPY --from=source /modules /opt/modules",
				"for file in /opt/modules/a.ko; do",
				"for file in /opt/modules/b.ko; do",
				"COPY --from=signimage /opt/modules /modules",
			},
			[]string{"source-extract"},
		),
		Entry(
			"sign with glob pattern",
			[]string{"/modules/*.ko"},
			[]string{
				"COPY --from=source /modules /opt/modules",
				"for file in /opt/modules/*.ko; do",
				"/usr/local/bin/sign-file sha256",
				"COPY --from=signimage /opt/modules /modules",
			},
			[]string{"source-extract"},
		),
	)
})
var _ = Describe("resourceLabels", func() {

	It("get build labels", func() {
		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{Name: "moduleName"},
		}
		expected := map[string]string{
			"app.kubernetes.io/name":      "kmm",
			"app.kubernetes.io/component": string(kmmv1beta1.BuildImage),
			"app.kubernetes.io/part-of":   "kmm",
			constants.ModuleNameLabel:     "moduleName",
			constants.TargetKernelTarget:  "targetKernel",
			constants.ResourceType:        string(kmmv1beta1.BuildImage),
		}

		labels := resourceLabels(mod.Name, "targetKernel", kmmv1beta1.BuildImage)
		Expect(labels).To(Equal(expected))
	})
})
