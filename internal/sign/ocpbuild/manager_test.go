package ocpbuild

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/budougumi0617/cmpmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	signmanager "github.com/rh-ecosystem-edge/kernel-module-management/internal/sign"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils/ocpbuild"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ShouldSync", func() {
	var (
		ctrl        *gomock.Controller
		clnt        *client.MockClient
		authFactory *auth.MockRegistryAuthGetterFactory
		reg         *registry.MockRegistry
		mgr         signmanager.SignManager
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
		mgr = NewManager(clnt, nil, nil, authFactory, reg)
	})

	It("should return false if there was no sign section", func() {
		ctx := context.Background()

		mld := api.ModuleLoaderData{}

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
			mgr.ShouldSync(ctx, &mld),
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
		mockKubeClient      *client.MockClient
		mockMaker           *MockMaker
		mockOCPBuildsHelper *ocpbuild.MockOCPBuildsHelper
		mgr                 signmanager.SignManager
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		mockMaker = NewMockMaker(ctrl)
		mockOCPBuildsHelper = ocpbuild.NewMockOCPBuildsHelper(ctrl)
		mgr = NewManager(mockKubeClient, mockMaker, mockOCPBuildsHelper, nil, nil)
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

		b := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: buildName},
		}

		gomock.InOrder(
			mockMaker.EXPECT().MakeBuildTemplate(ctx, &mld, unsignedImage, true, mld.Owner).Return(&b, nil),
			mockOCPBuildsHelper.EXPECT().GetModuleOCPBuildByKernel(ctx, &mld, mld.Owner).Return(nil, ocpbuild.ErrNoMatchingBuild),
			mockKubeClient.EXPECT().Create(ctx, &b),
		)

		status, err := mgr.Sync(ctx, &mld, unsignedImage, true, mld.Owner)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(ocpbuild.StatusCreated))
	})

	DescribeTable(
		"should return the Build status when a Build is present",
		func(phase buildv1.BuildPhase, expectedStatus ocpbuild.Status, expectError bool) {
			const buildName = "some-build"

			By("Authenticating with the ServiceAccount's pull secret")

			mld := api.ModuleLoaderData{
				Name:           moduleName,
				Namespace:      namespace,
				ContainerImage: containerImage,
				KernelVersion:  targetKernel,
				Sign:           &kmmv1beta1.Sign{UnsignedImage: unsignedImage},
			}

			build := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{
					Name:        buildName,
					Annotations: map[string]string{ocpbuild.HashAnnotation: "some hash"},
				},
				Status: buildv1.BuildStatus{Phase: phase},
			}

			gomock.InOrder(
				mockMaker.EXPECT().MakeBuildTemplate(ctx, &mld, unsignedImage, true, mld.Owner).Return(&build, nil),
				mockOCPBuildsHelper.EXPECT().GetModuleOCPBuildByKernel(ctx, &mld, mld.Owner).Return(&build, nil),
			)

			status, err := mgr.Sync(ctx, &mld, unsignedImage, true, mld.Owner)

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
		m                   signmanager.SignManager
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

	It("GetModuleOCPBuilds failed", func() {
		mockOCPBuildsHelper.EXPECT().GetModuleOCPBuilds(ctx, moduleName, namespace, nil).Return(nil, fmt.Errorf("some error"))

		deleted, err := m.GarbageCollect(ctx, moduleName, namespace, nil, 0)

		Expect(err).To(HaveOccurred())
		Expect(deleted).To(BeEmpty())
	})

	It("DeleteOCPBuild failed", func() {
		ocpBuild := buildv1.Build{
			Status: buildv1.BuildStatus{
				Phase: buildv1.BuildPhaseComplete,
			},
		}
		gomock.InOrder(
			mockOCPBuildsHelper.EXPECT().GetModuleOCPBuilds(ctx, moduleName, namespace, nil).Return([]buildv1.Build{ocpBuild}, nil),
			mockOCPBuildsHelper.EXPECT().DeleteOCPBuild(ctx, &ocpBuild).Return(fmt.Errorf("some error")),
		)
		deleted, err := m.GarbageCollect(ctx, moduleName, namespace, nil, 0)

		Expect(err).To(HaveOccurred())
		Expect(deleted).To(BeEmpty())
	})

	mld := api.ModuleLoaderData{
		Name:  "moduleName",
		Owner: &kmmv1beta1.Module{},
	}

	type testCase struct {
		podPhase1, podPhase2                       buildv1.BuildPhase
		gcDelay                                    time.Duration
		expectsErr                                 bool
		resShouldContainPod1, resShouldContainPod2 bool
	}

	DescribeTable("should return the correct error and names of the collected pods",
		func(tc testCase) {
			const (
				build1Name = "pod-1"
				build2Name = "pod-2"
			)

			ctx := context.Background()

			build1 := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: build1Name},
				Status:     buildv1.BuildStatus{Phase: tc.podPhase1},
			}
			build2 := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: build2Name},
				Status:     buildv1.BuildStatus{Phase: tc.podPhase2},
			}

			returnedError := fmt.Errorf("some error")
			if !tc.expectsErr {
				returnedError = nil
			}

			buildList := []buildv1.Build{build1, build2}

			calls := []any{
				mockOCPBuildsHelper.EXPECT().GetModuleOCPBuilds(ctx, mld.Name, mld.Namespace, mld.Owner).Return(buildList, returnedError),
			}

			if !tc.expectsErr {
				now := metav1.Now()

				for i := 0; i < len(buildList); i++ {
					build := &buildList[i]

					if build.Status.Phase == buildv1.BuildPhaseComplete {
						c := mockOCPBuildsHelper.
							EXPECT().
							DeleteOCPBuild(ctx, cmpmock.DiffEq(build)).
							Do(func(_ context.Context, b *buildv1.Build) {
								b.DeletionTimestamp = &now
								build.DeletionTimestamp = &now
							})

						calls = append(calls, c)

						if time.Now().After(now.Add(tc.gcDelay)) {
							calls = append(
								calls,
								mockOCPBuildsHelper.EXPECT().RemoveFinalizer(ctx, cmpmock.DiffEq(build), constants.GCDelayFinalizer),
							)
						}
					}
				}
			}

			gomock.InOrder(calls...)

			names, err := m.GarbageCollect(ctx, mld.Name, mld.Namespace, mld.Owner, tc.gcDelay)

			if tc.expectsErr {
				Expect(err).To(HaveOccurred())
				return
			}

			Expect(err).NotTo(HaveOccurred())

			if tc.resShouldContainPod1 {
				Expect(names).To(ContainElements(build1Name))
			}

			if tc.resShouldContainPod2 {
				Expect(names).To(ContainElements(build2Name))
			}
		},
		Entry(
			"all pods succeeded",
			testCase{
				podPhase1:            buildv1.BuildPhaseComplete,
				podPhase2:            buildv1.BuildPhaseComplete,
				resShouldContainPod1: true,
				resShouldContainPod2: true,
			},
		),
		Entry(
			"1 pod succeeded",
			testCase{
				podPhase1:            buildv1.BuildPhaseComplete,
				podPhase2:            buildv1.BuildPhaseFailed,
				resShouldContainPod1: true,
			},
		),
		Entry(
			"0 pod succeeded",
			testCase{
				podPhase1: buildv1.BuildPhaseFailed,
				podPhase2: buildv1.BuildPhaseFailed,
			},
		),
		Entry(
			"error occurred",
			testCase{expectsErr: true},
		),
		Entry(
			"GC delayed",
			testCase{
				podPhase1:            buildv1.BuildPhaseComplete,
				podPhase2:            buildv1.BuildPhaseComplete,
				gcDelay:              time.Hour,
				resShouldContainPod1: false,
				resShouldContainPod2: false,
			},
		),
	)

})
