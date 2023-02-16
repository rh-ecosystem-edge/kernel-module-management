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
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils"
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

			mld := api.ModuleLoaderData{}

			mgr := NewManager(clnt, nil, nil, authFactory, reg)

			shouldSync, err := mgr.ShouldSync(ctx, &mld)

			Expect(err).ToNot(HaveOccurred())
			Expect(shouldSync).To(BeFalse())
		})

		It("should return false if image already exists", func() {
			ctx := context.Background()

			mld := api.ModuleLoaderData{
				Name:            moduleName,
				Namespace:       namespace,
				Build:           &kmmv1beta1.Build{},
				ContainerImage:  imageName,
				ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-push-secret"},
			}

			authGetter := &auth.MockRegistryAuthGetter{}
			gomock.InOrder(
				authFactory.EXPECT().NewRegistryAuthGetterFrom(&mld).Return(authGetter),
				reg.EXPECT().ImageExists(ctx, imageName, gomock.Any(), authGetter).Return(true, nil),
			)

			mgr := NewManager(clnt, nil, nil, authFactory, reg)

			shouldSync, err := mgr.ShouldSync(ctx, &mld)

			Expect(err).ToNot(HaveOccurred())
			Expect(shouldSync).To(BeFalse())
		})

		It("should return false and an error if image check fails", func() {
			ctx := context.Background()

			mld := api.ModuleLoaderData{
				Name:            moduleName,
				Namespace:       namespace,
				Build:           &kmmv1beta1.Build{},
				ContainerImage:  imageName,
				ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-push-secret"},
			}

			authGetter := &auth.MockRegistryAuthGetter{}
			gomock.InOrder(
				authFactory.EXPECT().NewRegistryAuthGetterFrom(&mld).Return(authGetter),
				reg.EXPECT().ImageExists(ctx, imageName, gomock.Any(), authGetter).Return(false, errors.New("generic-registry-error")),
			)

			mgr := NewManager(clnt, nil, nil, authFactory, reg)

			shouldSync, err := mgr.ShouldSync(ctx, &mld)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("generic-registry-error"))
			Expect(shouldSync).To(BeFalse())
		})

		It("should return true if image does not exist", func() {
			ctx := context.Background()

			mld := api.ModuleLoaderData{
				Name:            moduleName,
				Namespace:       namespace,
				Build:           &kmmv1beta1.Build{},
				ContainerImage:  imageName,
				ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-push-secret"},
			}

			authGetter := &auth.MockRegistryAuthGetter{}
			gomock.InOrder(
				authFactory.EXPECT().NewRegistryAuthGetterFrom(&mld).Return(authGetter),
				reg.EXPECT().ImageExists(ctx, imageName, gomock.Any(), authGetter).Return(false, nil))

			mgr := NewManager(clnt, nil, nil, authFactory, reg)

			shouldSync, err := mgr.ShouldSync(ctx, &mld)

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

			tlsOptions := kmmv1beta1.TLSOptions{}

			buildCfg := kmmv1beta1.Build{
				BaseImageRegistryTLS: tlsOptions,
			}

			mld := api.ModuleLoaderData{
				Name:            moduleName,
				Namespace:       namespace,
				ImageRepoSecret: &v1.LocalObjectReference{Name: repoSecretName},
				Build:           &buildCfg,
				ContainerImage:  containerImage,
				KernelVersion:   targetKernel,
			}

			m := NewManager(mockKubeClient, mockMaker, mockOpenShiftBuildsHelper, nil, nil)

			build := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: buildName},
			}

			gomock.InOrder(
				mockMaker.EXPECT().MakeBuildTemplate(ctx, &mld, true, mld.Owner).Return(&build, nil),
				mockOpenShiftBuildsHelper.EXPECT().GetBuild(ctx, &mld).Return(nil, errNoMatchingBuild),
				mockKubeClient.EXPECT().Create(ctx, &build),
			)

			status, err := m.Sync(ctx, &mld, true, mld.Owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(utils.Status(utils.StatusCreated)))
		})

		DescribeTable(
			"should return the Build status when a Build is present",
			func(phase buildv1.BuildPhase, expectedStatus utils.Status, expectError bool) {
				const buildName = "some-build"

				By("Authenticating with the ServiceAccount's pull secret")

				tlsOptions := kmmv1beta1.TLSOptions{}

				buildCfg := kmmv1beta1.Build{
					BaseImageRegistryTLS: tlsOptions,
				}

				mld := api.ModuleLoaderData{
					Name:           moduleName,
					Namespace:      namespace,
					Build:          &buildCfg,
					ContainerImage: containerImage,
					KernelVersion:  targetKernel,
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
					mockMaker.EXPECT().MakeBuildTemplate(ctx, &mld, true, mld.Owner).Return(&build, nil),
					mockOpenShiftBuildsHelper.EXPECT().GetBuild(ctx, &mld).Return(&build, nil),
				)

				status, err := m.Sync(ctx, &mld, true, mld.Owner)

				if expectError {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(status).To(Equal(expectedStatus))
				}
			},
			Entry(nil, buildv1.BuildPhaseComplete, utils.Status(utils.StatusCompleted), false),
			Entry(nil, buildv1.BuildPhaseNew, utils.Status(utils.StatusInProgress), false),
			Entry(nil, buildv1.BuildPhasePending, utils.Status(utils.StatusInProgress), false),
			Entry(nil, buildv1.BuildPhaseRunning, utils.Status(utils.StatusInProgress), false),
			Entry(nil, buildv1.BuildPhaseFailed, utils.Status(""), true),
			Entry(nil, buildv1.BuildPhaseCancelled, utils.Status(""), true),
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
		mld := api.ModuleLoaderData{
			KernelVersion: targetKernel,
		}

		_, err := osbh.GetBuild(ctx, &mld)

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
		mld := api.ModuleLoaderData{
			KernelVersion: targetKernel,
		}

		_, err := osbh.GetBuild(ctx, &mld)

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
		mld := api.ModuleLoaderData{
			KernelVersion: targetKernel,
		}

		res, err := osbh.GetBuild(ctx, &mld)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(bc))
	})
})
