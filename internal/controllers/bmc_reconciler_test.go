package controllers

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("BMCReconcile", func() {
	var (
		ctrl               *gomock.Controller
		mockBmcReconHelper *MockbmcReconcilerHelperAPI
		br                 *bmcReconciler
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockBmcReconHelper = NewMockbmcReconcilerHelperAPI(ctrl)

		br = &bmcReconciler{
			reconHelper: mockBmcReconHelper,
		}
	})

	ctx := context.Background()
	bmcObj := &kmmv1beta1.BootModuleConfig{}

	DescribeTable("check good and error flows", func(setFinalizerError, machineConfigurationError, machineConfigError bool) {
		returnedError := errors.New("some error")
		expectedErr := returnedError
		if setFinalizerError {
			mockBmcReconHelper.EXPECT().setFinalizer(ctx, bmcObj).Return(returnedError)
			goto executeTestFunction
		}
		mockBmcReconHelper.EXPECT().setFinalizer(ctx, bmcObj).Return(nil)
		if machineConfigurationError {
			mockBmcReconHelper.EXPECT().handleMachineConfiguration(ctx, bmcObj).Return(returnedError)
			goto executeTestFunction
		}
		mockBmcReconHelper.EXPECT().handleMachineConfiguration(ctx, bmcObj).Return(nil)
		if machineConfigError {
			mockBmcReconHelper.EXPECT().handleMachineConfig(ctx, bmcObj).Return(returnedError)
			goto executeTestFunction
		}
		mockBmcReconHelper.EXPECT().handleMachineConfig(ctx, bmcObj).Return(nil)
		expectedErr = nil

	executeTestFunction:
		res, err := br.Reconcile(ctx, bmcObj)

		Expect(res).To(Equal(reconcile.Result{}))
		if expectedErr != nil {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).To(BeNil())
		}
	},
		Entry("setFinalizer failed", true, false, false),
		Entry("handleMachineConfiguration failed", false, true, false),
		Entry("handleMachineConfig failed", false, false, true),
		Entry("everything worked", false, false, false),
	)

	It("bmc is being deleted", func() {
		delTime := metav1.Now()
		bmcObj.SetDeletionTimestamp(&delTime)
		mockBmcReconHelper.EXPECT().finalizeBMC(ctx, bmcObj).Return(nil)

		res, err := br.Reconcile(ctx, bmcObj)

		bmcObj.SetDeletionTimestamp(nil)

		Expect(res).To(Equal(reconcile.Result{}))
		Expect(err).To(BeNil())
	})
})
