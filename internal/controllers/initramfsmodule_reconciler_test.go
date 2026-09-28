package controllers

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/mic"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/node"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	irmName   = "nic"
	irmNS     = "ns"
	imageRepo = "registry.example.com/kmods/nic:v1"
)

var workerSelector = map[string]string{"node-role.kubernetes.io/worker": ""}

func nodeWithKernel(name, kernel string) v1.Node {
	return v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status:     v1.NodeStatus{NodeInfo: v1.NodeSystemInfo{KernelVersion: kernel}},
	}
}

func newIRM(selector map[string]string, build *kmmv1beta1.Build, sign *kmmv1beta1.Sign) *kmmv1beta1.InitramfsModule {
	return &kmmv1beta1.InitramfsModule{
		ObjectMeta: metav1.ObjectMeta{Name: irmName, Namespace: irmNS},
		Spec: kmmv1beta1.InitramfsModuleSpec{
			Selector:        selector,
			ModuleNames:     []string{"nic"},
			ContainerImage:  imageRepo,
			Build:           build,
			Sign:            sign,
			DirName:         "/opt/modules",
			ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-secret"},
		},
	}
}

func imageSpec(kernel string, build *kmmv1beta1.Build, sign *kmmv1beta1.Sign, irm *kmmv1beta1.InitramfsModule) kmmv1beta1.ModuleImageSpec {
	return kmmv1beta1.ModuleImageSpec{
		Image:         imageRepo + "-" + kernel,
		KernelVersion: kernel,
		Build:         build,
		Sign:          sign,
		DirName:       irm.Spec.DirName,
	}
}

var _ = Describe("InitramfsModuleReconciler_Reconcile", func() {
	var (
		ctrl       *gomock.Controller
		mockHelper *MockinitramfsModuleReconcilerHelper
		r          *initramfsModuleReconciler
		ctx        context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockHelper = NewMockinitramfsModuleReconcilerHelper(ctrl)
		r = &initramfsModuleReconciler{helper: mockHelper}
		ctx = context.Background()
	})

	It("calls only finalize when the CR is deleting", func() {
		irm := newIRM(nil, nil, nil)
		now := metav1.Now()
		irm.DeletionTimestamp = &now
		mockHelper.EXPECT().finalize(ctx, irm).Return(nil)

		res, err := r.Reconcile(ctx, irm)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(reconcile.Result{}))
	})

	It("returns the finalize error", func() {
		irm := newIRM(nil, nil, nil)
		now := metav1.Now()
		irm.DeletionTimestamp = &now
		mockHelper.EXPECT().finalize(ctx, irm).Return(errors.New("finalize failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("finalize failed"))
	})

	It("returns the list error and does not build images", func() {
		irm := newIRM(nil, nil, nil)
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nil, errors.New("list failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("list failed"))
	})

	It("calls handleMIC, then the pool, then the MachineConfig", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		gomock.InOrder(
			mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil),
			mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil),
			mockHelper.EXPECT().handleMachineConfigPool(ctx, irm).Return(nil),
			mockHelper.EXPECT().handleMachineConfig(ctx, irm).Return(nil),
		)

		res, err := r.Reconcile(ctx, irm)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(reconcile.Result{}))
	})

	It("returns the handleMIC error", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil)
		mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(errors.New("apply failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("apply failed"))
	})

	It("returns the MachineConfigPool error", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil)
		mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil)
		mockHelper.EXPECT().handleMachineConfigPool(ctx, irm).Return(errors.New("mcp failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("mcp failed"))
	})

	It("returns the MachineConfig error", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil)
		mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil)
		mockHelper.EXPECT().handleMachineConfigPool(ctx, irm).Return(nil)
		mockHelper.EXPECT().handleMachineConfig(ctx, irm).Return(errors.New("mc failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("mc failed"))
	})
})

var _ = Describe("initramfsModuleReconcilerHelper", func() {
	var (
		ctrl     *gomock.Controller
		mockNode *node.MockNode
		mockMIC  *mic.MockMIC
		h        initramfsModuleReconcilerHelper
		ctx      context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockNode = node.NewMockNode(ctrl)
		mockMIC = mic.NewMockMIC(ctrl)
		h = newInitramfsModuleReconcilerHelper(nil, mockMIC, mockNode)
		ctx = context.Background()
	})

	expectPatch := func(irm *kmmv1beta1.InitramfsModule, images []kmmv1beta1.ModuleImageSpec, ret error) {
		mockMIC.EXPECT().CreateOrPatch(
			ctx, "irm-"+irmName, irmNS, images, irm.Spec.ImageRepoSecret,
			v1.PullPolicy(""), true, nil, nil, irm,
		).Return(ret)
	}

	It("lists worker nodes when the selector is nil", func() {
		mockNode.EXPECT().GetAllNodesBySelector(ctx, workerSelector).Return([]v1.Node{}, nil)

		nodes, err := h.listSelectedNodes(ctx, newIRM(nil, nil, nil))

		Expect(err).NotTo(HaveOccurred())
		Expect(nodes).To(BeEmpty())
	})

	It("lists worker nodes when the selector is empty", func() {
		mockNode.EXPECT().GetAllNodesBySelector(ctx, workerSelector).Return([]v1.Node{}, nil)

		nodes, err := h.listSelectedNodes(ctx, newIRM(map[string]string{}, nil, nil))

		Expect(err).NotTo(HaveOccurred())
		Expect(nodes).To(BeEmpty())
	})

	It("passes a custom selector through", func() {
		selector := map[string]string{"kmm-test": "initramfs"}
		mockNode.EXPECT().GetAllNodesBySelector(ctx, selector).Return(nil, errors.New("list failed"))

		_, err := h.listSelectedNodes(ctx, newIRM(selector, nil, nil))

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("list failed"))
	})

	It("does not patch the MIC when no nodes match", func() {
		Expect(h.handleMIC(ctx, newIRM(nil, nil, nil), nil)).To(Succeed())
	})

	It("does not patch the MIC when every kernel is empty", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("empty", "")}

		Expect(h.handleMIC(ctx, irm, nodes)).To(Succeed())
	})

	It("replaces + in the kernel and sets the MIC image fields", func() {
		irm := newIRM(nil, nil, nil)
		images := []kmmv1beta1.ModuleImageSpec{imageSpec("6.1.0-1-foo-bar", nil, nil, irm)}
		expectPatch(irm, images, nil)

		Expect(h.handleMIC(ctx, irm, []v1.Node{nodeWithKernel("n1", "6.1.0-1+foo+bar")})).To(Succeed())
	})

	It("sorts unique kernels and copies build, sign, and dirName", func() {
		build := &kmmv1beta1.Build{
			DockerfileConfigMap: &v1.LocalObjectReference{Name: "dockerfile"},
		}
		sign := &kmmv1beta1.Sign{
			KeySecret:  &v1.LocalObjectReference{Name: "key"},
			CertSecret: &v1.LocalObjectReference{Name: "cert"},
		}
		irm := newIRM(nil, build, sign)
		nodes := []v1.Node{
			nodeWithKernel("b", "6.1.0-1+foo+bar"),
			nodeWithKernel("a", "5.14.0-427.el9.x86_64"),
			nodeWithKernel("c", "6.1.0-1+foo+bar"),
		}
		expectPatch(irm, []kmmv1beta1.ModuleImageSpec{
			imageSpec("5.14.0-427.el9.x86_64", build, sign, irm),
			imageSpec("6.1.0-1-foo-bar", build, sign, irm),
		}, nil)

		Expect(h.handleMIC(ctx, irm, nodes)).To(Succeed())
	})

	It("skips nodes whose kernel version is empty", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{
			nodeWithKernel("empty", ""),
			nodeWithKernel("n1", "6.1.0"),
		}
		expectPatch(irm, []kmmv1beta1.ModuleImageSpec{imageSpec("6.1.0", nil, nil, irm)}, nil)

		Expect(h.handleMIC(ctx, irm, nodes)).To(Succeed())
	})

	It("returns the CreateOrPatch error", func() {
		irm := newIRM(nil, nil, nil)
		expectPatch(irm, []kmmv1beta1.ModuleImageSpec{imageSpec("6.1.0", nil, nil, irm)}, errors.New("apply failed"))

		err := h.handleMIC(ctx, irm, []v1.Node{nodeWithKernel("n1", "6.1.0")})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("apply failed"))
	})

	It("finalizes without calling the API", func() {
		Expect(h.finalize(ctx, newIRM(nil, nil, nil))).To(Succeed())
	})

	It("handles the MachineConfigPool without calling the API", func() {
		Expect(h.handleMachineConfigPool(ctx, newIRM(nil, nil, nil))).To(Succeed())
	})

	It("handles the MachineConfig without calling the API", func() {
		Expect(h.handleMachineConfig(ctx, newIRM(nil, nil, nil))).To(Succeed())
	})
})
