package ocpbuild

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("OCPBuildsHelper_GetModuleOCPBuildByKernel", func() {
	const (
		buildType    = "build-type"
		targetKernel = "target-kernels"
	)

	var (
		mockKubeClient *client.MockClient
		osbh           OCPBuildsHelper
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		osbh = NewOCPBuildsHelper(mockKubeClient, buildType)

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

		mld := api.ModuleLoaderData{
			KernelVersion: targetKernel,
		}

		_, err := osbh.GetModuleOCPBuildByKernel(ctx, &mld, &mod)

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

		mld := api.ModuleLoaderData{
			KernelVersion: targetKernel,
		}

		_, err = osbh.GetModuleOCPBuildByKernel(ctx, &mld, &mod)

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

		mld := api.ModuleLoaderData{
			KernelVersion: targetKernel,
		}

		res, err := osbh.GetModuleOCPBuildByKernel(ctx, &mld, &mod)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(&build))
	})
})

var _ = Describe("OCPBuildsHelper_GetOCPBuilds", func() {
	const (
		buildType       = "build-type"
		moduleName      = "moduleName"
		moduleNamespace = "moduleNamespace"
	)

	var (
		mockKubeClient *client.MockClient
		osbh           OCPBuildsHelper
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		osbh = NewOCPBuildsHelper(mockKubeClient, buildType)

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

		_, err := osbh.GetModuleOCPBuilds(ctx, moduleName, moduleNamespace, &mod)

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

		res, err := osbh.GetModuleOCPBuilds(ctx, moduleName, moduleNamespace, &mod)

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

		res, err := osbh.GetModuleOCPBuilds(ctx, moduleName, moduleNamespace, &mod)

		Expect(err).NotTo(HaveOccurred())
		Expect(len(res)).To(Equal(0))
	})
})

var _ = Describe("OCPBuildsHelper_DeleteOCPBuild", func() {
	const buildType = "build-type"

	var (
		ctrl           *gomock.Controller
		mockKubeClient *client.MockClient
		osbh           OCPBuildsHelper
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		osbh = NewOCPBuildsHelper(mockKubeClient, buildType)
	})

	ctx := context.Background()

	It("good flow", func() {
		build := buildv1.Build{}
		opts := []ctrlclient.DeleteOption{
			ctrlclient.PropagationPolicy(metav1.DeletePropagationBackground),
		}
		mockKubeClient.EXPECT().Delete(ctx, &build, opts).Return(nil)

		err := osbh.DeleteOCPBuild(ctx, &build)

		Expect(err).NotTo(HaveOccurred())
	})

	It("error flow", func() {
		build := buildv1.Build{}

		opts := []ctrlclient.DeleteOption{
			ctrlclient.PropagationPolicy(metav1.DeletePropagationBackground),
		}
		mockKubeClient.EXPECT().Delete(ctx, &build, opts).Return(errors.New("random error"))

		err := osbh.DeleteOCPBuild(ctx, &build)

		Expect(err).To(HaveOccurred())
	})
})
