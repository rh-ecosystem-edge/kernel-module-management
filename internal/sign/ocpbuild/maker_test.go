package ocpbuild

import (
	"context"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils/build"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
)

var _ = Describe("maker_MakeBuildTemplate", func() {
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
		mld  api.ModuleLoaderData
		m    Maker
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		m = NewMaker(clnt, signerImage, scheme)
		mld = api.ModuleLoaderData{
			Name:      moduleName,
			Namespace: namespace,
			Owner: &kmmv1beta1.Module{
				ObjectMeta: metav1.ObjectMeta{
					Name:      moduleName,
					Namespace: namespace,
				},
			},
			KernelVersion: kernelVersion,
		}
	})

	publicSignData := map[string][]byte{constants.PublicSignDataKey: []byte(publicKey)}
	privateSignData := map[string][]byte{constants.PrivateSignDataKey: []byte(privateKey)}

	DescribeTable("should set fields correctly", func(imagePullSecret *v1.LocalObjectReference) {
		ctx := context.Background()
		nodeSelector := map[string]string{"arch": "x64"}

		mld.Sign = &kmmv1beta1.Sign{
			UnsignedImage: unsignedImage,
			KeySecret:     &v1.LocalObjectReference{Name: keySecretName},
			CertSecret:    &v1.LocalObjectReference{Name: certSecretName},
			FilesToSign:   strings.Split(filesToSign, ","),
		}
		mld.ContainerImage = signedImage
		mld.RegistryTLS = &kmmv1beta1.TLSOptions{}

		const dockerfile = `FROM my.registry/my/image as source

FROM some-signer-image:some-tag AS signimage

USER 0

RUN ["mkdir", "/signroot"]

COPY --from=source /modules/simple-kmod.ko:/modules/simple-procfs-kmod.ko /signroot/modules/simple-kmod.ko:/modules/simple-procfs-kmod.ko
RUN /usr/local/bin/sign-file sha256 /run/secrets/key/key /run/secrets/cert/cert /signroot/modules/simple-kmod.ko:/modules/simple-procfs-kmod.ko

FROM source

COPY --from=signimage /signroot/modules/simple-kmod.ko:/modules/simple-procfs-kmod.ko /modules/simple-kmod.ko:/modules/simple-procfs-kmod.ko
`

		expected := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: moduleName + "-sign-",
				Labels:       build.GetBuildLabels(&mld, BuildType),
				Annotations:  map[string]string{build.HashAnnotation: "1404066013727235306"},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "kmm.sigs.x-k8s.io/v1beta1",
						Kind:               "Module",
						Name:               moduleName,
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				},
			},
			Spec: buildv1.BuildSpec{
				CommonSpec: buildv1.CommonSpec{
					ServiceAccount: constants.OCPBuilderServiceAccountName,
					Source: buildv1.BuildSource{
						Dockerfile: pointer.String(dockerfile),
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
											Optional:   pointer.Bool(false),
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
											Optional:   pointer.Bool(false),
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
					MountTrustedCA: pointer.Bool(true),
				},
			},
			Status: buildv1.BuildStatus{},
		}

		mld.Selector = nodeSelector

		gomock.InOrder(
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.CertSecret.Name, Namespace: mld.Namespace}, gomock.Any()).DoAndReturn(
				func(_ interface{}, _ interface{}, secret *v1.Secret, _ ...ctrlclient.GetOption) error {
					secret.Data = publicSignData
					return nil
				},
			),
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.KeySecret.Name, Namespace: mld.Namespace}, gomock.Any()).DoAndReturn(
				func(_ interface{}, _ interface{}, secret *v1.Secret, _ ...ctrlclient.GetOption) error {
					secret.Data = privateSignData
					return nil
				},
			),
		)

		actual, err := m.MakeBuildTemplate(ctx, &mld, unsignedImage, true, mld.Owner)
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
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.CertSecret.Name, Namespace: mld.Namespace}, gomock.Any()),
			clnt.EXPECT().Get(ctx, types.NamespacedName{Name: mld.Sign.KeySecret.Name, Namespace: mld.Namespace}, gomock.Any()),
		)

		actual, err := m.MakeBuildTemplate(ctx, &mld, "", false, mld.Owner)
		Expect(err).NotTo(HaveOccurred())
		Expect(actual.Spec.Output).To(BeZero())
	})
})
