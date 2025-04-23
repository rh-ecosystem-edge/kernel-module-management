package ocpbuild

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("getModuleOCPBuildByKernel", func() {
	const targetKernel = "target-kernels"

	var (
		mockKubeClient *client.MockClient
		obm            ocpbuildManager
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		obm = newOCPBuildManager(mockKubeClient)

	})

	ctx := context.Background()
	mod := kmmv1beta1.Module{
		ObjectMeta: metav1.ObjectMeta{Name: "moduleName", Namespace: "moduleNamespace"},
	}

	It("should return an error if an error occurred", func() {
		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Return(errors.New("random error"))

		_, err := obm.getModuleOCPBuildByKernel(ctx, "moduleName", "moduleNamespace", targetKernel, ocpbuildTypeBuild, &mod)

		Expect(err).To(HaveOccurred())
	})

	It("should return an error if there are two Builds with the same labels and owner", func() {
		build1 := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildName", Namespace: "moduleNamespace"},
		}
		build2 := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildName", Namespace: "moduleNamespace"},
		}

		err := controllerutil.SetControllerReference(&mod, &build1, scheme)
		Expect(err).NotTo(HaveOccurred())
		err = controllerutil.SetControllerReference(&mod, &build2, scheme)
		Expect(err).NotTo(HaveOccurred())

		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildList, _ ...ctrlclient.ListOption) {
				bcs.Items = make([]buildv1.Build, 2)
			})

		_, err = obm.getModuleOCPBuildByKernel(ctx, "moduleName", "moduleNamespace", targetKernel, ocpbuildTypeSign, &mod)

		Expect(err).To(HaveOccurred())
	})

	It("should work as expected", func() {
		build := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildName", Namespace: "moduleNamespace"},
		}
		err := controllerutil.SetControllerReference(&mod, &build, scheme)
		Expect(err).NotTo(HaveOccurred())

		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildList, _ ...ctrlclient.ListOption) {
				bcs.Items = []buildv1.Build{build}
			})

		res, err := obm.getModuleOCPBuildByKernel(ctx, "moduleName", "moduleNamespace", targetKernel, ocpbuildTypeBuild, &mod)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(&build))
	})
})

var _ = Describe("getModuleOCPBuilds", func() {
	const (
		moduleName      = "moduleName"
		moduleNamespace = "moduleNamespace"
	)

	var (
		mockKubeClient *client.MockClient
		obm            ocpbuildManager
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		obm = newOCPBuildManager(mockKubeClient)

	})

	ctx := context.Background()
	mod := kmmv1beta1.Module{
		ObjectMeta: metav1.ObjectMeta{Name: moduleName, Namespace: moduleNamespace},
	}

	It("should return an error if an error occurred", func() {
		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Return(errors.New("random error"))

		_, err := obm.getModuleOCPBuilds(ctx, moduleName, moduleNamespace, ocpbuildTypeBuild, &mod)

		Expect(err).To(HaveOccurred())
	})

	It("should work as expected", func() {
		build1 := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildName1", Namespace: moduleNamespace},
		}
		build2 := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "buildName2", Namespace: moduleNamespace},
		}
		err := controllerutil.SetControllerReference(&mod, &build1, scheme)
		Expect(err).NotTo(HaveOccurred())
		err = controllerutil.SetControllerReference(&mod, &build2, scheme)
		Expect(err).NotTo(HaveOccurred())

		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildList, _ ...ctrlclient.ListOption) {
				bcs.Items = []buildv1.Build{build1, build2}
			})

		res, err := obm.getModuleOCPBuilds(ctx, moduleName, moduleNamespace, ocpbuildTypeSign, &mod)

		Expect(err).NotTo(HaveOccurred())
		Expect(res[0]).To(Equal(build1))
		Expect(res[1]).To(Equal(build2))
	})

	It("zero builds found", func() {
		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildList, _ ...ctrlclient.ListOption) {
				bcs.Items = []buildv1.Build{}
			})

		res, err := obm.getModuleOCPBuilds(ctx, moduleName, moduleNamespace, "build", &mod)

		Expect(err).NotTo(HaveOccurred())
		Expect(len(res)).To(Equal(0))
	})
})

var _ = Describe("deleteOCPBuild", func() {

	var (
		ctrl           *gomock.Controller
		mockKubeClient *client.MockClient
		obm            ocpbuildManager
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		obm = newOCPBuildManager(mockKubeClient)
	})

	ctx := context.Background()

	It("good flow", func() {
		build := buildv1.Build{}
		opts := []ctrlclient.DeleteOption{
			ctrlclient.PropagationPolicy(metav1.DeletePropagationBackground),
		}
		mockKubeClient.EXPECT().Delete(ctx, &build, opts).Return(nil)

		err := obm.deleteOCPBuild(ctx, &build)

		Expect(err).NotTo(HaveOccurred())
	})

	It("error flow", func() {
		build := buildv1.Build{}

		opts := []ctrlclient.DeleteOption{
			ctrlclient.PropagationPolicy(metav1.DeletePropagationBackground),
		}
		mockKubeClient.EXPECT().Delete(ctx, &build, opts).Return(errors.New("random error"))

		err := obm.deleteOCPBuild(ctx, &build)

		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("createOCPBuild", func() {
	var (
		ctrl           *gomock.Controller
		mockKubeClient *client.MockClient
		obm            ocpbuildManager
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		obm = newOCPBuildManager(mockKubeClient)
	})

	It("good flow", func() {
		ctx := context.Background()

		build := buildv1.Build{}
		mockKubeClient.EXPECT().Create(ctx, &build).Return(nil)

		err := obm.createOCPBuild(ctx, &build)

		Expect(err).NotTo(HaveOccurred())
	})

	It("error flow", func() {
		ctx := context.Background()

		build := buildv1.Build{}
		mockKubeClient.EXPECT().Create(ctx, &build).Return(errors.New("random error"))

		err := obm.createOCPBuild(ctx, &build)

		Expect(err).To(HaveOccurred())

	})
})

var _ = Describe("getOCPBuildStatus", func() {
	var (
		ctrl           *gomock.Controller
		mockKubeClient *client.MockClient
		obm            ocpbuildManager
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		obm = newOCPBuildManager(mockKubeClient)
	})

	DescribeTable("should return the correct status depending on the build status",
		func(b *buildv1.Build, expectedStatus Status, expectsErr bool) {

			res, err := obm.getOCPBuildStatus(b)
			if expectsErr {
				Expect(err).To(HaveOccurred())
				return
			}

			Expect(res).To(Equal(expectedStatus))
		},
		Entry("succeeded", &buildv1.Build{Status: buildv1.BuildStatus{Phase: buildv1.BuildPhaseComplete}}, StatusCompleted, false),
		Entry("in progress", &buildv1.Build{Status: buildv1.BuildStatus{Phase: buildv1.BuildPhaseRunning}}, StatusInProgress, false),
		Entry("pending", &buildv1.Build{Status: buildv1.BuildStatus{Phase: buildv1.BuildPhasePending}}, StatusInProgress, false),
		Entry("failed", &buildv1.Build{Status: buildv1.BuildStatus{Phase: buildv1.BuildPhaseFailed}}, StatusFailed, false),
		Entry("error", &buildv1.Build{Status: buildv1.BuildStatus{Phase: buildv1.BuildPhaseError}}, StatusFailed, false),
		Entry("cancelled", &buildv1.Build{Status: buildv1.BuildStatus{Phase: buildv1.BuildPhaseCancelled}}, StatusFailed, false),
		Entry("unknown", &buildv1.Build{Status: buildv1.BuildStatus{Phase: "unknown"}}, StatusFailed, true),
	)
})

var _ = Describe("isOCPBuildChanged", func() {
	var (
		ctrl           *gomock.Controller
		mockKubeClient *client.MockClient
		obm            ocpbuildManager
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		obm = newOCPBuildManager(mockKubeClient)
	})

	DescribeTable("should detect if a build has changed",
		func(annotation map[string]string, expectchanged bool, expectsErr bool) {
			existingBuild := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotation,
				},
			}
			newBuild := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{hashAnnotation: "some hash"},
				},
			}

			changed, err := obm.isOCPBuildChanged(&existingBuild, &newBuild)

			if expectsErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(expectchanged).To(Equal(changed))
		},

		Entry("should error if build has no annotations", nil, false, true),
		Entry("should return true if build has changed", map[string]string{hashAnnotation: "some other hash"}, true, false),
		Entry("should return false is build has not changed ", map[string]string{hashAnnotation: "some hash"}, false, false),
	)
})

var _ = Describe("ocpbuildLabels", func() {
	var (
		ctrl           *gomock.Controller
		mockKubeClient *client.MockClient
		obm            ocpbuildManager
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		obm = newOCPBuildManager(mockKubeClient)
	})

	It("get build labels", func() {
		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{Name: "moduleName"},
		}
		labels := obm.ocpbuildLabels(mod.Name, "targetKernel", ocpbuildTypeBuild)

		expected := map[string]string{
			"app.kubernetes.io/name":      "kmm",
			"app.kubernetes.io/component": ocpbuildTypeBuild,
			"app.kubernetes.io/part-of":   "kmm",
			constants.ModuleNameLabel:     "moduleName",
			constants.TargetKernelTarget:  "targetKernel",
			constants.BuildTypeLabel:      ocpbuildTypeBuild,
		}

		Expect(labels).To(Equal(expected))
	})
})
