package build

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("OpenShiftBuildsHelper_GetBuild", func() {
	const (
		buildType    = "build-type"
		targetKernel = "target-kernels"
	)

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

		osbh := NewOpenShiftBuildsHelper(mockKubeClient, buildType)
		mld := api.ModuleLoaderData{
			KernelVersion: targetKernel,
		}

		_, err := osbh.GetModuleBuildByKernel(ctx, &mld)

		Expect(err).To(HaveOccurred())
	})

	It("should return an error if there are two Builids with the same labels", func() {
		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildList, _ ...ctrlclient.ListOption) {
				bcs.Items = make([]buildv1.Build, 2)
			})

		osbh := NewOpenShiftBuildsHelper(mockKubeClient, buildType)
		mld := api.ModuleLoaderData{
			KernelVersion: targetKernel,
		}

		_, err := osbh.GetModuleBuildByKernel(ctx, &mld)

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

		osbh := NewOpenShiftBuildsHelper(mockKubeClient, buildType)
		mld := api.ModuleLoaderData{
			KernelVersion: targetKernel,
		}

		res, err := osbh.GetModuleBuildByKernel(ctx, &mld)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(bc))
	})
})
