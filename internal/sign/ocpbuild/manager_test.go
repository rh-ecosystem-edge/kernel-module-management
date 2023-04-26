package ocpbuild

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils/build"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		It("should return false if there was no sign section", func() {
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
				ContainerImage:  imageName,
				ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-push-secret"},
				Sign:            &kmmv1beta1.Sign{},
			}

			authGetter := &auth.MockRegistryAuthGetter{}
			gomock.InOrder(
				authFactory.EXPECT().NewRegistryAuthGetterFrom(&mld).Return(authGetter),
				reg.EXPECT().ImageExists(ctx, imageName, gomock.Any(), authGetter).Return(true, nil),
			)

			Expect(
				NewManager(clnt, nil, nil, authFactory, reg).ShouldSync(ctx, &mld),
			).To(
				BeFalse(),
			)
		})

		It("should return false and an error if image check fails", func() {
			ctx := context.Background()

			mld := api.ModuleLoaderData{
				Name:            moduleName,
				Namespace:       namespace,
				ContainerImage:  imageName,
				ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-push-secret"},
				Sign:            &kmmv1beta1.Sign{},
			}

			const errMsg = "generic-registry-error"

			authGetter := &auth.MockRegistryAuthGetter{}

			gomock.InOrder(
				authFactory.EXPECT().NewRegistryAuthGetterFrom(&mld).Return(authGetter),
				reg.EXPECT().ImageExists(ctx, imageName, gomock.Any(), authGetter).Return(false, errors.New(errMsg)),
			)

			mgr := NewManager(clnt, nil, nil, authFactory, reg)

			_, err := mgr.ShouldSync(ctx, &mld)

			Expect(err).To(MatchError(ContainSubstring(errMsg)))
		})

		It("should return true if image does not exist", func() {
			ctx := context.Background()

			mld := api.ModuleLoaderData{
				Name:            moduleName,
				Namespace:       namespace,
				ContainerImage:  imageName,
				ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-push-secret"},
				Sign:            &kmmv1beta1.Sign{},
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
			unsignedImage  = "unsigned-image:tag"
		)

		var (
			mockKubeClient            *client.MockClient
			mockMaker                 *MockMaker
			mockOpenShiftBuildsHelper *build.MockOpenShiftBuildsHelper
		)

		BeforeEach(func() {
			ctrl := gomock.NewController(GinkgoT())
			mockKubeClient = client.NewMockClient(ctrl)
			mockMaker = NewMockMaker(ctrl)
			mockOpenShiftBuildsHelper = build.NewMockOpenShiftBuildsHelper(ctrl)
		})

		ctx := context.Background()

		It("should create a Build when none is present", func() {
			const (
				buildName      = "some-build-config"
				repoSecretName = "repo-secret"
			)

			By("Authenticating with a secret")

			mld := api.ModuleLoaderData{
				Name:            moduleName,
				Namespace:       namespace,
				ImageRepoSecret: &v1.LocalObjectReference{Name: repoSecretName},
				ContainerImage:  containerImage,
				KernelVersion:   targetKernel,
				Sign:            &kmmv1beta1.Sign{UnsignedImage: unsignedImage},
			}

			m := NewManager(mockKubeClient, mockMaker, mockOpenShiftBuildsHelper, nil, nil)

			b := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: buildName},
			}

			gomock.InOrder(
				mockMaker.EXPECT().MakeBuildTemplate(ctx, &mld, unsignedImage, true, mld.Owner).Return(&b, nil),
				mockOpenShiftBuildsHelper.EXPECT().GetBuild(ctx, &mld).Return(nil, build.ErrNoMatchingBuild),
				mockKubeClient.EXPECT().Create(ctx, &b),
			)

			status, err := m.Sync(ctx, &mld, unsignedImage, true, mld.Owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(build.StatusCreated))
		})

		DescribeTable(
			"should return the Build status when a Build is present",
			func(phase buildv1.BuildPhase, expectedStatus build.Status, expectError bool) {
				const buildName = "some-build"

				By("Authenticating with the ServiceAccount's pull secret")

				mld := api.ModuleLoaderData{
					Name:           moduleName,
					Namespace:      namespace,
					ContainerImage: containerImage,
					KernelVersion:  targetKernel,
					Sign:           &kmmv1beta1.Sign{UnsignedImage: unsignedImage},
				}

				m := NewManager(mockKubeClient, mockMaker, mockOpenShiftBuildsHelper, nil, nil)

				build := buildv1.Build{
					ObjectMeta: metav1.ObjectMeta{
						Name:        buildName,
						Annotations: map[string]string{build.HashAnnotation: "some hash"},
					},
					Status: buildv1.BuildStatus{Phase: phase},
				}

				gomock.InOrder(
					mockMaker.EXPECT().MakeBuildTemplate(ctx, &mld, unsignedImage, true, mld.Owner).Return(&build, nil),
					mockOpenShiftBuildsHelper.EXPECT().GetBuild(ctx, &mld).Return(&build, nil),
				)

				status, err := m.Sync(ctx, &mld, unsignedImage, true, mld.Owner)

				if expectError {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
					Expect(status).To(Equal(expectedStatus))
				}
			},
			Entry(nil, buildv1.BuildPhaseComplete, build.StatusCompleted, false),
			Entry(nil, buildv1.BuildPhaseNew, build.StatusInProgress, false),
			Entry(nil, buildv1.BuildPhasePending, build.StatusInProgress, false),
			Entry(nil, buildv1.BuildPhaseRunning, build.StatusInProgress, false),
			Entry(nil, buildv1.BuildPhaseFailed, build.Status(""), true),
			Entry(nil, buildv1.BuildPhaseCancelled, build.Status(""), true),
		)
	})
})
