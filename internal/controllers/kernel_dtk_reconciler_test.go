package controllers

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	clienttest "github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimectrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("NodeKernelReconciler_Reconcile", func() {
	var (
		gCtrl     *gomock.Controller
		clnt      *clienttest.MockClient
		mockSKODM *syncronizedmap.MockKernelOsDtkMapping
	)
	BeforeEach(func() {
		gCtrl = gomock.NewController(GinkgoT())
		clnt = clienttest.NewMockClient(gCtrl)
		mockSKODM = syncronizedmap.NewMockKernelOsDtkMapping(gCtrl)
	})
	const (
		kernelVersion = "1.2.3"
		nodeName      = "node-name"
	)

	It("should return an error if the node cannot be found anymore", func() {
		ctx := context.Background()
		clnt.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(errors.New("some error"))

		nkr := NewKernelDTKReconciler(clnt, nil)
		req := runtimectrl.Request{
			NamespacedName: types.NamespacedName{Name: nodeName},
		}

		_, err := nkr.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())
	})

	const osVersion = "411.86.202210072320-0"

	validOSImage := fmt.Sprintf("Red Hat Enterprise Linux CoreOS %s (Ootpa)", osVersion)

	DescribeTable(
		"should register the NodeInfo",
		func(statusKernelVersion, expectedKernelVersion, osImage string) {
			node := v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
				Status: v1.NodeStatus{
					NodeInfo: v1.NodeSystemInfo{
						KernelVersion: statusKernelVersion,
						OSImage:       osImage,
					},
				},
			}

			nsn := types.NamespacedName{Name: nodeName}

			ctx := context.Background()
			gomock.InOrder(
				clnt.EXPECT().Get(ctx, nsn, &v1.Node{}).DoAndReturn(
					func(_ interface{}, _ interface{}, n *v1.Node, _ ...ctrlclient.GetOption) error {
						n.Status = node.Status
						return nil
					},
				),
				mockSKODM.EXPECT().SetNodeInfo(expectedKernelVersion, osVersion),
			)

			nkr := NewKernelDTKReconciler(clnt, mockSKODM)
			req := runtimectrl.Request{NamespacedName: nsn}

			res, err := nkr.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Requeue).To(BeFalse())
		},
		Entry(nil, kernelVersion, kernelVersion, validOSImage),
		Entry(nil, kernelVersion+"+", kernelVersion, validOSImage),
	)

	It("should fail if it cannot parse the osImage version from the nodeInfo", func() {

		node := v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			Status: v1.NodeStatus{
				NodeInfo: v1.NodeSystemInfo{
					KernelVersion: kernelVersion,
					OSImage:       "Red Hat Enterprise Linux CoreOS MISSING VERSION (Ootpa)",
				},
			},
		}

		ctx := context.Background()
		gomock.InOrder(
			clnt.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ interface{}, _ interface{}, n *v1.Node, _ ...ctrlclient.GetOption) error {
					n.ObjectMeta = node.ObjectMeta
					n.Status = node.Status
					return nil
				},
			),
		)

		nkr := NewKernelDTKReconciler(clnt, nil)
		req := runtimectrl.Request{
			NamespacedName: types.NamespacedName{Name: nodeName},
		}

		_, err := nkr.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())

	})
})
