package buildconfig

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/mock/gomock"
	buildv1 "github.com/openshift/api/build/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	kmmbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
)

var _ = Describe("Manager", func() {
	var _ = Describe("ShouldSync", func() {
		var (
			ctrl        *gomock.Controller
			clnt        *client.MockClient
			authFactory *auth.MockRegistryAuthGetterFactory
			reg         *registry.MockRegistry
		)
		const (
			moduleName = "module-name"
			imageName  = "image-name"
			namespace  = "some-namespace"
		)

		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			clnt = client.NewMockClient(ctrl)
			authFactory = auth.NewMockRegistryAuthGetterFactory(ctrl)
			reg = registry.NewMockRegistry(ctrl)
		})

		It("should return false if there was no build section", func() {
			ctx := context.Background()

			mod := kmmv1beta1.Module{}
			km := kmmv1beta1.KernelMapping{}

			mgr := NewManager(clnt, nil, nil, authFactory, reg)

			shouldSync, err := mgr.ShouldSync(ctx, mod, km)

			Expect(err).ToNot(HaveOccurred())
			Expect(shouldSync).To(BeFalse())
		})

		It("should return false if image already exists", func() {
			ctx := context.Background()

			km := kmmv1beta1.KernelMapping{
				Build:          &kmmv1beta1.Build{},
				ContainerImage: imageName,
			}

			mod := kmmv1beta1.Module{
				ObjectMeta: metav1.ObjectMeta{
					Name:      moduleName,
					Namespace: namespace,
				},
				Spec: kmmv1beta1.ModuleSpec{
					ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-push-secret"},
				},
			}

			authGetter := &auth.MockRegistryAuthGetter{}
			gomock.InOrder(
				authFactory.EXPECT().NewRegistryAuthGetterFrom(&mod).Return(authGetter),
				reg.EXPECT().ImageExists(ctx, imageName, gomock.Any(), authGetter).Return(true, nil),
			)

			mgr := NewManager(clnt, nil, nil, authFactory, reg)

			shouldSync, err := mgr.ShouldSync(ctx, mod, km)

			Expect(err).ToNot(HaveOccurred())
			Expect(shouldSync).To(BeFalse())
		})

		It("should return false and an error if image check fails", func() {
			ctx := context.Background()

			km := kmmv1beta1.KernelMapping{
				Build:          &kmmv1beta1.Build{},
				ContainerImage: imageName,
			}

			mod := kmmv1beta1.Module{
				ObjectMeta: metav1.ObjectMeta{
					Name:      moduleName,
					Namespace: namespace,
				},
				Spec: kmmv1beta1.ModuleSpec{
					ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-push-secret"},
				},
			}

			authGetter := &auth.MockRegistryAuthGetter{}
			gomock.InOrder(
				authFactory.EXPECT().NewRegistryAuthGetterFrom(&mod).Return(authGetter),
				reg.EXPECT().ImageExists(ctx, imageName, gomock.Any(), authGetter).Return(false, errors.New("generic-registry-error")),
			)

			mgr := NewManager(clnt, nil, nil, authFactory, reg)

			shouldSync, err := mgr.ShouldSync(ctx, mod, km)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("generic-registry-error"))
			Expect(shouldSync).To(BeFalse())
		})

		It("should return true if image does not exist", func() {
			ctx := context.Background()

			km := kmmv1beta1.KernelMapping{
				Build:          &kmmv1beta1.Build{},
				ContainerImage: imageName,
			}

			mod := kmmv1beta1.Module{
				ObjectMeta: metav1.ObjectMeta{
					Name:      moduleName,
					Namespace: namespace,
				},
				Spec: kmmv1beta1.ModuleSpec{
					ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-push-secret"},
				},
			}

			authGetter := &auth.MockRegistryAuthGetter{}
			gomock.InOrder(
				authFactory.EXPECT().NewRegistryAuthGetterFrom(&mod).Return(authGetter),
				reg.EXPECT().ImageExists(ctx, imageName, gomock.Any(), authGetter).Return(false, nil))

			mgr := NewManager(clnt, nil, nil, authFactory, reg)

			shouldSync, err := mgr.ShouldSync(ctx, mod, km)

			Expect(err).ToNot(HaveOccurred())
			Expect(shouldSync).To(BeTrue())
		})
	})

	var _ = Describe("Sync", func() {
		const (
			containerImage = "some-image-name:tag"
			moduleName     = "some-module-names"
			namespace      = "some-namespace"
			targetKernel   = "target-kernel"
		)

		var (
			mockKubeClient            *client.MockClient
			mockMaker                 *MockMaker
			mockOpenShiftBuildsHelper *MockOpenShiftBuildsHelper
		)

		BeforeEach(func() {
			ctrl := gomock.NewController(GinkgoT())
			mockKubeClient = client.NewMockClient(ctrl)
			mockMaker = NewMockMaker(ctrl)
			mockOpenShiftBuildsHelper = NewMockOpenShiftBuildsHelper(ctrl)
		})

		ctx := context.Background()

		It("should create a Build when none is present", func() {
			const (
				buildName      = "some-build-config"
				repoSecretName = "repo-secret"
			)

			By("Authenticating with a secret")

			mod := kmmv1beta1.Module{
				ObjectMeta: metav1.ObjectMeta{
					Name:      moduleName,
					Namespace: namespace,
				},
				Spec: kmmv1beta1.ModuleSpec{
					ImageRepoSecret: &v1.LocalObjectReference{Name: repoSecretName},
				},
			}

			tlsOptions := kmmv1beta1.TLSOptions{}

			buildCfg := kmmv1beta1.Build{
				BaseImageRegistryTLS: tlsOptions,
			}

			mapping := kmmv1beta1.KernelMapping{
				Build:          &buildCfg,
				ContainerImage: containerImage,
			}

			m := NewManager(mockKubeClient, mockMaker, mockOpenShiftBuildsHelper, nil, nil)

			build := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: buildName},
			}

			gomock.InOrder(
				mockMaker.EXPECT().MakeBuildTemplate(ctx, mod, mapping, targetKernel, true, &mod).Return(&build, nil),
				mockOpenShiftBuildsHelper.EXPECT().GetBuild(ctx, mod, targetKernel).Return(nil, errNoMatchingBuild),
				mockKubeClient.EXPECT().Create(ctx, &build),
			)

			res, err := m.Sync(ctx, mod, mapping, targetKernel, true, &mod)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Status).To(BeEquivalentTo(kmmbuild.StatusCreated))
			Expect(res.Requeue).To(BeTrue())
		})

		DescribeTable(
			"should return the Build status when a Build is present",
			func(phase buildv1.BuildPhase, expectedResult kmmbuild.Result, expectError bool) {
				const buildName = "some-build"

				By("Authenticating with the ServiceAccount's pull secret")

				mod := kmmv1beta1.Module{
					ObjectMeta: metav1.ObjectMeta{
						Name:      moduleName,
						Namespace: namespace,
					},
				}

				tlsOptions := kmmv1beta1.TLSOptions{}

				buildCfg := kmmv1beta1.Build{
					BaseImageRegistryTLS: tlsOptions,
				}

				mapping := kmmv1beta1.KernelMapping{
					Build:          &buildCfg,
					ContainerImage: containerImage,
				}

				m := NewManager(mockKubeClient, mockMaker, mockOpenShiftBuildsHelper, nil, nil)

				build := buildv1.Build{
					ObjectMeta: metav1.ObjectMeta{
						Name:        buildName,
						Annotations: map[string]string{buildHashAnnotation: "some hash"},
					},
					Status: buildv1.BuildStatus{Phase: phase},
				}

				gomock.InOrder(
					mockMaker.EXPECT().MakeBuildTemplate(ctx, mod, mapping, targetKernel, true, &mod).Return(&build, nil),
					mockOpenShiftBuildsHelper.EXPECT().GetBuild(ctx, mod, targetKernel).Return(&build, nil),
				)

				res, err := m.Sync(ctx, mod, mapping, targetKernel, true, &mod)

				if expectError {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(res).To(BeEquivalentTo(expectedResult))
				}
			},
			Entry(nil, buildv1.BuildPhaseComplete, kmmbuild.Result{Status: kmmbuild.StatusCompleted}, false),
			Entry(nil, buildv1.BuildPhaseNew, kmmbuild.Result{Status: kmmbuild.StatusInProgress, Requeue: true}, false),
			Entry(nil, buildv1.BuildPhasePending, kmmbuild.Result{Status: kmmbuild.StatusInProgress, Requeue: true}, false),
			Entry(nil, buildv1.BuildPhaseRunning, kmmbuild.Result{Status: kmmbuild.StatusInProgress, Requeue: true}, false),
			Entry(nil, buildv1.BuildPhaseFailed, kmmbuild.Result{}, true),
			Entry(nil, buildv1.BuildPhaseCancelled, kmmbuild.Result{}, true),
		)
	})
})

var _ = Describe("OpenShiftBuildsHelper_GetBuild", func() {
	const targetKernel = "target-kernels"

	var mockKubeClient *client.MockClient

	ctx := context.Background()

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
	})

	It("should return an error if an error occurred", func() {
		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Return(errors.New("random error"))

		osbh := NewOpenShiftBuildsHelper(mockKubeClient)

		_, err := osbh.GetBuild(ctx, kmmv1beta1.Module{}, targetKernel)

		Expect(err).To(HaveOccurred())
	})

	It("should return an error if there are two Builids with the same labels", func() {
		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildList, _ ...ctrlclient.ListOption) {
				bcs.Items = make([]buildv1.Build, 2)
			})

		osbh := NewOpenShiftBuildsHelper(mockKubeClient)

		_, err := osbh.GetBuild(ctx, kmmv1beta1.Module{}, targetKernel)

		Expect(err).To(HaveOccurred())
	})

	It("should work as expected", func() {
		bc := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		}

		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildList, _ ...ctrlclient.ListOption) {
				bcs.Items = []buildv1.Build{*bc}
			})

		osbh := NewOpenShiftBuildsHelper(mockKubeClient)

		res, err := osbh.GetBuild(ctx, kmmv1beta1.Module{}, targetKernel)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(bc))
	})
})
