package ocpbuild

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
		mockSigner          *Mocksigner
		mockOCPBuildManager *MockocpbuildManager
		mgr                 *manager
	)
	const (
		mbscName      = "some-name"
		mbscNamespace = "some-namespace"
		kernelVersion = "some version"
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		mockSigner = NewMocksigner(ctrl)
		mockOCPBuildManager = NewMockocpbuildManager(ctrl)
		mgr = &manager{
			client:          clnt,
			signer:          mockSigner,
			ocpbuildManager: mockOCPBuildManager,
		}
	})

	ctx := context.Background()
	testMBSC := kmmv1beta1.ModuleBuildSignConfig{}

	It("failed flow, getModuleOCPBuildByKernel fails", func() {
		normalizedKernel := kernel.NormalizeVersion(kernelVersion)
		mockOCPBuildManager.EXPECT().getModuleOCPBuildByKernel(ctx, mbscName, mbscNamespace, normalizedKernel, ocpbuildTypeBuild, &testMBSC).
			Return(nil, fmt.Errorf("some error"))

		status, err := mgr.GetStatus(ctx, mbscName, mbscNamespace, kernelVersion, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
		Expect(status).To(Equal(kmmv1beta1.BuildOrSignStatus("")))
	})

	It("getModuleOCPBuildByKernel returns pod does not exists", func() {
		normalizedKernel := kernel.NormalizeVersion(kernelVersion)
		mockOCPBuildManager.EXPECT().getModuleOCPBuildByKernel(ctx, mbscName, mbscNamespace, normalizedKernel, ocpbuildTypeBuild, &testMBSC).
			Return(nil, ErrNoMatchingBuild)

		status, err := mgr.GetStatus(ctx, mbscName, mbscNamespace, kernelVersion, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(BeNil())
		Expect(status).To(Equal(kmmv1beta1.BuildOrSignStatus("")))
	})

	It("failed flow, getOCPBuildStatus fails", func() {
		foundBuild := buildv1.Build{}
		normalizedKernel := kernel.NormalizeVersion(kernelVersion)
		gomock.InOrder(
			mockOCPBuildManager.EXPECT().getModuleOCPBuildByKernel(ctx, mbscName, mbscNamespace, normalizedKernel, ocpbuildTypeBuild, &testMBSC).
				Return(&foundBuild, nil),
			mockOCPBuildManager.EXPECT().getOCPBuildStatus(&foundBuild).Return(Status(""), fmt.Errorf("some error")),
		)

		status, err := mgr.GetStatus(ctx, mbscName, mbscNamespace, kernelVersion, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
		Expect(status).To(Equal(kmmv1beta1.BuildOrSignStatus("")))
	})

	DescribeTable("check good flow and returned statuses", func(buildStatus Status, expectedStatus kmmv1beta1.BuildOrSignStatus) {
		foundBuild := buildv1.Build{}
		normalizedKernel := kernel.NormalizeVersion(kernelVersion)
		gomock.InOrder(
			mockOCPBuildManager.EXPECT().getModuleOCPBuildByKernel(ctx, mbscName, mbscNamespace, normalizedKernel, ocpbuildTypeBuild, &testMBSC).
				Return(&foundBuild, nil),
			mockOCPBuildManager.EXPECT().getOCPBuildStatus(&foundBuild).Return(buildStatus, nil),
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
		mockSigner          *Mocksigner
		mockOCPBuildManager *MockocpbuildManager
		mgr                 *manager
	)
	const (
		mbscName      = "some-name"
		mbscNamespace = "some-namespace"
		kernelVersion = "some version"
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		mockSigner = NewMocksigner(ctrl)
		mockOCPBuildManager = NewMockocpbuildManager(ctrl)
		mgr = &manager{
			client:          clnt,
			signer:          mockSigner,
			ocpbuildManager: mockOCPBuildManager,
		}
	})

	ctx := context.Background()
	testMBSC := kmmv1beta1.ModuleBuildSignConfig{}
	testMLD := &api.ModuleLoaderData{
		Name:                    mbscName,
		Namespace:               mbscNamespace,
		KernelNormalizedVersion: kernelVersion,
	}

	It("makeBuildTemplate failed", func() {
		By("test build action")
		mockOCPBuildManager.EXPECT().makeOCPBuildTemplate(ctx, testMLD, true, &testMBSC).Return(nil, fmt.Errorf("some error"))
		err := mgr.Sync(ctx, testMLD, true, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())

		By("test sign action")
		mockSigner.EXPECT().makeBuildTemplate(ctx, testMLD, true, &testMBSC).Return(nil, fmt.Errorf("some error"))
		err = mgr.Sync(ctx, testMLD, true, kmmv1beta1.SignImage, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("GetModulePodByKernel failed", func() {
		gomock.InOrder(
			mockOCPBuildManager.EXPECT().makeOCPBuildTemplate(ctx, testMLD, true, &testMBSC).Return(nil, nil),
			mockOCPBuildManager.EXPECT().getModuleOCPBuildByKernel(ctx, mbscName, mbscNamespace, kernelVersion, ocpbuildTypeBuild, &testMBSC).
				Return(nil, fmt.Errorf("some error")),
		)
		err := mgr.Sync(ctx, testMLD, true, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("CreateOCPBuild failed", func() {
		testTemplate := buildv1.Build{}
		gomock.InOrder(
			mockOCPBuildManager.EXPECT().makeOCPBuildTemplate(ctx, testMLD, true, &testMBSC).Return(&testTemplate, nil),
			mockOCPBuildManager.EXPECT().getModuleOCPBuildByKernel(ctx, mbscName, mbscNamespace, kernelVersion, ocpbuildTypeBuild, &testMBSC).
				Return(nil, ErrNoMatchingBuild),
			mockOCPBuildManager.EXPECT().createOCPBuild(ctx, &testTemplate).Return(fmt.Errorf("some error")),
		)
		err := mgr.Sync(ctx, testMLD, true, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("isOCPBuildChanged failed", func() {
		testTemplate := buildv1.Build{}
		testBuild := buildv1.Build{}
		gomock.InOrder(
			mockOCPBuildManager.EXPECT().makeOCPBuildTemplate(ctx, testMLD, true, &testMBSC).Return(&testTemplate, nil),
			mockOCPBuildManager.EXPECT().getModuleOCPBuildByKernel(ctx, mbscName, mbscNamespace, kernelVersion, ocpbuildTypeBuild, &testMBSC).
				Return(&testBuild, nil),
			mockOCPBuildManager.EXPECT().isOCPBuildChanged(&testBuild, &testTemplate).Return(false, fmt.Errorf("some error")),
		)
		err := mgr.Sync(ctx, testMLD, true, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("deleteOCPBuild failed should not cause failure", func() {
		testTemplate := buildv1.Build{}
		testBuild := buildv1.Build{}
		gomock.InOrder(
			mockOCPBuildManager.EXPECT().makeOCPBuildTemplate(ctx, testMLD, true, &testMBSC).Return(&testTemplate, nil),
			mockOCPBuildManager.EXPECT().getModuleOCPBuildByKernel(ctx, mbscName, mbscNamespace, kernelVersion, ocpbuildTypeBuild, &testMBSC).
				Return(&testBuild, nil),
			mockOCPBuildManager.EXPECT().isOCPBuildChanged(&testBuild, &testTemplate).Return(true, nil),
			mockOCPBuildManager.EXPECT().deleteOCPBuild(ctx, &testBuild).Return(fmt.Errorf("some error")),
		)
		err := mgr.Sync(ctx, testMLD, true, kmmv1beta1.BuildImage, &testMBSC)
		Expect(err).To(BeNil())
	})

	DescribeTable("check good flow", func(buildAction, buildExists, buildChanged, pushImage bool) {
		testAction := kmmv1beta1.BuildImage
		testBuildTemplate := buildv1.Build{}
		existingTestBuild := buildv1.Build{}
		buildType := ocpbuildTypeBuild
		if !buildAction {
			buildType = ocpbuildTypeSign
			testAction = kmmv1beta1.SignImage
		}

		if buildAction {
			mockOCPBuildManager.EXPECT().makeOCPBuildTemplate(ctx, testMLD, pushImage, &testMBSC).Return(&testBuildTemplate, nil)
		} else {
			mockSigner.EXPECT().makeBuildTemplate(ctx, testMLD, pushImage, &testMBSC).Return(&testBuildTemplate, nil)
		}
		var getBuildError error
		if !buildExists {
			getBuildError = ErrNoMatchingBuild
		}
		mockOCPBuildManager.EXPECT().getModuleOCPBuildByKernel(ctx, mbscName, mbscNamespace, kernelVersion, buildType, &testMBSC).
			Return(&existingTestBuild, getBuildError)
		if !buildExists {
			mockOCPBuildManager.EXPECT().createOCPBuild(ctx, &testBuildTemplate).Return(nil)
			goto executeTestFunction
		}
		mockOCPBuildManager.EXPECT().isOCPBuildChanged(&existingTestBuild, &testBuildTemplate).Return(buildChanged, nil)
		if buildChanged {
			mockOCPBuildManager.EXPECT().deleteOCPBuild(ctx, &existingTestBuild).Return(nil)
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
		mockSigner          *Mocksigner
		mockOCPBuildManager *MockocpbuildManager
		mgr                 *manager
	)
	const (
		mbscName      = "some-name"
		mbscNamespace = "some-namespace"
		kernelVersion = "some version"
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		mockSigner = NewMocksigner(ctrl)
		mockOCPBuildManager = NewMockocpbuildManager(ctrl)
		mgr = &manager{
			client:          clnt,
			signer:          mockSigner,
			ocpbuildManager: mockOCPBuildManager,
		}
	})

	ctx := context.Background()
	testMBSC := kmmv1beta1.ModuleBuildSignConfig{}

	It("failed to get module buildss", func() {
		mockOCPBuildManager.EXPECT().getModuleOCPBuilds(ctx, mbscName, mbscNamespace, ocpbuildTypeBuild, &testMBSC).Return(nil, fmt.Errorf("some error"))

		_, err := mgr.GarbageCollect(ctx, mbscName, mbscNamespace, ocpbuildTypeBuild, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("delete ocpbuild failed", func() {
		testBuild := buildv1.Build{
			Status: buildv1.BuildStatus{
				Phase: buildv1.BuildPhaseComplete,
			},
		}
		gomock.InOrder(
			mockOCPBuildManager.EXPECT().getModuleOCPBuilds(ctx, mbscName, mbscNamespace, ocpbuildTypeBuild, &testMBSC).
				Return([]buildv1.Build{testBuild}, nil),
			mockOCPBuildManager.EXPECT().deleteOCPBuild(ctx, &testBuild).Return(fmt.Errorf("some error")),
		)

		_, err := mgr.GarbageCollect(ctx, mbscName, mbscNamespace, ocpbuildTypeBuild, &testMBSC)
		Expect(err).To(HaveOccurred())
	})

	It("good flow", func() {
		testBuildSuccess := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildSuccess"},
			Status:     buildv1.BuildStatus{Phase: buildv1.BuildPhaseComplete},
		}
		testBuildFailure := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildFailure"},
			Status:     buildv1.BuildStatus{Phase: buildv1.BuildPhaseFailed},
		}
		testBuildRunning := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildRunning"},
			Status:     buildv1.BuildStatus{Phase: buildv1.BuildPhaseRunning},
		}
		testBuildError := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildError"},
			Status:     buildv1.BuildStatus{Phase: buildv1.BuildPhaseError},
		}
		testBuildPending := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildPending"},
			Status:     buildv1.BuildStatus{Phase: buildv1.BuildPhasePending},
		}
		returnedBuilds := []buildv1.Build{testBuildSuccess, testBuildFailure, testBuildRunning, testBuildError, testBuildPending}
		gomock.InOrder(
			mockOCPBuildManager.EXPECT().getModuleOCPBuilds(ctx, mbscName, mbscNamespace, ocpbuildTypeBuild, &testMBSC).
				Return(returnedBuilds, nil),
			mockOCPBuildManager.EXPECT().deleteOCPBuild(ctx, &testBuildSuccess).Return(nil),
		)

		res, err := mgr.GarbageCollect(ctx, mbscName, mbscNamespace, ocpbuildTypeBuild, &testMBSC)
		Expect(err).To(BeNil())
		Expect(res).To(Equal([]string{"buildSuccess"}))
	})
})
