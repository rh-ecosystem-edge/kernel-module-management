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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	It("returns the unstamped-object error and does not list nodes", func() {
		irm := newIRM(nil, nil, nil)
		mockHelper.EXPECT().checkUnstampedPoolAndMachineConfig(ctx, irm).Return(errors.New("MachineConfigPool kmm-initramfs already exists and is not stamped for this CR"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not stamped"))
	})

	It("returns the list error and does not build images", func() {
		irm := newIRM(nil, nil, nil)
		mockHelper.EXPECT().checkUnstampedPoolAndMachineConfig(ctx, irm).Return(nil)
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nil, errors.New("list failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("list failed"))
	})

	It("calls the parse job, then the pool, then the MachineConfig", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		parsed := initramfsParseParams{
			Kernels: map[string]mcfg.KernelInspectLists{
				"6.1.0": {InTreeModules: []string{"crc32"}},
			},
		}
		gomock.InOrder(
			mockHelper.EXPECT().checkUnstampedPoolAndMachineConfig(ctx, irm).Return(nil),
			mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil),
			mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil),
			mockHelper.EXPECT().handleParseJob(ctx, irm).Return(parsed, nil),
			mockHelper.EXPECT().handleMachineConfigPool(ctx, irm).Return(nil),
			mockHelper.EXPECT().handleMachineConfig(ctx, irm, parsed).Return(nil),
		)

		res, err := r.Reconcile(ctx, irm)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(reconcile.Result{}))
	})

	It("returns the handleMIC error", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		mockHelper.EXPECT().checkUnstampedPoolAndMachineConfig(ctx, irm).Return(nil)
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil)
		mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(errors.New("apply failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("apply failed"))
	})

	It("returns the parse job error and does not create the pool", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		gomock.InOrder(
			mockHelper.EXPECT().checkUnstampedPoolAndMachineConfig(ctx, irm).Return(nil),
			mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil),
			mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil),
			mockHelper.EXPECT().handleParseJob(ctx, irm).Return(initramfsParseParams{}, errors.New("parse failed")),
		)

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("parse failed"))
	})

	It("returns the MachineConfigPool error", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		parsed := initramfsParseParams{}
		mockHelper.EXPECT().checkUnstampedPoolAndMachineConfig(ctx, irm).Return(nil)
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil)
		mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil)
		mockHelper.EXPECT().handleParseJob(ctx, irm).Return(parsed, nil)
		mockHelper.EXPECT().handleMachineConfigPool(ctx, irm).Return(errors.New("mcp failed"))

		_, err := r.Reconcile(ctx, irm)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("mcp failed"))
	})

	It("returns the MachineConfig error", func() {
		irm := newIRM(nil, nil, nil)
		nodes := []v1.Node{nodeWithKernel("n1", "6.1.0")}
		parsed := initramfsParseParams{}
		mockHelper.EXPECT().checkUnstampedPoolAndMachineConfig(ctx, irm).Return(nil)
		mockHelper.EXPECT().listSelectedNodes(ctx, irm).Return(nodes, nil)
		mockHelper.EXPECT().handleMIC(ctx, irm, nodes).Return(nil)
		mockHelper.EXPECT().handleParseJob(ctx, irm).Return(parsed, nil)
		mockHelper.EXPECT().handleMachineConfigPool(ctx, irm).Return(nil)
		mockHelper.EXPECT().handleMachineConfig(ctx, irm, parsed).Return(errors.New("mc failed"))

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

	It("returns no parse parameters and does not use the client", func() {
		parsed, err := h.handleParseJob(ctx, newIRM(nil, nil, nil))

		Expect(err).NotTo(HaveOccurred())
		Expect(parsed).To(Equal(initramfsParseParams{}))
	})
})

var _ = Describe("initramfs MachineConfigPool and MachineConfig", func() {
	var (
		ctx  context.Context
		ctrl *gomock.Controller
		clnt *kmmclient.MockClient
		h    initramfsModuleReconcilerHelper
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		clnt = kmmclient.NewMockClient(ctrl)
		h = newInitramfsModuleReconcilerHelper(clnt, nil, nil, mcfg.NewMCFG(""))
	})

	It("accepts a cluster with neither object", func() {
		expectGetNotFound(clnt, ctx, mcfg.InitramfsPoolName)
		expectGetNotFound(clnt, ctx, mcfg.InitramfsMachineConfigName)

		Expect(h.checkUnstampedPoolAndMachineConfig(ctx, stampedIRM())).To(Succeed())
	})

	It("creates the pool and MachineConfig with stamps and no ownerReference", func() {
		irm := stampedIRM()
		var pool *mcfgv1.MachineConfigPool
		var mc *mcfgv1.MachineConfig
		gomock.InOrder(
			expectGetNotFound(clnt, ctx, mcfg.InitramfsPoolName),
			expectCreate(clnt, ctx, func(obj client.Object) { pool = obj.(*mcfgv1.MachineConfigPool).DeepCopy() }),
			expectGetNotFound(clnt, ctx, mcfg.InitramfsMachineConfigName),
			expectCreate(clnt, ctx, func(obj client.Object) { mc = obj.(*mcfgv1.MachineConfig).DeepCopy() }),
		)

		Expect(h.handleMachineConfigPool(ctx, irm)).To(Succeed())
		Expect(h.handleMachineConfig(ctx, irm, initramfsParseParams{})).To(Succeed())

		expectStamps(pool, irm)
		expectStamps(mc, irm)
		Expect(pool.OwnerReferences).To(BeEmpty())
		Expect(mc.OwnerReferences).To(BeEmpty())
		Expect(pool.Spec.NodeSelector).To(Equal(&metav1.LabelSelector{
			MatchLabels: map[string]string{"node-role.kubernetes.io/initramfs": ""},
		}))
		Expect(pool.Spec.MachineConfigSelector).To(Equal(&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "machineconfiguration.openshift.io/role",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"worker", mcfg.InitramfsPoolName},
			}},
		}))
		Expect(pool.Spec.MaxUnavailable).To(Equal(ptrTo(intstr.FromInt(1))))
		Expect(mc.Labels).To(HaveKeyWithValue("machineconfiguration.openshift.io/role", mcfg.InitramfsPoolName))
	})

	It("patches a stamped pool instead of creating it again", func() {
		irm := stampedIRM()
		existing := &mcfgv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{
			Name:        mcfg.InitramfsPoolName,
			UID:         "pool-uid",
			Annotations: stampAnnotations(irm),
		}}
		var patched *mcfgv1.MachineConfigPool
		gomock.InOrder(
			expectGetNotFound(clnt, ctx, mcfg.InitramfsPoolName),
			expectCreate(clnt, ctx, func(client.Object) {}),
			expectGetObject(clnt, ctx, existing),
			expectPatch(clnt, ctx, func(obj client.Object) { patched = obj.(*mcfgv1.MachineConfigPool).DeepCopy() }),
		)

		Expect(h.handleMachineConfigPool(ctx, irm)).To(Succeed())
		Expect(h.handleMachineConfigPool(ctx, irm)).To(Succeed())

		Expect(patched.UID).To(Equal(existing.UID))
		Expect(patched.Spec.MaxUnavailable).To(Equal(ptrTo(intstr.FromInt(1))))
	})

	It("refuses a pool that has no stamp and does not create the MachineConfig", func() {
		irm := stampedIRM()
		existing := &mcfgv1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: mcfg.InitramfsPoolName},
			Spec:       mcfgv1.MachineConfigPoolSpec{Paused: true},
		}
		expectGetObject(clnt, ctx, existing)
		expectGetNotFound(clnt, ctx, mcfg.InitramfsMachineConfigName)

		err := h.checkUnstampedPoolAndMachineConfig(ctx, irm)

		Expect(err).To(MatchError(ContainSubstring("MachineConfigPool kmm-initramfs already exists and is not stamped for this CR")))
		Expect(existing.Spec.Paused).To(BeTrue())
		Expect(existing.Annotations).To(BeEmpty())
		Expect(existing.Spec.NodeSelector).To(BeNil())
		Expect(existing.Spec.MachineConfigSelector).To(BeNil())
	})

	It("refuses a pool whose UID stamp does not match", func() {
		irm := stampedIRM()
		existing := &mcfgv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{
			Name: mcfg.InitramfsPoolName,
			Annotations: map[string]string{
				mcfg.InitramfsModuleNamespaceAnnotation: irm.Namespace,
				mcfg.InitramfsModuleNameAnnotation:      irm.Name,
				mcfg.InitramfsModuleUIDAnnotation:       "other-uid",
			},
		}}
		expectGetObject(clnt, ctx, existing)
		expectGetNotFound(clnt, ctx, mcfg.InitramfsMachineConfigName)

		err := h.checkUnstampedPoolAndMachineConfig(ctx, irm)

		Expect(err).To(MatchError(ContainSubstring("MachineConfigPool kmm-initramfs already exists and is not stamped for this CR")))
		Expect(existing.Annotations[mcfg.InitramfsModuleUIDAnnotation]).To(Equal("other-uid"))
	})

	It("refuses a MachineConfig that has no stamp and does not create the pool", func() {
		irm := stampedIRM()
		existing := &mcfgv1.MachineConfig{
			ObjectMeta: metav1.ObjectMeta{Name: mcfg.InitramfsMachineConfigName},
			Spec:       mcfgv1.MachineConfigSpec{OSImageURL: "keep"},
		}
		expectGetNotFound(clnt, ctx, mcfg.InitramfsPoolName)
		expectGetObject(clnt, ctx, existing)

		err := h.checkUnstampedPoolAndMachineConfig(ctx, irm)

		Expect(err).To(MatchError(ContainSubstring("MachineConfig 99-kmm-initramfs already exists and is not stamped for this CR")))
		Expect(existing.Spec.OSImageURL).To(Equal("keep"))
		Expect(existing.Annotations).To(BeEmpty())
		Expect(existing.Labels).To(BeEmpty())
	})

	It("refuses a MachineConfig whose UID stamp does not match and does not patch a stamped pool", func() {
		irm := stampedIRM()
		pool := &mcfgv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{
			Name:        mcfg.InitramfsPoolName,
			Annotations: stampAnnotations(irm),
		}}
		pool.Spec.NodeSelector = &metav1.LabelSelector{MatchLabels: map[string]string{"touched": "yes"}}
		foreign := &mcfgv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{
			Name: mcfg.InitramfsMachineConfigName,
			Annotations: map[string]string{
				mcfg.InitramfsModuleNamespaceAnnotation: irm.Namespace,
				mcfg.InitramfsModuleNameAnnotation:      irm.Name,
				mcfg.InitramfsModuleUIDAnnotation:       "other-uid",
			},
		}}
		expectGetObject(clnt, ctx, pool)
		expectGetObject(clnt, ctx, foreign)

		err := h.checkUnstampedPoolAndMachineConfig(ctx, irm)

		Expect(err).To(MatchError(ContainSubstring("MachineConfig 99-kmm-initramfs already exists and is not stamped for this CR")))
		Expect(pool.Spec.NodeSelector.MatchLabels).To(HaveKeyWithValue("touched", "yes"))
		Expect(foreign.Annotations[mcfg.InitramfsModuleUIDAnnotation]).To(Equal("other-uid"))
	})

	It("creates both objects from Reconcile when the parse job returns no lists", func() {
		mockNode := node.NewMockNode(ctrl)
		irm := stampedIRM()
		r := NewInitramfsModuleReconciler(clnt, nil, mockNode, mcfg.NewMCFG(""))
		var pool *mcfgv1.MachineConfigPool
		var mc *mcfgv1.MachineConfig
		gomock.InOrder(
			expectGetNotFound(clnt, ctx, mcfg.InitramfsPoolName),
			expectGetNotFound(clnt, ctx, mcfg.InitramfsMachineConfigName),
			mockNode.EXPECT().GetAllNodesBySelector(ctx, workerSelector).Return([]v1.Node{}, nil),
			expectGetNotFound(clnt, ctx, mcfg.InitramfsPoolName),
			expectCreate(clnt, ctx, func(obj client.Object) { pool = obj.(*mcfgv1.MachineConfigPool).DeepCopy() }),
			expectGetNotFound(clnt, ctx, mcfg.InitramfsMachineConfigName),
			expectCreate(clnt, ctx, func(obj client.Object) { mc = obj.(*mcfgv1.MachineConfig).DeepCopy() }),
		)

		res, err := r.Reconcile(ctx, irm)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(reconcile.Result{}))
		expectStamps(pool, irm)
		expectStamps(mc, irm)
		Expect(pool.OwnerReferences).To(BeEmpty())
		Expect(mc.OwnerReferences).To(BeEmpty())
	})
})

func stampedIRM() *kmmv1beta1.InitramfsModule {
	irm := newIRM(nil, nil, nil)
	irm.UID = types.UID("cr-uid")
	return irm
}

func stampAnnotations(irm *kmmv1beta1.InitramfsModule) map[string]string {
	return map[string]string{
		mcfg.InitramfsModuleNamespaceAnnotation: irm.Namespace,
		mcfg.InitramfsModuleNameAnnotation:      irm.Name,
		mcfg.InitramfsModuleUIDAnnotation:       string(irm.UID),
	}
}

func notFound() error {
	return apierrors.NewNotFound(schema.GroupResource{}, "missing")
}

func expectGetNotFound(clnt *kmmclient.MockClient, ctx context.Context, name string) *gomock.Call {
	return clnt.EXPECT().Get(ctx, client.ObjectKey{Name: name}, gomock.Any()).Return(notFound())
}

func expectGetObject(clnt *kmmclient.MockClient, ctx context.Context, src client.Object) *gomock.Call {
	return clnt.EXPECT().Get(ctx, client.ObjectKey{Name: src.GetName()}, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			switch s := src.(type) {
			case *mcfgv1.MachineConfigPool:
				s.DeepCopyInto(obj.(*mcfgv1.MachineConfigPool))
			case *mcfgv1.MachineConfig:
				s.DeepCopyInto(obj.(*mcfgv1.MachineConfig))
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

func expectStamps(obj metav1.Object, irm *kmmv1beta1.InitramfsModule) {
	Expect(obj.GetAnnotations()).To(HaveKeyWithValue(mcfg.InitramfsModuleNamespaceAnnotation, irm.Namespace))
	Expect(obj.GetAnnotations()).To(HaveKeyWithValue(mcfg.InitramfsModuleNameAnnotation, irm.Name))
	Expect(obj.GetAnnotations()).To(HaveKeyWithValue(mcfg.InitramfsModuleUIDAnnotation, string(irm.UID)))
}

func ptrTo[T any](v T) *T {
	return &v
}
