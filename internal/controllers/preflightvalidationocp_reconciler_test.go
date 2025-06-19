package controllers

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta2"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("PreflightValidationOCPReconciler", func() {
	var (
		mockCtrl   *gomock.Controller
		mockHelper *MockpreflightOCPReconcilerHelper
		p          *preflightValidationOCPReconciler
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockHelper = NewMockpreflightOCPReconcilerHelper(mockCtrl)
		p = &preflightValidationOCPReconciler{
			filter: nil,
			helper: mockHelper,
		}
	})

	ctx := context.Background()
	pvo := &v1beta2.PreflightValidationOCP{}

	DescribeTable("check good and error flows", func(preparePVFailed, updateStatusFailed bool) {
		returnedError := errors.New("some error")

		mockHelper.EXPECT().setDTKMapping(pvo).Return()
		if preparePVFailed {
			mockHelper.EXPECT().preparePreflightValidation(ctx, pvo).Return(returnedError)
			goto executeTestFunction
		}
		mockHelper.EXPECT().preparePreflightValidation(ctx, pvo).Return(nil)
		if updateStatusFailed {
			mockHelper.EXPECT().updateStatus(ctx, pvo).Return(returnedError)
			goto executeTestFunction
		}
		mockHelper.EXPECT().updateStatus(ctx, pvo).Return(nil)

	executeTestFunction:
		_, err := p.Reconcile(ctx, pvo)
		if preparePVFailed || updateStatusFailed {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).To(BeNil())
		}
	},
		Entry("preparePreflightValidation failed", true, false),
		Entry("updateStatus failed", false, true),
		Entry("good flow", false, false),
	)
})

var _ = Describe("setDTKMapping", func() {
	var (
		mockCtrl  *gomock.Controller
		mockSKODM *syncronizedmap.MockKernelOsDtkMapping
		p         preflightOCPReconcilerHelper
		pvo       *v1beta2.PreflightValidationOCP
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockSKODM = syncronizedmap.NewMockKernelOsDtkMapping(mockCtrl)
		p = newPreflightOCPReconcilerHelper(nil, mockSKODM, nil)
		pvo = &v1beta2.PreflightValidationOCP{
			Spec: v1beta2.PreflightValidationOCPSpec{
				KernelVersion: "some kernel version",
			},
		}
	})

	It("DTKImage is not set in PreflightValidationOCP", func() {
		p.setDTKMapping(pvo)
	})

	It("DTK is not present in the mapping", func() {
		pvo.Spec.DTKImage = "some image"
		gomock.InOrder(
			mockSKODM.EXPECT().GetImage("some kernel version").Return("", fmt.Errorf("some error")),
			mockSKODM.EXPECT().SetNodeInfo("some kernel version", preflightOSVersionValue),
			mockSKODM.EXPECT().SetImageStreamInfo(preflightOSVersionValue, "some image"),
		)
		p.setDTKMapping(pvo)
	})
})

var _ = Describe("updateStatus", func() {
	var (
		mockCtrl         *gomock.Controller
		mockClient       *client.MockClient
		mockStatusWriter *client.MockStatusWriter
		p                preflightOCPReconcilerHelper
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = client.NewMockClient(mockCtrl)
		mockStatusWriter = client.NewMockStatusWriter(mockCtrl)
		p = newPreflightOCPReconcilerHelper(mockClient, nil, nil)
	})

	ctx := context.Background()
	pvo := &v1beta2.PreflightValidationOCP{
		ObjectMeta: metav1.ObjectMeta{Name: "some name", Namespace: "some namespace"},
	}

	It("failed to get the preflightValidation", func() {
		mockClient.EXPECT().Get(ctx, types.NamespacedName{Name: "some name", Namespace: "some namespace"}, gomock.Any()).Return(fmt.Errorf("some error"))

		err := p.updateStatus(ctx, pvo)
		Expect(err).To(HaveOccurred())
	})

	It("update status successful", func() {
		gomock.InOrder(
			mockClient.EXPECT().Get(ctx, types.NamespacedName{Name: "some name", Namespace: "some namespace"}, gomock.Any()).Return(nil),
			mockClient.EXPECT().Status().Return(mockStatusWriter),
			mockStatusWriter.EXPECT().Patch(ctx, pvo, gomock.Any()).Return(nil),
		)

		err := p.updateStatus(ctx, pvo)
		Expect(err).To(BeNil())
	})
})
