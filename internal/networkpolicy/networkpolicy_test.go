package networkpolicy

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/pod"
	"go.uber.org/mock/gomock"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("NetworkPolicy", func() {
	const (
		testNamespace = "test-namespace"
		testName      = "test-networkpolicy"
	)

	var (
		ctrl       *gomock.Controller
		mockClient *client.MockClient
		np         NetworkPolicy
		ctx        context.Context
		testOwner  metav1.Object
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockClient = client.NewMockClient(ctrl)
		np = NewNetworkPolicy(mockClient, scheme)
		ctx = context.Background()
		testOwner = &kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-owner",
				Namespace: testNamespace,
				UID:       "test-uid",
			},
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("NewNetworkPolicy", func() {
		It("should create a new NetworkPolicy instance", func() {
			result := NewNetworkPolicy(mockClient, scheme)
			Expect(result).NotTo(BeNil())
			Expect(result).To(BeAssignableToTypeOf(&networkPolicy{}))
		})
	})

	Describe("CreateOrAddOwnerReference", func() {
		var testPolicy *networkingv1.NetworkPolicy

		BeforeEach(func() {
			testPolicy = &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
			}
		})

		It("should return nil if NetworkPolicy already exists with owner", func() {
			existingPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							UID: testOwner.GetUID(),
						},
					},
				},
			}
			mockClient.EXPECT().
				Get(ctx, ctrlclient.ObjectKey{Name: testName, Namespace: testNamespace}, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ ctrlclient.ObjectKey, obj *networkingv1.NetworkPolicy, _ ...ctrlclient.GetOption) error {
					*obj = *existingPolicy
					return nil
				})

			err := np.CreateOrAddOwnerReference(ctx, testPolicy, testOwner)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add owner to existing NetworkPolicy without owner", func() {
			existingPolicy := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
			}
			mockClient.EXPECT().
				Get(ctx, ctrlclient.ObjectKey{Name: testName, Namespace: testNamespace}, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ ctrlclient.ObjectKey, obj *networkingv1.NetworkPolicy, _ ...ctrlclient.GetOption) error {
					*obj = *existingPolicy
					return nil
				})
			mockClient.EXPECT().
				Patch(ctx, gomock.Any(), gomock.Any()).
				Return(nil)

			err := np.CreateOrAddOwnerReference(ctx, testPolicy, testOwner)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create NetworkPolicy if it doesn't exist", func() {
			notFoundErr := k8serrors.NewNotFound(schema.GroupResource{
				Group:    "networking.k8s.io",
				Resource: "networkpolicies",
			}, testName)

			mockClient.EXPECT().
				Get(ctx, ctrlclient.ObjectKey{Name: testName, Namespace: testNamespace}, gomock.Any()).
				Return(notFoundErr)
			mockClient.EXPECT().
				Create(ctx, testPolicy).
				Return(nil)

			err := np.CreateOrAddOwnerReference(ctx, testPolicy, testOwner)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error if Get fails with non-NotFound error", func() {
			getErr := errors.New("some other error")
			mockClient.EXPECT().
				Get(ctx, ctrlclient.ObjectKey{Name: testName, Namespace: testNamespace}, gomock.Any()).
				Return(getErr)

			err := np.CreateOrAddOwnerReference(ctx, testPolicy, testOwner)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get NetworkPolicy"))
		})

		It("should return error if Create fails", func() {
			notFoundErr := k8serrors.NewNotFound(schema.GroupResource{
				Group:    "networking.k8s.io",
				Resource: "networkpolicies",
			}, testName)
			createErr := errors.New("create failed")

			mockClient.EXPECT().
				Get(ctx, ctrlclient.ObjectKey{Name: testName, Namespace: testNamespace}, gomock.Any()).
				Return(notFoundErr)
			mockClient.EXPECT().
				Create(ctx, testPolicy).
				Return(createErr)

			err := np.CreateOrAddOwnerReference(ctx, testPolicy, testOwner)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create NetworkPolicy"))
		})
	})

	Describe("WorkerPodNetworkPolicy", func() {
		It("should return correct NetworkPolicy for worker pods", func() {
			result := np.WorkerPodNetworkPolicy(testNamespace)

			Expect(result).NotTo(BeNil())
			Expect(result.Name).To(Equal(workerPodNetworkPolicyName))
			Expect(result.Namespace).To(Equal(testNamespace))

			Expect(result.Spec.PodSelector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/component", "worker"))

			Expect(result.Spec.PolicyTypes).To(ContainElements(
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			))

			Expect(result.Spec.Ingress).To(BeEmpty())
			Expect(result.Spec.Egress).To(BeEmpty())
		})
	})

	Describe("BuildSignPodsNetworkPolicy", func() {
		It("should return correct NetworkPolicy for build and sign pods", func() {
			result := np.BuildSignPodsNetworkPolicy(testNamespace)

			Expect(result).NotTo(BeNil())
			Expect(result.Name).To(Equal(buildSignPodsNetworkPolicyName))
			Expect(result.Namespace).To(Equal(testNamespace))

			Expect(result.Spec.PodSelector.MatchExpressions).To(HaveLen(1))
			Expect(result.Spec.PodSelector.MatchExpressions[0].Key).To(Equal("openshift.io/build.name"))
			Expect(result.Spec.PodSelector.MatchExpressions[0].Operator).To(Equal(metav1.LabelSelectorOpExists))

			Expect(result.Spec.PolicyTypes).To(BeEmpty())
			Expect(result.Spec.Egress).To(BeEmpty())
			Expect(result.Spec.Ingress).To(BeEmpty())
		})
	})

	Describe("PullPodNetworkPolicy", func() {
		It("should return correct NetworkPolicy for pull pods", func() {
			result := np.PullPodNetworkPolicy(testNamespace)

			Expect(result).NotTo(BeNil())
			Expect(result.Name).To(Equal(pullPodNetworkPolicyName))
			Expect(result.Namespace).To(Equal(testNamespace))

			Expect(result.Spec.PodSelector.MatchExpressions).To(HaveLen(1))
			Expect(result.Spec.PodSelector.MatchExpressions[0].Key).To(Equal(pod.PullPodTypeLabelKey))
			Expect(result.Spec.PodSelector.MatchExpressions[0].Operator).To(Equal(metav1.LabelSelectorOpExists))

			Expect(result.Spec.PolicyTypes).To(ContainElements(
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			))

			Expect(result.Spec.Ingress).To(BeEmpty())
			Expect(result.Spec.Egress).To(BeEmpty())
		})

		It("should use the correct pull pod type label key", func() {
			result := np.PullPodNetworkPolicy(testNamespace)

			Expect(result.Spec.PodSelector.MatchExpressions[0].Key).To(Equal("kmm.node.kubernetes.io/pull-pod-type"))
		})
	})

	Describe("ensureOwnerReference", func() {
		var policy *networkingv1.NetworkPolicy

		BeforeEach(func() {
			policy = &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
			}
		})

		It("should return nil if owner already exists", func() {
			policy.SetOwnerReferences([]metav1.OwnerReference{{UID: testOwner.GetUID()}})

			err := np.(*networkPolicy).ensureOwnerReference(ctx, policy, testOwner)
			Expect(err).NotTo(HaveOccurred())
			Expect(policy.GetOwnerReferences()[0].UID).To(Equal(testOwner.GetUID()))

		})

		It("should add owner and patch if owner doesn't exist", func() {
			mockClient.EXPECT().
				Patch(ctx, gomock.Any(), gomock.Any()).
				Return(nil)

			err := np.(*networkPolicy).ensureOwnerReference(ctx, policy, testOwner)
			Expect(err).NotTo(HaveOccurred())
			Expect(policy.GetOwnerReferences()[0].UID).To(Equal(testOwner.GetUID()))

		})

		It("should return error if patch fails", func() {
			patchErr := errors.New("patch failed")
			mockClient.EXPECT().
				Patch(ctx, gomock.Any(), gomock.Any()).
				Return(patchErr)

			err := np.(*networkPolicy).ensureOwnerReference(ctx, policy, testOwner)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update NetworkPolicy owner references"))
		})
	})

})
