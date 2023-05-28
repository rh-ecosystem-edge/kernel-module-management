package ocpbuild

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	buildmanager "github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils/ocpbuild"
)

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
		mockKubeClient      *client.MockClient
		mockMaker           *MockMaker
		mockOCPBuildsHelper *ocpbuild.MockOCPBuildsHelper
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		mockMaker = NewMockMaker(ctrl)
		mockOCPBuildsHelper = ocpbuild.NewMockOCPBuildsHelper(ctrl)
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

		m := NewManager(mockKubeClient, mockMaker, mockOCPBuildsHelper, nil, nil)

		b := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: buildName},
		}

		gomock.InOrder(
			mockMaker.EXPECT().MakeBuildTemplate(ctx, &mld, true, mld.Owner).Return(&b, nil),
			mockOCPBuildsHelper.EXPECT().GetModuleOCPBuildByKernel(ctx, &mld).Return(nil, ocpbuild.ErrNoMatchingBuild),
			mockKubeClient.EXPECT().Create(ctx, &b),
		)

		status, err := m.Sync(ctx, &mld, true, mld.Owner)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(ocpbuild.StatusCreated))
	})

	DescribeTable(
		"should return the Build status when a Build is present",
		func(phase buildv1.BuildPhase, expectedStatus ocpbuild.Status, expectError bool) {
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

			m := NewManager(mockKubeClient, mockMaker, mockOCPBuildsHelper, nil, nil)

			build := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{
					Name:        buildName,
					Annotations: map[string]string{ocpbuild.HashAnnotation: "some hash"},
				},
				Status: buildv1.BuildStatus{Phase: phase},
			}

			gomock.InOrder(
				mockMaker.EXPECT().MakeBuildTemplate(ctx, &mld, true, mld.Owner).Return(&build, nil),
				mockOCPBuildsHelper.EXPECT().GetModuleOCPBuildByKernel(ctx, &mld).Return(&build, nil),
			)

			status, err := m.Sync(ctx, &mld, true, mld.Owner)

			if expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal(expectedStatus))
			}
		},
		Entry(nil, buildv1.BuildPhaseComplete, ocpbuild.StatusCompleted, false),
		Entry(nil, buildv1.BuildPhaseNew, ocpbuild.StatusInProgress, false),
		Entry(nil, buildv1.BuildPhasePending, ocpbuild.StatusInProgress, false),
		Entry(nil, buildv1.BuildPhaseRunning, ocpbuild.StatusInProgress, false),
		Entry(nil, buildv1.BuildPhaseFailed, ocpbuild.Status(""), true),
		Entry(nil, buildv1.BuildPhaseCancelled, ocpbuild.Status(""), true),
	)
})

var _ = Describe("GarbageCollect", func() {
	var (
		ctrl                *gomock.Controller
		clnt                *client.MockClient
		mockOCPBuildsHelper *ocpbuild.MockOCPBuildsHelper
		m                   buildmanager.Manager
	)
	const (
		moduleName = "module-name"
		namespace  = "some-namespace"
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		mockOCPBuildsHelper = ocpbuild.NewMockOCPBuildsHelper(ctrl)
		m = NewManager(clnt, nil, mockOCPBuildsHelper, nil, nil)
	})

	ctx := context.Background()

	It("GetModuleBuilds failed", func() {
		mockOCPBuildsHelper.EXPECT().GetModuleOCPBuilds(ctx, moduleName, namespace).Return(nil, fmt.Errorf("some error"))

		deleted, err := m.GarbageCollect(ctx, moduleName, namespace, nil)

		Expect(err).To(HaveOccurred())
		Expect(deleted).To(BeEmpty())
	})

	It("DeleteBuild failed", func() {
		ocpBuild := buildv1.Build{
			Status: buildv1.BuildStatus{
				Phase: buildv1.BuildPhaseComplete,
			},
		}
		gomock.InOrder(
			mockOCPBuildsHelper.EXPECT().GetModuleOCPBuilds(ctx, moduleName, namespace).Return([]buildv1.Build{ocpBuild}, nil),
			mockOCPBuildsHelper.EXPECT().DeleteOCPBuild(ctx, &ocpBuild).Return(fmt.Errorf("some error")),
		)
		deleted, err := m.GarbageCollect(ctx, moduleName, namespace, nil)

		Expect(err).To(HaveOccurred())
		Expect(deleted).To(BeEmpty())
	})

	DescribeTable("should return the correct error and names of the collected OCP builds",
		func(buildPhase1 buildv1.BuildPhase, buildPhase2 buildv1.BuildPhase, numSuccessfulBuilds int) {
			ocpBuild1 := buildv1.Build{
				Status: buildv1.BuildStatus{
					Phase: buildPhase1,
				},
			}
			ocpBuild2 := buildv1.Build{
				Status: buildv1.BuildStatus{
					Phase: buildPhase2,
				},
			}
			mockOCPBuildsHelper.EXPECT().GetModuleOCPBuilds(ctx, moduleName, namespace).Return([]buildv1.Build{ocpBuild1, ocpBuild2}, nil)
			if buildPhase1 == buildv1.BuildPhaseComplete {
				mockOCPBuildsHelper.EXPECT().DeleteOCPBuild(ctx, &ocpBuild1).Return(nil)
			}
			if buildPhase2 == buildv1.BuildPhaseComplete {
				mockOCPBuildsHelper.EXPECT().DeleteOCPBuild(ctx, &ocpBuild2).Return(nil)
			}

			deleted, err := m.GarbageCollect(ctx, moduleName, namespace, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(len(deleted)).To(Equal(numSuccessfulBuilds))
		},
		Entry("0 successfull builds", buildv1.BuildPhaseRunning, buildv1.BuildPhaseRunning, 0),
		Entry("1 successfull builds", buildv1.BuildPhaseRunning, buildv1.BuildPhaseComplete, 1),
		Entry("2 successfull builds", buildv1.BuildPhaseComplete, buildv1.BuildPhaseComplete, 2),
	)

})
