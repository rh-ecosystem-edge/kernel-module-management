package networkpolicy

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mockclient "github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
)

var _ = Describe("NetworkPolicy Manager", func() {
	var (
		scheme        *runtime.Scheme
		testNamespace string
		testPrefix    string
		mgr           *manager
	)

	BeforeEach(func() {
		testNamespace = "test-namespace"
		testPrefix = "test-operator"

		// Setup scheme
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(networkingv1.AddToScheme(scheme)).To(Succeed())

		mgr = &manager{
			client: nil,
			scheme: scheme,
		}
	})

	Describe("createDefaultDenyPolicy", func() {
		It("should create default deny policy with correct spec", func() {
			policy := mgr.createDefaultDenyPolicy(testNamespace, testPrefix)

			Expect(policy.Name).To(Equal(testPrefix + "-default-deny"))
			Expect(policy.Namespace).To(Equal(testNamespace))
			Expect(policy.Spec.PodSelector).To(Equal(metav1.LabelSelector{}))
			Expect(policy.Spec.PolicyTypes).To(ContainElements(
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			))
		})
	})

	Describe("createControllerPolicy", func() {
		It("should create controller policy with correct spec", func() {
			policy := mgr.createControllerPolicy(testNamespace, testPrefix)

			Expect(policy.Name).To(Equal(testPrefix + "-controller"))
			Expect(policy.Namespace).To(Equal(testNamespace))
			Expect(policy.Spec.PodSelector.MatchLabels).To(HaveKeyWithValue("control-plane", "controller"))

			Expect(policy.Spec.Ingress).To(HaveLen(1))
			ingressRule := policy.Spec.Ingress[0]
			Expect(ingressRule.Ports).To(HaveLen(1))

			webhookPort := ingressRule.Ports[0]
			Expect(webhookPort.Protocol).To(Equal(ptr.To(corev1.ProtocolTCP)))
			Expect(webhookPort.Port).To(Equal(ptr.To(intstr.FromInt32(8443))))
		})
	})

	Describe("createWebhookPolicy", func() {
		It("should create webhook policy with correct spec", func() {
			policy := mgr.createWebhookPolicy(testNamespace, testPrefix)

			Expect(policy.Name).To(Equal(testPrefix + "-webhook"))
			Expect(policy.Namespace).To(Equal(testNamespace))
			Expect(policy.Spec.PodSelector.MatchLabels).To(HaveKeyWithValue("control-plane", "webhook-server"))

			Expect(policy.Spec.Ingress).To(HaveLen(1))
			ingressRule := policy.Spec.Ingress[0]
			Expect(ingressRule.Ports).To(HaveLen(1))

			webhookPort := ingressRule.Ports[0]
			Expect(webhookPort.Protocol).To(Equal(ptr.To(corev1.ProtocolTCP)))
			Expect(webhookPort.Port).To(Equal(ptr.To(intstr.FromInt32(9443))))
		})
	})

	Describe("createBuildAndSignPolicy", func() {
		It("should create build and sign policy with correct spec", func() {
			policy := mgr.createBuildAndSignPolicy(testNamespace, testPrefix)

			Expect(policy.Name).To(Equal(testPrefix + "-build-and-sign"))
			Expect(policy.Namespace).To(Equal(testNamespace))

			Expect(policy.Spec.PodSelector.MatchExpressions).To(HaveLen(1))
			expr := policy.Spec.PodSelector.MatchExpressions[0]
			Expect(expr.Key).To(Equal("openshift.io/build.name"))
			Expect(expr.Operator).To(Equal(metav1.LabelSelectorOpExists))

			Expect(policy.Spec.Egress).To(HaveLen(1))
			egressRule := policy.Spec.Egress[0]
			Expect(egressRule.Ports).To(BeNil())
			Expect(egressRule.To).To(BeNil())
		})
	})
})

var _ = Describe("DeployNetworkPolicies", func() {
	var (
		ctx           context.Context
		mockCtrl      *gomock.Controller
		mockClient    *mockclient.MockClient
		manager       Manager
		testNamespace string
		testPrefix    string
		replicaSet    *appsv1.ReplicaSet
		scheme        *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = "test-namespace"
		testPrefix = "test-operator"
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = mockclient.NewMockClient(mockCtrl)

		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(networkingv1.AddToScheme(scheme)).To(Succeed())

		manager = NewManager(mockClient, scheme)

		replicaSet = &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-replicaset",
				Namespace: testNamespace,
				UID:       types.UID("test-uid"),
			},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("when all policies are created successfully", func() {
		It("should create all 4 network policies with owner references", func() {
			mockClient.EXPECT().
				Create(ctx, gomock.Any()).
				DoAndReturn(func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
					policy, ok := obj.(*networkingv1.NetworkPolicy)
					Expect(ok).To(BeTrue())

					Expect(policy.GetOwnerReferences()).To(HaveLen(1))
					ownerRef := policy.GetOwnerReferences()[0]
					Expect(ownerRef.APIVersion).To(Equal("apps/v1"))
					Expect(ownerRef.Kind).To(Equal("ReplicaSet"))
					Expect(ownerRef.Name).To(Equal(replicaSet.Name))
					Expect(ownerRef.UID).To(Equal(replicaSet.UID))
					Expect(*ownerRef.Controller).To(BeTrue())
					Expect(*ownerRef.BlockOwnerDeletion).To(BeTrue())

					Expect(policy.Namespace).To(Equal(testNamespace))
					Expect(policy.Name).To(ContainSubstring(testPrefix))

					return nil
				}).
				Times(4)

			err := manager.DeployNetworkPolicies(ctx, testNamespace, replicaSet, testPrefix)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when policies already exist", func() {
		It("should not return error for existing policies", func() {
			mockClient.EXPECT().
				Create(ctx, gomock.Any()).
				Return(apierrors.NewAlreadyExists(schema.GroupResource{
					Group:    "networking.k8s.io",
					Resource: "networkpolicies",
				}, "test-policy")).
				Times(4)

			err := manager.DeployNetworkPolicies(ctx, testNamespace, replicaSet, testPrefix)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when client returns an error", func() {
		It("should return error when Create fails", func() {
			mockClient.EXPECT().
				Create(ctx, gomock.Any()).
				Return(fmt.Errorf("create failed")).
				Times(1)

			err := manager.DeployNetworkPolicies(ctx, testNamespace, replicaSet, testPrefix)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create network policy"))
		})
	})
})
