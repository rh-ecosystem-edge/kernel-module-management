package controllers

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	kmmclient "github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/mcfg"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/mic"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/node"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	irmName   = "nic"
	irmNS     = "ns"
	imageRepo = "registry.example.com/kmods/nic:v1"
)

var (
	workerSelector     = map[string]string{"node-role.kubernetes.io/worker": ""}
	initramfsRoleLabel = map[string]string{mcfg.InitramfsNodeRoleLabel: ""}
)

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

	It("returns the node label error and does not build images", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil)
		mockHelper.EXPECT().handleNodeLabels(ctx, irm, nodes).Return(errors.New("label failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("label failed"))
	})

	It("handles the pool before the MachineOSConfig", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		gomock.InOrder(
			mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil),
			mockHelper.EXPECT().handleNodeLabels(ctx, irm, nodes).Return(nil),
			mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil),
			mockHelper.EXPECT().areImagesReady(ctx, irm).Return(true, nil),
			mockHelper.EXPECT().handleMachineConfigPool(ctx, irm).Return(nil),
			mockHelper.EXPECT().handleMachineOSConfig(ctx, irm).Return(nil),
		)

		res, err := r.Reconcile(ctx, irm)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(reconcile.Result{}))
	})

	It("returns the handleMIC error", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil)
		mockHelper.EXPECT().handleNodeLabels(ctx, irm, nodes).Return(nil)
		mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(errors.New("apply failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("apply failed"))
	})

	It("does not create the pool or MachineOSConfig when the image does not exist yet", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		gomock.InOrder(
			mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil),
			mockHelper.EXPECT().handleNodeLabels(ctx, irm, nodes).Return(nil),
			mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil),
			mockHelper.EXPECT().areImagesReady(ctx, irm).Return(false, nil),
		)

		res, err := r.Reconcile(ctx, irm)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(reconcile.Result{}))
	})

	It("returns the image status error and does not create the pool", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		gomock.InOrder(
			mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil),
			mockHelper.EXPECT().handleNodeLabels(ctx, irm, nodes).Return(nil),
			mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil),
			mockHelper.EXPECT().areImagesReady(ctx, irm).Return(false, errors.New("get failed")),
		)

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("get failed"))
	})

	It("returns the MachineConfigPool error", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil)
		mockHelper.EXPECT().handleNodeLabels(ctx, irm, nodes).Return(nil)
		mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil)
		mockHelper.EXPECT().areImagesReady(ctx, irm).Return(true, nil)
		mockHelper.EXPECT().handleMachineConfigPool(ctx, irm).Return(errors.New("mcp failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("mcp failed"))
	})

	It("returns the MachineOSConfig error", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil)
		mockHelper.EXPECT().handleNodeLabels(ctx, irm, nodes).Return(nil)
		mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil)
		mockHelper.EXPECT().areImagesReady(ctx, irm).Return(true, nil)
		mockHelper.EXPECT().handleMachineConfigPool(ctx, irm).Return(nil)
		mockHelper.EXPECT().handleMachineOSConfig(ctx, irm).Return(errors.New("mc failed"))

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
		h = newInitramfsModuleReconcilerHelper(nil, mockMIC, mockNode, nil)
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

})

var _ = Describe("areImagesReady", func() {
	var (
		ctrl    *gomock.Controller
		clnt    *kmmclient.MockClient
		mockMIC *mic.MockMIC
		h       initramfsModuleReconcilerHelper
		ctx     context.Context
		irm     *kmmv1beta1.InitramfsModule
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = kmmclient.NewMockClient(ctrl)
		mockMIC = mic.NewMockMIC(ctrl)
		h = newInitramfsModuleReconcilerHelper(clnt, mockMIC, nil, nil)
		ctx = context.Background()
		irm = newIRM(nil, nil, nil)
	})

	It("is not ready when the ModuleImagesConfig does not exist", func() {
		clnt.EXPECT().Get(ctx, client.ObjectKey{Name: "irm-" + irmName, Namespace: irmNS}, gomock.Any()).Return(notFound())

		ready, err := h.areImagesReady(ctx, irm)

		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeFalse())
	})

	It("returns the Get error", func() {
		clnt.EXPECT().Get(ctx, client.ObjectKey{Name: "irm-" + irmName, Namespace: irmNS}, gomock.Any()).Return(errors.New("get failed"))

		ready, err := h.areImagesReady(ctx, irm)

		Expect(err).To(MatchError(ContainSubstring("get failed")))
		Expect(ready).To(BeFalse())
	})

	It("is not ready when an image does not exist", func() {
		micObj := existingInitramfsMIC()
		expectGetMIC(clnt, ctx, micObj)
		mockMIC.EXPECT().DoAllImagesExist(gomock.Any()).Return(false)

		ready, err := h.areImagesReady(ctx, irm)

		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeFalse())
	})

	It("is ready when every image exists", func() {
		micObj := existingInitramfsMIC()
		expectGetMIC(clnt, ctx, micObj)
		mockMIC.EXPECT().DoAllImagesExist(gomock.Any()).DoAndReturn(func(got *kmmv1beta1.ModuleImagesConfig) bool {
			Expect(got.Name).To(Equal(micObj.Name))
			Expect(got.Status.ImagesStates).To(Equal(micObj.Status.ImagesStates))
			return true
		})

		ready, err := h.areImagesReady(ctx, irm)

		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue())
	})
})

var _ = Describe("handleNodeLabels", func() {
	var (
		ctrl     *gomock.Controller
		mockNode *node.MockNode
		h        initramfsModuleReconcilerHelper
		ctx      context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockNode = node.NewMockNode(ctrl)
		h = newInitramfsModuleReconcilerHelper(nil, nil, mockNode, nil)
		ctx = context.Background()
	})

	It("adds the initramfs role label to worker nodes when the selector is empty", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{
			nodeWithKernel("w1", "6.1.0"),
			nodeWithKernel("w2", "5.14.0"),
		}
		gomock.InOrder(
			mockNode.EXPECT().UpdateLabels(ctx, &nodes[0], initramfsRoleLabel, nil).Return(nil),
			mockNode.EXPECT().UpdateLabels(ctx, &nodes[1], initramfsRoleLabel, nil).Return(nil),
		)

		Expect(h.handleNodeLabels(ctx, irm, nodes)).To(Succeed())
	})

	It("adds the initramfs role label when the selector map is empty", func() {
		irm := newIRM(map[string]string{}, nil, nil)
		nodes := []v1.Node{nodeWithKernel("w1", "6.1.0")}
		mockNode.EXPECT().UpdateLabels(ctx, &nodes[0], initramfsRoleLabel, nil).Return(nil)

		Expect(h.handleNodeLabels(ctx, irm, nodes)).To(Succeed())
	})

	It("does not label nodes when the user set a selector", func() {
		irm := newIRM(map[string]string{"kmm-test": "initramfs"}, nil, nil)
		nodes := []v1.Node{nodeWithKernel("w1", "6.1.0")}

		Expect(h.handleNodeLabels(ctx, irm, nodes)).To(Succeed())
	})

	It("keeps labeling the remaining workers when one update fails", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{
			nodeWithKernel("w1", "6.1.0"),
			nodeWithKernel("w2", "6.1.0"),
		}
		gomock.InOrder(
			mockNode.EXPECT().UpdateLabels(ctx, &nodes[0], initramfsRoleLabel, nil).Return(errors.New("conflict")),
			mockNode.EXPECT().UpdateLabels(ctx, &nodes[1], initramfsRoleLabel, nil).Return(nil),
		)

		err := h.handleNodeLabels(ctx, irm, nodes)

		Expect(err).To(MatchError(ContainSubstring("w1")))
		Expect(err.Error()).NotTo(ContainSubstring("w2"))
	})
})

var _ = Describe("initramfs MachineConfigPool and MachineOSConfig", func() {
	var (
		ctx      context.Context
		ctrl     *gomock.Controller
		clnt     *kmmclient.MockClient
		mockMCFG *mcfg.MockMCFG
		h        initramfsModuleReconcilerHelper
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		clnt = kmmclient.NewMockClient(ctrl)
		mockMCFG = mcfg.NewMockMCFG(ctrl)
		h = newInitramfsModuleReconcilerHelper(clnt, nil, nil, mockMCFG)
	})

	It("creates the pool without an ownerReference", func() {
		irm := newIRM(nil, nil, nil)
		var pool *mcfgv1.MachineConfigPool
		gomock.InOrder(
			expectGetNotFound(clnt, ctx, mcfg.InitramfsPoolName, &mcfgv1.MachineConfigPool{}),
			mockMCFG.EXPECT().UpdateInitramfsPool(gomock.Any(), irm),
			expectCreate(clnt, ctx, func(obj client.Object) { pool = obj.(*mcfgv1.MachineConfigPool).DeepCopy() }),
		)

		Expect(h.handleMachineConfigPool(ctx, irm)).To(Succeed())
		Expect(pool.Name).To(Equal(mcfg.InitramfsPoolName))
		Expect(pool.OwnerReferences).To(BeEmpty())
	})

	It("patches an existing pool instead of creating it again", func() {
		irm := newIRM(nil, nil, nil)
		existing := &mcfgv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{
			Name: mcfg.InitramfsPoolName,
			UID:  "pool-uid",
		}}
		var patched *mcfgv1.MachineConfigPool
		gomock.InOrder(
			expectGetObject(clnt, ctx, existing),
			mockMCFG.EXPECT().UpdateInitramfsPool(gomock.Any(), irm).Do(
				func(pool *mcfgv1.MachineConfigPool, _ *kmmv1beta1.InitramfsModule) {
					pool.Annotations = map[string]string{"updated": "yes"}
				},
			),
			expectPatch(clnt, ctx, func(obj client.Object) { patched = obj.(*mcfgv1.MachineConfigPool).DeepCopy() }),
		)

		Expect(h.handleMachineConfigPool(ctx, irm)).To(Succeed())
		Expect(patched.UID).To(Equal(existing.UID))
		Expect(patched.Annotations).To(HaveKeyWithValue("updated", "yes"))
	})

	DescribeTable("returns pool API errors", func(failCreate bool) {
		irm := newIRM(nil, nil, nil)
		failure := errors.New("pool API failed")
		if failCreate {
			expectGetNotFound(clnt, ctx, mcfg.InitramfsPoolName, &mcfgv1.MachineConfigPool{})
			mockMCFG.EXPECT().UpdateInitramfsPool(gomock.Any(), irm)
			clnt.EXPECT().Create(ctx, gomock.AssignableToTypeOf(&mcfgv1.MachineConfigPool{})).Return(failure)
		} else {
			clnt.EXPECT().Get(ctx, client.ObjectKey{Name: mcfg.InitramfsPoolName}, gomock.AssignableToTypeOf(&mcfgv1.MachineConfigPool{})).Return(failure)
		}
		Expect(h.handleMachineConfigPool(ctx, irm)).To(MatchError(failure))
	}, Entry("get", false), Entry("create", true))

	DescribeTable("creates or patches the MachineOSConfig", func(exists, stamped bool) {
		irm := newIRM(nil, nil, nil)
		existing := &mcfgv1.MachineOSConfig{ObjectMeta: metav1.ObjectMeta{
			Name:        mcfg.InitramfsMachineOSConfigName,
			UID:         "mosc-uid",
			Annotations: map[string]string{"keep": "yes"},
		}, Spec: mcfgv1.MachineOSConfigSpec{RenderedImagePushSpec: "registry.example.com/os:existing"}}
		if stamped {
			existing.Annotations[mcfg.InitramfsModuleNamespaceAnnotation] = irm.Namespace
			existing.Annotations[mcfg.InitramfsModuleNameAnnotation] = irm.Name
		}
		var actual *mcfgv1.MachineOSConfig
		if exists {
			expectGetObject(clnt, ctx, existing)
		} else {
			expectGetNotFound(clnt, ctx, mcfg.InitramfsMachineOSConfigName, &mcfgv1.MachineOSConfig{})
		}
		mockMCFG.EXPECT().UpdateInitramfsMachineOSConfig(gomock.Any(), irm).DoAndReturn(
			func(mosc *mcfgv1.MachineOSConfig, irm *kmmv1beta1.InitramfsModule) error {
				return mcfg.NewMCFG("").UpdateInitramfsMachineOSConfig(mosc, irm)
			},
		)
		capture := func(obj client.Object) { actual = obj.(*mcfgv1.MachineOSConfig).DeepCopy() }
		if !exists {
			expectCreate(clnt, ctx, capture)
		} else if !stamped {
			expectPatch(clnt, ctx, capture)
		}

		Expect(h.handleMachineOSConfig(ctx, irm)).To(Succeed())

		if stamped {
			Expect(actual).To(BeNil())
			return
		}
		Expect(actual).NotTo(BeNil())
		Expect(actual.Name).To(Equal(mcfg.InitramfsMachineOSConfigName))
		Expect(actual.Namespace).To(BeEmpty())
		Expect(actual.OwnerReferences).To(BeEmpty())
		Expect(actual.Annotations).To(HaveKeyWithValue(mcfg.InitramfsModuleNamespaceAnnotation, irm.Namespace))
		Expect(actual.Annotations).To(HaveKeyWithValue(mcfg.InitramfsModuleNameAnnotation, irm.Name))
		if exists {
			Expect(actual.UID).To(Equal(existing.UID))
			Expect(actual.Annotations).To(HaveKeyWithValue("keep", "yes"))
			Expect(actual.Spec).To(Equal(existing.Spec))
		} else {
			Expect(actual.Spec).To(Equal(mcfgv1.MachineOSConfigSpec{}))
		}
	}, Entry("create", false, false), Entry("patch", true, false), Entry("unchanged", true, true))

	DescribeTable("propagates MachineOSConfig errors", func(stage string) {
		irm := newIRM(nil, nil, nil)
		failure := errors.New("MachineOSConfig failed")
		if stage == "get" {
			clnt.EXPECT().Get(ctx, client.ObjectKey{Name: mcfg.InitramfsMachineOSConfigName}, gomock.AssignableToTypeOf(&mcfgv1.MachineOSConfig{})).Return(failure)
		} else {
			if stage == "patch" {
				expectGetObject(clnt, ctx, &mcfgv1.MachineOSConfig{ObjectMeta: metav1.ObjectMeta{Name: mcfg.InitramfsMachineOSConfigName}})
			} else {
				expectGetNotFound(clnt, ctx, mcfg.InitramfsMachineOSConfigName, &mcfgv1.MachineOSConfig{})
			}
			mockMCFG.EXPECT().UpdateInitramfsMachineOSConfig(gomock.Any(), irm).DoAndReturn(
				func(mosc *mcfgv1.MachineOSConfig, _ *kmmv1beta1.InitramfsModule) error {
					if stage == "populate" {
						return failure
					}
					mosc.Annotations = map[string]string{"updated": "yes"}
					return nil
				},
			)
			if stage == "create" {
				clnt.EXPECT().Create(ctx, gomock.AssignableToTypeOf(&mcfgv1.MachineOSConfig{})).Return(failure)
			} else if stage == "patch" {
				clnt.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&mcfgv1.MachineOSConfig{}), gomock.Any()).Return(failure)
			}
		}

		Expect(h.handleMachineOSConfig(ctx, irm)).To(MatchError(failure))
	}, Entry("get", "get"), Entry("populate", "populate"), Entry("create", "create"), Entry("patch", "patch"))

})

var _ = Describe("InitramfsModuleReconciler resource integration", func() {
	var (
		ctx      context.Context
		ctrl     *gomock.Controller
		clnt     *kmmclient.MockClient
		mockMIC  *mic.MockMIC
		mockNode *node.MockNode
		r        *initramfsModuleReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		clnt = kmmclient.NewMockClient(ctrl)
		mockMIC = mic.NewMockMIC(ctrl)
		mockNode = node.NewMockNode(ctrl)
		r = NewInitramfsModuleReconciler(clnt, mockMIC, mockNode, mcfg.NewMCFG(""), nil)
	})

	DescribeTable("creates or patches KMM resources only when images are ready", func(ready, exists bool) {
		selector := map[string]string{"kmm-test": "initramfs"}
		irm := newIRM(selector, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		var pool *mcfgv1.MachineConfigPool
		var mosc *mcfgv1.MachineOSConfig
		calls := []*gomock.Call{
			mockNode.EXPECT().GetAllNodesBySelector(ctx, selector).Return(nodes, nil),
			mockMIC.EXPECT().CreateOrPatch(
				ctx, "irm-"+irmName, irmNS,
				[]kmmv1beta1.ModuleImageSpec{imageSpec("6.1.0", nil, nil, irm)},
				irm.Spec.ImageRepoSecret, v1.PullPolicy(""), true, nil, nil, irm,
			).Return(nil),
			expectGetMIC(clnt, ctx, existingInitramfsMIC()),
			mockMIC.EXPECT().DoAllImagesExist(gomock.Any()).Return(ready),
		}
		poolBefore := &mcfgv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{
			Name: mcfg.InitramfsPoolName, UID: "pool-uid", Annotations: map[string]string{"keep": "yes"},
		}}
		moscBefore := &mcfgv1.MachineOSConfig{ObjectMeta: metav1.ObjectMeta{
			Name: mcfg.InitramfsMachineOSConfigName, UID: "mosc-uid", Annotations: map[string]string{"keep": "yes"},
		}, Spec: mcfgv1.MachineOSConfigSpec{RenderedImagePushSpec: "registry.example.com/os:existing"}}
		if ready {
			if exists {
				calls = append(calls,
					expectGetObject(clnt, ctx, poolBefore),
					expectPatch(clnt, ctx, func(obj client.Object) { pool = obj.(*mcfgv1.MachineConfigPool).DeepCopy() }),
					expectGetObject(clnt, ctx, moscBefore),
					expectPatch(clnt, ctx, func(obj client.Object) { mosc = obj.(*mcfgv1.MachineOSConfig).DeepCopy() }),
				)
			} else {
				calls = append(calls,
					expectGetNotFound(clnt, ctx, mcfg.InitramfsPoolName, &mcfgv1.MachineConfigPool{}),
					expectCreate(clnt, ctx, func(obj client.Object) { pool = obj.(*mcfgv1.MachineConfigPool).DeepCopy() }),
					expectGetNotFound(clnt, ctx, mcfg.InitramfsMachineOSConfigName, &mcfgv1.MachineOSConfig{}),
					expectCreate(clnt, ctx, func(obj client.Object) { mosc = obj.(*mcfgv1.MachineOSConfig).DeepCopy() }),
				)
			}
		}
		for i := 1; i < len(calls); i++ {
			calls[i].After(calls[i-1])
		}

		res, err := r.Reconcile(ctx, irm)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(reconcile.Result{}))
		if ready {
			Expect(pool).NotTo(BeNil())
			Expect(mosc).NotTo(BeNil())
			Expect(pool.Name).To(Equal(mcfg.InitramfsPoolName))
			Expect(pool.Spec.NodeSelector.MatchLabels).To(Equal(selector))
			Expect(pool.OwnerReferences).To(BeEmpty())
			Expect(pool.Annotations).To(HaveKeyWithValue(mcfg.InitramfsModuleNamespaceAnnotation, irm.Namespace))
			Expect(pool.Annotations).To(HaveKeyWithValue(mcfg.InitramfsModuleNameAnnotation, irm.Name))
			Expect(mosc.Name).To(Equal(mcfg.InitramfsMachineOSConfigName))
			Expect(mosc.OwnerReferences).To(BeEmpty())
			Expect(mosc.Annotations).To(HaveKeyWithValue(mcfg.InitramfsModuleNamespaceAnnotation, irm.Namespace))
			Expect(mosc.Annotations).To(HaveKeyWithValue(mcfg.InitramfsModuleNameAnnotation, irm.Name))
			if exists {
				Expect(pool.UID).To(Equal(poolBefore.UID))
				Expect(pool.Annotations).To(HaveKeyWithValue("keep", "yes"))
				Expect(mosc.UID).To(Equal(moscBefore.UID))
				Expect(mosc.Annotations).To(HaveKeyWithValue("keep", "yes"))
				Expect(mosc.Spec).To(Equal(moscBefore.Spec))
			} else {
				Expect(mosc.Spec).To(Equal(mcfgv1.MachineOSConfigSpec{}))
			}
		} else {
			Expect(pool).To(BeNil())
			Expect(mosc).To(BeNil())
		}
	}, Entry("image pending", false, false), Entry("image ready with new resources", true, false), Entry("image ready with existing unstamped resources", true, true))

})

func existingInitramfsMIC() *kmmv1beta1.ModuleImagesConfig {
	return &kmmv1beta1.ModuleImagesConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "irm-" + irmName, Namespace: irmNS},
		Status: kmmv1beta1.ModuleImagesConfigStatus{
			ImagesStates: []kmmv1beta1.ModuleImageState{{
				Image:  imageRepo + "-6.1.0",
				Status: kmmv1beta1.ImageExists,
			}},
		},
	}
}

func notFound() error {
	return apierrors.NewNotFound(schema.GroupResource{}, "missing")
}

func expectGetNotFound(clnt *kmmclient.MockClient, ctx context.Context, name string, obj client.Object) *gomock.Call {
	return clnt.EXPECT().Get(ctx, client.ObjectKey{Name: name}, gomock.AssignableToTypeOf(obj)).Return(notFound())
}

func expectGetMIC(clnt *kmmclient.MockClient, ctx context.Context, src *kmmv1beta1.ModuleImagesConfig) *gomock.Call {
	return clnt.EXPECT().Get(ctx, client.ObjectKey{Name: src.GetName(), Namespace: src.GetNamespace()}, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			src.DeepCopyInto(obj.(*kmmv1beta1.ModuleImagesConfig))
			return nil
		},
	)
}

func expectGetObject(clnt *kmmclient.MockClient, ctx context.Context, src client.Object) *gomock.Call {
	return clnt.EXPECT().Get(ctx, client.ObjectKey{Name: src.GetName()}, gomock.AssignableToTypeOf(src)).DoAndReturn(
		func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			switch s := src.(type) {
			case *mcfgv1.MachineConfigPool:
				s.DeepCopyInto(obj.(*mcfgv1.MachineConfigPool))
			case *mcfgv1.MachineOSConfig:
				s.DeepCopyInto(obj.(*mcfgv1.MachineOSConfig))
			}
			return nil
		},
	)
}

func expectCreate(clnt *kmmclient.MockClient, ctx context.Context, capture func(client.Object)) *gomock.Call {
	return clnt.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
			capture(obj)
			return nil
		},
	)
}

func expectPatch(clnt *kmmclient.MockClient, ctx context.Context, capture func(client.Object)) *gomock.Call {
	return clnt.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
			capture(obj)
			return nil
		},
	)
}
