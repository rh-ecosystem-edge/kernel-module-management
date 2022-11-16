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
	runtimectrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("ImageStreamReconciler_Reconcile", func() {

	var (
		ctx       context.Context
		gCtrl     *gomock.Controller
		clnt      *client.MockClient
		mockSKODM *syncronizedmap.MockKernelOsDtkMapping
	)

	BeforeEach(func() {
		ctx = context.Background()
		gCtrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(gCtrl)
		mockSKODM = syncronizedmap.NewMockKernelOsDtkMapping(gCtrl)
	})

	It("should return an error if the imagestream isn't reachable", func() {

		isr := NewImageStreamReconciler(clnt, nil, nil)

		clnt.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(errors.New("some error"))

		_, err := isr.Reconcile(ctx, runtimectrl.Request{})
		Expect(err).To(HaveOccurred())
	})

	It("should work as expected", func() {

		isr := NewImageStreamReconciler(clnt, nil, mockSKODM)

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
			clnt.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ interface{}, _ interface{}, is *imagev1.ImageStream) error {
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
