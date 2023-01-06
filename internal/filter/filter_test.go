package filter

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	imagev1 "github.com/openshift/api/image/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hubv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api-hub/v1beta1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	mockClient "github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
)

var (
	ctrl *gomock.Controller
	clnt *mockClient.MockClient
)

var _ = Describe("HasLabel", func() {
	const label = "test-label"

	dsWithEmptyLabel := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{label: ""},
		},
	}

	dsWithLabel := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{label: "some-module"},
		},
	}

	DescribeTable("Should return the expected value",
		func(obj client.Object, expected bool) {
			Expect(
				HasLabel(label).Delete(event.DeleteEvent{Object: obj}),
			).To(
				Equal(expected),
			)
		},
		Entry("label not set", &appsv1.DaemonSet{}, false),
		Entry("label set to empty value", dsWithEmptyLabel, false),
		Entry("label set to a concrete value", dsWithLabel, true),
	)
})

var _ = Describe("skipDeletions", func() {
	It("should return false for delete events", func() {
		Expect(
			skipDeletions.Delete(event.DeleteEvent{}),
		).To(
			BeFalse(),
		)
	})
})

var _ = Describe("ModuleReconcilerNodePredicate", func() {
	const kernelLabel = "kernel-label"
	var p predicate.Predicate

	BeforeEach(func() {
		p = New(nil, logr.Discard()).ModuleReconcilerNodePredicate(kernelLabel)
	})

	It("should return true for creations", func() {
		ev := event.CreateEvent{
			Object: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{kernelLabel: "1.2.3"},
				},
			},
		}

		Expect(
			p.Create(ev),
		).To(
			BeTrue(),
		)
	})

	It("should return true for label updates", func() {
		ev := event.UpdateEvent{
			ObjectOld: &v1.Node{},
			ObjectNew: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{kernelLabel: "1.2.3"},
				},
			},
		}

		Expect(
			p.Update(ev),
		).To(
			BeTrue(),
		)
	})
	It("should return false for label updates without the expected label", func() {
		ev := event.UpdateEvent{
			ObjectOld: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"a": "b"},
				},
			},
			ObjectNew: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"c": "d"},
				},
			},
		}

		Expect(
			p.Update(ev),
		).To(
			BeFalse(),
		)
	})

	It("should return false for deletions", func() {
		ev := event.DeleteEvent{
			Object: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{kernelLabel: "1.2.3"},
				},
			},
		}

		Expect(
			p.Delete(ev),
		).To(
			BeFalse(),
		)
	})
})

var _ = Describe("NodeKernelReconcilerPredicate", func() {
	const (
		kernelVersion = "1.2.3"
		labelName     = "test-label"
	)

	var p predicate.Predicate

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		p = New(nil, logr.Discard()).NodeKernelReconcilerPredicate(labelName)
	})

	It("should return true on CREATE events", func() {
		ev := event.CreateEvent{
			Object: &v1.Node{},
		}

		Expect(
			p.Create(ev),
		).To(
			BeTrue(),
		)
	})

	It("should return false on UPDATE events if the data hasn't changed", func() {

		By("kernel version in nodeInfo is the same as the label")

		ev := event.UpdateEvent{
			ObjectNew: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{labelName: kernelVersion},
				},
				Status: v1.NodeStatus{
					NodeInfo: v1.NodeSystemInfo{KernelVersion: kernelVersion},
				},
			},
			ObjectOld: &v1.Node{},
		}

		Expect(
			p.Update(ev),
		).To(
			BeFalse(),
		)

		By("os image version in nodeInfo hasn't changed")

		const osImageVersion = "411.86"

		ev = event.UpdateEvent{
			ObjectNew: &v1.Node{
				Status: v1.NodeStatus{
					NodeInfo: v1.NodeSystemInfo{OSImage: osImageVersion},
				},
			},
			ObjectOld: &v1.Node{
				Status: v1.NodeStatus{
					NodeInfo: v1.NodeSystemInfo{OSImage: osImageVersion},
				},
			},
		}

		Expect(
			p.Update(ev),
		).To(
			BeFalse(),
		)
	})

	It("should return true on UPDATE events if the data has changed", func() {

		By("kernel version in nodeInfo is different than the label")

		ev := event.UpdateEvent{
			ObjectNew: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{labelName: labelName},
				},
				Status: v1.NodeStatus{
					NodeInfo: v1.NodeSystemInfo{KernelVersion: kernelVersion},
				},
			},
			ObjectOld: &v1.Node{},
		}

		Expect(
			p.Update(ev),
		).To(
			BeTrue(),
		)

		By("os image version in nodeInfo has changed")

		ev = event.UpdateEvent{
			ObjectNew: &v1.Node{
				Status: v1.NodeStatus{
					NodeInfo: v1.NodeSystemInfo{OSImage: "412.86"},
				},
			},
			ObjectOld: &v1.Node{
				Status: v1.NodeStatus{
					NodeInfo: v1.NodeSystemInfo{OSImage: "411.86"},
				},
			},
		}

		Expect(
			p.Update(ev),
		).To(
			BeTrue(),
		)
	})

	It("should return false on DELETE events", func() {
		ev := event.DeleteEvent{
			Object: &v1.Node{},
		}

		Expect(
			p.Delete(ev),
		).To(
			BeFalse(),
		)
	})
})

var _ = Describe("NodeUpdateKernelChangedPredicate", func() {
	updateFunc := NodeUpdateKernelChangedPredicate().Update

	node1 := v1.Node{
		Status: v1.NodeStatus{
			NodeInfo: v1.NodeSystemInfo{KernelVersion: "v1"},
		},
	}

	node2 := v1.Node{
		Status: v1.NodeStatus{
			NodeInfo: v1.NodeSystemInfo{KernelVersion: "v2"},
		},
	}

	DescribeTable(
		"should work as expected",
		func(updateEvent event.UpdateEvent, expectedResult bool) {
			Expect(
				updateFunc(updateEvent),
			).To(
				Equal(expectedResult),
			)
		},
		Entry(nil, event.UpdateEvent{ObjectOld: &v1.Pod{}, ObjectNew: &v1.Node{}}, false),
		Entry(nil, event.UpdateEvent{ObjectOld: &v1.Node{}, ObjectNew: &v1.Pod{}}, false),
		Entry(nil, event.UpdateEvent{ObjectOld: &v1.Node{}, ObjectNew: &v1.Pod{}}, false),
		Entry(nil, event.UpdateEvent{ObjectOld: &node1, ObjectNew: &node1}, false),
		Entry(nil, event.UpdateEvent{ObjectOld: &node1, ObjectNew: &node2}, true),
	)
})

var _ = Describe("FindModulesForNode", func() {
	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = mockClient.NewMockClient(ctrl)
	})

	It("should return nothing if there are no modules", func() {
		clnt.EXPECT().List(context.Background(), gomock.Any(), gomock.Any())

		p := New(clnt, logr.Discard())
		Expect(
			p.FindModulesForNode(&v1.Node{}),
		).To(
			BeEmpty(),
		)
	})

	It("should return nothing if the node labels match no module", func() {
		mod := kmmv1beta1.Module{
			Spec: kmmv1beta1.ModuleSpec{
				Selector: map[string]string{"key": "value"},
			},
		}

		clnt.EXPECT().List(context.Background(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ interface{}, list *kmmv1beta1.ModuleList, _ ...interface{}) error {
				list.Items = []kmmv1beta1.Module{mod}
				return nil
			},
		)

		p := New(clnt, logr.Discard())

		Expect(
			p.FindModulesForNode(&v1.Node{}),
		).To(
			BeEmpty(),
		)
	})

	It("should return only modules matching the node", func() {
		nodeLabels := map[string]string{"key": "value"}

		node := v1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: nodeLabels},
		}

		const mod1Name = "mod1"

		mod1 := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{Name: mod1Name},
			Spec:       kmmv1beta1.ModuleSpec{Selector: nodeLabels},
		}

		mod2 := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{Name: "mod2"},
			Spec: kmmv1beta1.ModuleSpec{
				Selector: map[string]string{"other-key": "other-value"},
			},
		}
		clnt.EXPECT().List(context.Background(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ interface{}, list *kmmv1beta1.ModuleList, _ ...interface{}) error {
				list.Items = []kmmv1beta1.Module{mod1, mod2}
				return nil
			},
		)

		p := New(clnt, logr.Discard())

		expectedReq := reconcile.Request{
			NamespacedName: types.NamespacedName{Name: mod1Name},
		}

		reqs := p.FindModulesForNode(&node)
		Expect(reqs).To(Equal([]reconcile.Request{expectedReq}))
	})
})

var _ = Describe("FindManagedClusterModulesForCluster", func() {
	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = mockClient.NewMockClient(ctrl)
	})

	It("should return nothing if there are no ManagedClusterModules", func() {
		clnt.EXPECT().List(context.Background(), gomock.Any(), gomock.Any())

		p := New(clnt, logr.Discard())
		Expect(
			p.FindManagedClusterModulesForCluster(&clusterv1.ManagedCluster{}),
		).To(
			BeEmpty(),
		)
	})

	It("should return nothing if the cluster labels match no ManagedClusterModule", func() {
		mod := hubv1beta1.ManagedClusterModule{
			Spec: hubv1beta1.ManagedClusterModuleSpec{
				Selector: map[string]string{"key": "value"},
			},
		}

		clnt.EXPECT().List(context.Background(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ interface{}, list *hubv1beta1.ManagedClusterModuleList, _ ...interface{}) error {
				list.Items = []hubv1beta1.ManagedClusterModule{mod}
				return nil
			},
		)

		p := New(clnt, logr.Discard())

		Expect(
			p.FindManagedClusterModulesForCluster(&clusterv1.ManagedCluster{}),
		).To(
			BeEmpty(),
		)
	})

	It("should return only ManagedClusterModules matching the cluster", func() {
		clusterLabels := map[string]string{"key": "value"}

		cluster := clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Labels: clusterLabels,
			},
		}

		matchingMod := hubv1beta1.ManagedClusterModule{
			ObjectMeta: metav1.ObjectMeta{Name: "matching-mod"},
			Spec: hubv1beta1.ManagedClusterModuleSpec{
				Selector: clusterLabels,
			},
		}

		mod := hubv1beta1.ManagedClusterModule{
			ObjectMeta: metav1.ObjectMeta{Name: "mod"},
			Spec: hubv1beta1.ManagedClusterModuleSpec{
				Selector: map[string]string{"other-key": "other-value"},
			},
		}

		clnt.EXPECT().List(context.Background(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ interface{}, list *hubv1beta1.ManagedClusterModuleList, _ ...interface{}) error {
				list.Items = []hubv1beta1.ManagedClusterModule{matchingMod, mod}
				return nil
			},
		)

		p := New(clnt, logr.Discard())

		expectedReq := reconcile.Request{
			NamespacedName: types.NamespacedName{Name: matchingMod.Name},
		}

		reqs := p.FindManagedClusterModulesForCluster(&cluster)
		Expect(reqs).To(Equal([]reconcile.Request{expectedReq}))
	})
})

var _ = Describe("DeletingPredicate", func() {
	now := metav1.Now()

	DescribeTable("should return the expected value",
		func(o client.Object, expected bool) {
			Expect(
				DeletingPredicate().Generic(event.GenericEvent{Object: o}),
			).To(
				Equal(expected),
			)
		},
		Entry(nil, &v1.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}}, true),
		Entry(nil, &v1.Pod{}, false),
	)
})

var _ = Describe("MatchesNamespacedNamePredicate", func() {
	const (
		name      = "name"
		namespace = "namespace"
	)

	p := MatchesNamespacedNamePredicate(types.NamespacedName{Name: name, Namespace: namespace})

	DescribeTable("should return the expected value",
		func(nsn types.NamespacedName, expected bool) {
			cm := v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: nsn.Name, Namespace: nsn.Namespace},
			}

			Expect(
				p.Create(event.CreateEvent{Object: &cm}),
			).To(
				Equal(expected),
			)
		},
		Entry("bad name", types.NamespacedName{Name: "other-name", Namespace: namespace}, false),
		Entry("bad namespace", types.NamespacedName{Name: name, Namespace: "other-namespace"}, false),
		Entry("both bad", types.NamespacedName{Name: "other-name", Namespace: "other-namespace"}, false),
		Entry("both good", types.NamespacedName{Name: name, Namespace: namespace}, true),
	)
})

var _ = Describe("PodHasSpecNodeName", func() {
	p := PodHasSpecNodeName()

	DescribeTable(
		"should return the expected value",
		func(o client.Object, expected bool) {
			Expect(
				p.Create(event.CreateEvent{Object: o}),
			).To(
				Equal(expected),
			)
		},
		Entry("ConfigMap: false", &v1.ConfigMap{}, false),
		Entry("Pod with no nodeName: false", &v1.Pod{}, false),
		Entry("Pod with a nodeName: true", &v1.Pod{Spec: v1.PodSpec{NodeName: "test"}}, true),
	)
})

var _ = Describe("PodReadinessChangedPredicate", func() {
	p := PodReadinessChangedPredicate(logr.Discard())

	DescribeTable(
		"should return the expected value",
		func(e event.UpdateEvent, expected bool) {
			Expect(p.Update(e)).To(Equal(expected))
		},
		Entry("objects are nil", event.UpdateEvent{}, true),
		Entry("old object is not a Pod", event.UpdateEvent{ObjectOld: &v1.Node{}}, true),
		Entry(
			"new object is not a Pod",
			event.UpdateEvent{
				ObjectOld: &v1.Pod{},
				ObjectNew: &v1.Node{},
			},
			true,
		),
		Entry(
			"both objects are pods with the same conditions",
			event.UpdateEvent{
				ObjectOld: &v1.Pod{},
				ObjectNew: &v1.Pod{},
			},
			false,
		),
		Entry(
			"both objects are pods with different conditions",
			event.UpdateEvent{
				ObjectOld: &v1.Pod{},
				ObjectNew: &v1.Pod{
					Status: v1.PodStatus{
						Conditions: []v1.PodCondition{
							{
								Type:   v1.PodReady,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			true,
		),
	)
})

var _ = Describe("FindPreflightsForModule", func() {

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = mockClient.NewMockClient(ctrl)
	})

	It("no preflight exists", func() {
		clnt.EXPECT().List(context.Background(), gomock.Any(), gomock.Any())

		p := New(clnt, logr.Discard())

		res := p.EnqueueAllPreflightValidations(&kmmv1beta1.Module{})
		Expect(res).To(BeEmpty())
	})

	It("preflight exists", func() {
		preflight := kmmv1beta1.PreflightValidation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "preflight",
				Namespace: "preflightNamespace",
			},
		}

		clnt.EXPECT().List(context.Background(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ interface{}, list *kmmv1beta1.PreflightValidationList, _ ...interface{}) error {
				list.Items = []kmmv1beta1.PreflightValidation{preflight}
				return nil
			},
		)

		expectedRes := []reconcile.Request{
			reconcile.Request{
				NamespacedName: types.NamespacedName{Name: preflight.Name, Namespace: preflight.Namespace},
			},
		}

		p := New(clnt, logr.Discard())
		res := p.EnqueueAllPreflightValidations(&kmmv1beta1.Module{})
		Expect(res).To(Equal(expectedRes))
	})

})

var _ = Describe("ImageStreamReconcilerPredicate", func() {

	var p predicate.Predicate = New(nil, logr.Discard()).ImageStreamReconcilerPredicate()

	It("should return true on CREATE events", func() {
		ev := event.CreateEvent{
			Object: &imagev1.ImageStream{},
		}

		Expect(
			p.Create(ev),
		).To(
			BeTrue(),
		)
	})

	It("should return true on UPDATE events if any of the tags has changed", func() {

		ev := event.UpdateEvent{
			ObjectNew: &imagev1.ImageStream{
				Spec: imagev1.ImageStreamSpec{
					Tags: []imagev1.TagReference{
						{
							Name: "411.86.202210072320-0",
							From: &v1.ObjectReference{
								Name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:111",
							},
						},
					},
				},
			},
			ObjectOld: &imagev1.ImageStream{
				Spec: imagev1.ImageStreamSpec{
					Tags: []imagev1.TagReference{
						{
							Name: "412.86.202210072320-0",
							From: &v1.ObjectReference{
								Name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:222",
							},
						},
					},
				},
			},
		}

		Expect(
			p.Update(ev),
		).To(
			BeTrue(),
		)
	})

	It("should return false on UPDATE events if non of the tags has changed", func() {

		is := imagev1.ImageStream{
			Status: imagev1.ImageStreamStatus{
				Tags: []imagev1.NamedTagEventList{
					{
						Tag: "411.86.202210072320-0",
						Items: []imagev1.TagEvent{
							{
								DockerImageReference: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:111",
							},
						},
					},
				},
			},
		}

		ev := event.UpdateEvent{
			ObjectNew: &is,
			ObjectOld: &is,
		}

		Expect(
			p.Update(ev),
		).To(
			BeFalse(),
		)
	})

	It("should return true on DELETE events", func() {
		ev := event.DeleteEvent{
			Object: &imagev1.ImageStream{},
		}

		Expect(
			p.Delete(ev),
		).To(
			BeTrue(),
		)
	})
})
