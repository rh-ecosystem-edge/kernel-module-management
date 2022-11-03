package controllers

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimectrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("NodeKernelReconciler_Reconcile", func() {

	const (
		kernelVersion = "1.2.3"
		osVersion     = "411.86.202210072320-0"
		labelName     = "label-name"
		nodeName      = "node-name"
	)

	var (
		gCtrl     *gomock.Controller
		clnt      *client.MockClient
		mockSKODM *syncronizedmap.MockKernelOsDtkMapping
		osImage   = fmt.Sprintf("Red Hat Enterprise Linux CoreOS %s (Ootpa)", osVersion)
	)

	BeforeEach(func() {
		gCtrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(gCtrl)
		mockSKODM = syncronizedmap.NewMockKernelOsDtkMapping(gCtrl)
	})

	It("should return an error if the node cannot be found anymore", func() {
		ctx := context.Background()
		clnt.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(errors.New("some error"))

		nkr := NewNodeKernelReconciler(clnt, labelName, nil, nil)
		req := runtimectrl.Request{
			NamespacedName: types.NamespacedName{Name: nodeName},
		}

		_, err := nkr.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())
	})

	It("should set the label if it does not exist", func() {
		node := v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			Status: v1.NodeStatus{
				NodeInfo: v1.NodeSystemInfo{
					KernelVersion: kernelVersion,
					OSImage:       osImage,
				},
			},
		}

		ctx := context.Background()
		gomock.InOrder(
			clnt.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ interface{}, _ interface{}, n *v1.Node) error {
					n.ObjectMeta = node.ObjectMeta
					n.Status = node.Status
					return nil
				},
			),
			mockSKODM.EXPECT().SetNodeInfo(kernelVersion, osVersion),
			clnt.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()),
		)

		nkr := NewNodeKernelReconciler(clnt, labelName, nil, mockSKODM)
		req := runtimectrl.Request{
			NamespacedName: types.NamespacedName{Name: nodeName},
		}

		res, err := nkr.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(res))
	})

	It("should set the label if it already exists", func() {
		node := v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   nodeName,
				Labels: map[string]string{kernelVersion: "4.5.6"},
			},
			Status: v1.NodeStatus{
				NodeInfo: v1.NodeSystemInfo{
					KernelVersion: kernelVersion,
					OSImage:       osImage,
				},
			},
		}

		ctx := context.Background()
		gomock.InOrder(
			clnt.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ interface{}, _ interface{}, n *v1.Node) error {
					n.ObjectMeta = node.ObjectMeta
					n.Status = node.Status
					return nil
				},
			),
			mockSKODM.EXPECT().SetNodeInfo(kernelVersion, osVersion),
			clnt.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()),
		)

		nkr := NewNodeKernelReconciler(clnt, labelName, nil, mockSKODM)
		req := runtimectrl.Request{
			NamespacedName: types.NamespacedName{Name: nodeName},
		}

		res, err := nkr.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(res))
	})

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
				func(_ interface{}, _ interface{}, n *v1.Node) error {
					n.ObjectMeta = node.ObjectMeta
					n.Status = node.Status
					return nil
				},
			),
		)

		nkr := NewNodeKernelReconciler(clnt, labelName, nil, nil)
		req := runtimectrl.Request{
			NamespacedName: types.NamespacedName{Name: nodeName},
		}

		_, err := nkr.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())

	})
})
