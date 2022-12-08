package controllers

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	imagev1 "github.com/openshift/api/image/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimectrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ImageStreamReconciler_Reconcile", func() {

	var (
		ctx       context.Context
		gCtrl     *gomock.Controller
		clnt      *client.MockClient
		mockSKODM *syncronizedmap.MockKernelOsDtkMapping
		nsn       = types.NamespacedName{Namespace: "namespace", Name: "name"}
	)

	BeforeEach(func() {
		ctx = context.Background()
		gCtrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(gCtrl)
		mockSKODM = syncronizedmap.NewMockKernelOsDtkMapping(gCtrl)
	})

	It("should return an error if the imagestream isn't reachable", func() {

		isr := NewImageStreamReconciler(clnt, nil, nsn)

		clnt.EXPECT().Get(ctx, nsn, gomock.Any()).Return(errors.New("some error"))

		_, err := isr.Reconcile(ctx, runtimectrl.Request{})
		Expect(err).To(HaveOccurred())
	})

	It("should work as expected", func() {

		isr := NewImageStreamReconciler(clnt, mockSKODM, nsn)

		isSpec := imagev1.ImageStreamSpec{
			Tags: []imagev1.TagReference{
				{
					Name: "411.86.202210072320-0",
					From: &v1.ObjectReference{
						Name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:<some digest>",
					},
				},
			},
		}

		gomock.InOrder(
			clnt.EXPECT().Get(ctx, nsn, gomock.Any()).DoAndReturn(
				func(_ interface{}, _ interface{}, is *imagev1.ImageStream, _ ...ctrlclient.GetOption) error {
					is.Spec = isSpec
					return nil
				},
			),
			mockSKODM.EXPECT().SetImageStreamInfo(gomock.Any(), gomock.Any()),
		)

		_, err := isr.Reconcile(ctx, runtimectrl.Request{})
		Expect(err).NotTo(HaveOccurred())
	})
})
