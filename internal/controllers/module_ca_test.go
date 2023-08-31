package controllers

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	testclient "github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/ocp/ca"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ModuleCAReconciler_Reconcile", func() {
	const operatorNamespace = "operator-ns"

	ctx := context.TODO()

	DescribeTable(
		"should sync if the Module is not in the operator namespace",
		func(moduleNamespace string, shouldSync bool) {
			ctrl := gomock.NewController(GinkgoT())
			mockClient := testclient.NewMockClient(ctrl)
			mockCAHelper := ca.NewMockHelper(ctrl)

			r := NewModuleCAReconciler(mockClient, mockCAHelper, operatorNamespace)

			mod := kmmv1beta1.Module{}
			nsn := types.NamespacedName{Namespace: moduleNamespace}
			req := reconcile.Request{NamespacedName: nsn}

			moduleGet := mockClient.EXPECT().Get(ctx, nsn, &mod)

			if shouldSync {
				mockCAHelper.EXPECT().Sync(ctx, moduleNamespace, &mod).After(moduleGet)
			}

			Expect(
				r.Reconcile(ctx, req),
			).To(
				Equal(reconcile.Result{}),
			)
		},
		Entry("Module in the operator namespace", operatorNamespace, false),
		Entry("Module in another namespace", "some-other-namespace", true),
	)
})
