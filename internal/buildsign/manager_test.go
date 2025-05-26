package buildsign

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/kernel"
)

var _ = Describe("GetStatus", func() {
	var (
		ctrl                *gomock.Controller
		clnt                *client.MockClient
		mockResourceManager *MockResourceManager
		mgr                 Manager
	)
	const (
		mbscName      = "some-name"
		imageName     = "image-name"
		mbscNamespace = "some-namespace"
		kernelVersion = "some version"
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		mockResourceManager = NewMockResourceManager(ctrl)
		mgr = NewManager(clnt, mockResourceManager, scheme)
	})

	ctx := context.Background()
	testMBSC := kmmv1beta1.ModuleBuildSignConfig{}

	It("failed flow, GetResourceByKernel fails", func() {
		normalizedKernel := kernel.NormalizeVersion(kernelVersion)
		mockResourceManager.EXPECT().GetResourceByKernel(ctx, mbscName, mbscNamespace, normalizedKernel,
			kmmv1beta1.BuildImage, &testMBSC).
			Return(nil, fmt.Errorf("some error"))

		status, err := mgr.GetStatus(ctx, mbscName, mbscNamespace, kernelVersion, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
		Expect(status).To(Equal(kmmv1beta1.BuildOrSignStatus("")))
	})

	It("GetResourceByKernel returns pod does not exists", func() {
		normalizedKernel := kernel.NormalizeVersion(kernelVersion)
		mockResourceManager.EXPECT().GetResourceByKernel(ctx, mbscName, mbscNamespace, normalizedKernel,
			kmmv1beta1.BuildImage, &testMBSC).
			Return(nil, ErrNoMatchingBuildSignResource)

		status, err := mgr.GetStatus(ctx, mbscName, mbscNamespace, kernelVersion, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(BeNil())
		Expect(status).To(Equal(kmmv1beta1.BuildOrSignStatus("")))
	})

	It("failed flow, GetResourceStatus fails", func() {
		foundBuild := buildv1.Build{}
		normalizedKernel := kernel.NormalizeVersion(kernelVersion)
		gomock.InOrder(
			mockResourceManager.EXPECT().GetResourceByKernel(ctx, mbscName, mbscNamespace, normalizedKernel,
				kmmv1beta1.BuildImage, &testMBSC).
				Return(&foundBuild, nil),
			mockResourceManager.EXPECT().GetResourceStatus(&foundBuild).Return(Status(""), fmt.Errorf("some error")),
		)

		status, err := mgr.GetStatus(ctx, mbscName, mbscNamespace, kernelVersion, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
		Expect(status).To(Equal(kmmv1beta1.BuildOrSignStatus("")))
	})

	DescribeTable("check good flow and returned statuses", func(buildStatus Status, expectedStatus kmmv1beta1.BuildOrSignStatus) {
		foundBuild := buildv1.Build{}
		normalizedKernel := kernel.NormalizeVersion(kernelVersion)
		gomock.InOrder(
			mockResourceManager.EXPECT().GetResourceByKernel(ctx, mbscName, mbscNamespace, normalizedKernel,
				kmmv1beta1.BuildImage, &testMBSC).
				Return(&foundBuild, nil),
			mockResourceManager.EXPECT().GetResourceStatus(&foundBuild).Return(buildStatus, nil),
		)
		status, err := mgr.GetStatus(ctx, mbscName, mbscNamespace, kernelVersion, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(BeNil())
		Expect(status).To(Equal(expectedStatus))
	},
		Entry("build's status is success", StatusCompleted, kmmv1beta1.ActionSuccess),
		Entry("build's status is failure", StatusFailed, kmmv1beta1.ActionFailure),
		Entry("build's status is in progress", StatusInProgress, kmmv1beta1.BuildOrSignStatus("")),
		Entry("build's status is in unknown", Status(""), kmmv1beta1.BuildOrSignStatus("")),
	)
})

var _ = Describe("Sync", func() {
	var (
		ctrl                *gomock.Controller
		clnt                *client.MockClient
		mockResourceManager *MockResourceManager
		mgr                 Manager
	)
	const (
		mbscName      = "some-name"
		imageName     = "image-name"
		mbscNamespace = "some-namespace"
		kernelVersion = "some version"
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		mockResourceManager = NewMockResourceManager(ctrl)
		mgr = NewManager(clnt, mockResourceManager, scheme)
	})

	ctx := context.Background()
	testMBSC := kmmv1beta1.ModuleBuildSignConfig{}
	testMLD := &api.ModuleLoaderData{
		Name:                    mbscName,
		Namespace:               mbscNamespace,
		KernelNormalizedVersion: kernelVersion,
	}

	It("makeSignTemplate failed", func() {
		By("test build action")
		mockResourceManager.EXPECT().MakeResourceTemplate(ctx, testMLD, &testMBSC, true, kmmv1beta1.BuildImage).
			Return(nil, fmt.Errorf("some error"))
		err := mgr.Sync(ctx, testMLD, true, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())

		By("test sign action")
		mockResourceManager.EXPECT().MakeResourceTemplate(ctx, testMLD, &testMBSC, true, kmmv1beta1.SignImage).
			Return(nil, fmt.Errorf("some error"))
		err = mgr.Sync(ctx, testMLD, true, kmmv1beta1.SignImage, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("GetResourceByKernel failed", func() {
		gomock.InOrder(
			mockResourceManager.EXPECT().MakeResourceTemplate(ctx, testMLD, &testMBSC, true, kmmv1beta1.BuildImage).
				Return(nil, nil),
			mockResourceManager.EXPECT().GetResourceByKernel(ctx, mbscName, mbscNamespace, kernelVersion,
				kmmv1beta1.BuildImage, &testMBSC).
				Return(nil, fmt.Errorf("some error")),
		)
		err := mgr.Sync(ctx, testMLD, true, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("CreateResource failed", func() {
		testTemplate := buildv1.Build{}
		gomock.InOrder(
			mockResourceManager.EXPECT().MakeResourceTemplate(ctx, testMLD, &testMBSC, true, kmmv1beta1.BuildImage).
				Return(&testTemplate, nil),
			mockResourceManager.EXPECT().GetResourceByKernel(ctx, mbscName, mbscNamespace, kernelVersion,
				kmmv1beta1.BuildImage, &testMBSC).
				Return(nil, ErrNoMatchingBuildSignResource),
			mockResourceManager.EXPECT().CreateResource(ctx, &testTemplate).Return(fmt.Errorf("some error")),
		)
		err := mgr.Sync(ctx, testMLD, true, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("IsResourceChanged failed", func() {
		testTemplate := buildv1.Build{}
		testBuild := buildv1.Build{}
		gomock.InOrder(
			mockResourceManager.EXPECT().MakeResourceTemplate(ctx, testMLD, &testMBSC, true, kmmv1beta1.BuildImage).
				Return(&testTemplate, nil),
			mockResourceManager.EXPECT().GetResourceByKernel(ctx, mbscName, mbscNamespace, kernelVersion,
				kmmv1beta1.BuildImage, &testMBSC).
				Return(&testBuild, nil),
			mockResourceManager.EXPECT().IsResourceChanged(&testBuild, &testTemplate).Return(false, fmt.Errorf("some error")),
		)
		err := mgr.Sync(ctx, testMLD, true, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("DeleteResource failed should not cause failure", func() {
		testTemplate := buildv1.Build{}
		testBuild := buildv1.Build{}
		gomock.InOrder(
			mockResourceManager.EXPECT().MakeResourceTemplate(ctx, testMLD, &testMBSC, true, kmmv1beta1.BuildImage).
				Return(&testTemplate, nil),
			mockResourceManager.EXPECT().GetResourceByKernel(ctx, mbscName, mbscNamespace, kernelVersion,
				kmmv1beta1.BuildImage, &testMBSC).
				Return(&testBuild, nil),
			mockResourceManager.EXPECT().IsResourceChanged(&testBuild, &testTemplate).Return(true, nil),
			mockResourceManager.EXPECT().DeleteResource(ctx, &testBuild).Return(fmt.Errorf("some error")),
		)
		err := mgr.Sync(ctx, testMLD, true, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(BeNil())
	})

	DescribeTable("check good flow", func(buildAction, buildExists, buildChanged, pushImage bool) {
		testBuildTemplate := buildv1.Build{}
		existingTestBuild := buildv1.Build{}
		testAction := kmmv1beta1.BuildImage
		if !buildAction {
			testAction = kmmv1beta1.SignImage
		}

		if buildAction {
			mockResourceManager.EXPECT().MakeResourceTemplate(ctx, testMLD, &testMBSC, pushImage, kmmv1beta1.BuildImage).
				Return(&testBuildTemplate, nil)
		} else {
			mockResourceManager.EXPECT().MakeResourceTemplate(ctx, testMLD, &testMBSC, pushImage, kmmv1beta1.SignImage).
				Return(&testBuildTemplate, nil)
		}
		var getBuildError error
		if !buildExists {
			getBuildError = ErrNoMatchingBuildSignResource
		}
		mockResourceManager.EXPECT().GetResourceByKernel(ctx, mbscName, mbscNamespace, kernelVersion,
			testAction, &testMBSC).Return(&existingTestBuild, getBuildError)
		if !buildExists {
			mockResourceManager.EXPECT().CreateResource(ctx, &testBuildTemplate).Return(nil)
			goto executeTestFunction
		}
		mockResourceManager.EXPECT().IsResourceChanged(&existingTestBuild, &testBuildTemplate).Return(buildChanged, nil)
		if buildChanged {
			mockResourceManager.EXPECT().DeleteResource(ctx, &existingTestBuild).Return(nil)
		}

	executeTestFunction:
		err := mgr.Sync(ctx, testMLD, pushImage, testAction, &testMBSC)
		Expect(err).To(BeNil())
	},
		Entry("action build, build OCPBuild does not exists", true, false, false, true),
		Entry("action sign, sign OCPBuild does not exists", false, false, false, false),
		Entry("action build, build OCPBuild exists, build object has not changed", true, true, false, true),
		Entry("action sign, sign OCPBuild exists, build object has not changed", false, true, false, false),
		Entry("action build, build OCPBuild exists, build object has changed", true, true, true, false),
		Entry("action sign, sign OCPBuild exists, build object has changed", false, true, true, false),
	)
})

var _ = Describe("GarbageCollect", func() {
	var (
		ctrl                *gomock.Controller
		clnt                *client.MockClient
		mockResourceManager *MockResourceManager
		mgr                 Manager
	)
	const (
		mbscName      = "some-name"
		imageName     = "image-name"
		mbscNamespace = "some-namespace"
		kernelVersion = "some version"
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		mockResourceManager = NewMockResourceManager(ctrl)
		mgr = NewManager(clnt, mockResourceManager, scheme)
	})

	ctx := context.Background()
	testMBSC := kmmv1beta1.ModuleBuildSignConfig{}

	It("failed to get module buildss", func() {
		mockResourceManager.EXPECT().GetModuleResources(ctx, mbscName, mbscNamespace,
			kmmv1beta1.BuildImage, &testMBSC).Return(nil, fmt.Errorf("some error"))

		_, err := mgr.GarbageCollect(ctx, mbscName, mbscNamespace, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("delete ocpbuild failed", func() {
		testBuild := buildv1.Build{
			Status: buildv1.BuildStatus{
				Phase: buildv1.BuildPhaseComplete,
			},
		}
		gomock.InOrder(
			mockResourceManager.EXPECT().GetModuleResources(ctx, mbscName, mbscNamespace, kmmv1beta1.BuildImage, &testMBSC).
				Return([]metav1.Object{&testBuild}, nil),
			mockResourceManager.EXPECT().HasResourcesCompletedSuccessfully(ctx, &testBuild).Return(true, nil),
			mockResourceManager.EXPECT().DeleteResource(ctx, &testBuild).Return(fmt.Errorf("some error")),
		)

		_, err := mgr.GarbageCollect(ctx, mbscName, mbscNamespace, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("good flow", func() {
		testBuildSuccess := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildSuccess"},
			Status:     buildv1.BuildStatus{Phase: buildv1.BuildPhaseComplete},
		}
		testBuildFailure := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildFailure"},
			Status:     buildv1.BuildStatus{Phase: buildv1.BuildPhaseFailed},
		}
		testBuildRunning := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildRunning"},
			Status:     buildv1.BuildStatus{Phase: buildv1.BuildPhaseRunning},
		}
		testBuildError := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildError"},
			Status:     buildv1.BuildStatus{Phase: buildv1.BuildPhaseError},
		}
		testBuildPending := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildPending"},
			Status:     buildv1.BuildStatus{Phase: buildv1.BuildPhasePending},
		}
		returnedBuilds := []metav1.Object{testBuildSuccess, testBuildFailure, testBuildRunning, testBuildError, testBuildPending}
		gomock.InOrder(
			mockResourceManager.EXPECT().GetModuleResources(ctx, mbscName, mbscNamespace, kmmv1beta1.BuildImage, &testMBSC).
				Return(returnedBuilds, nil),
			mockResourceManager.EXPECT().HasResourcesCompletedSuccessfully(ctx, testBuildSuccess).Return(true, nil),
			mockResourceManager.EXPECT().DeleteResource(ctx, testBuildSuccess).Return(nil),
			mockResourceManager.EXPECT().HasResourcesCompletedSuccessfully(ctx, testBuildFailure).Return(false, nil),
			mockResourceManager.EXPECT().HasResourcesCompletedSuccessfully(ctx, testBuildRunning).Return(false, nil),
			mockResourceManager.EXPECT().HasResourcesCompletedSuccessfully(ctx, testBuildError).Return(false, nil),
			mockResourceManager.EXPECT().HasResourcesCompletedSuccessfully(ctx, testBuildPending).Return(false, nil),
		)

		res, err := mgr.GarbageCollect(ctx, mbscName, mbscNamespace, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(BeNil())
		Expect(res).To(Equal([]string{"buildSuccess"}))
	})
})
