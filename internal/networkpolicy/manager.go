package networkpolicy

//go:generate mockgen -source=manager.go -package=networkpolicy -destination=mock_manager.go Manager

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Manager interface {
	DeployNetworkPolicies(ctx context.Context, namespace string, owner metav1.Object, namePrefix string) error
}

type manager struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewManager(client client.Client, scheme *runtime.Scheme) Manager {
	return &manager{
		client: client,
		scheme: scheme,
	}
}

// DeployNetworkPolicies deploys all required network policies to the specified namespace
func (m *manager) DeployNetworkPolicies(ctx context.Context, namespace string, owner metav1.Object, namePrefix string) error {
	logger := log.FromContext(ctx).WithName("network-policy-manager")

	policies := []*networkingv1.NetworkPolicy{
		m.createDefaultDenyPolicy(namespace, namePrefix),
		m.createControllerPolicy(namespace, namePrefix),
		m.createWebhookPolicy(namespace, namePrefix),
		m.createBuildAndSignPolicy(namespace, namePrefix),
	}

	for _, policy := range policies {
		if owner != nil {
			ownerRef := metav1.OwnerReference{
				APIVersion:         "apps/v1",
				Kind:               "ReplicaSet",
				Name:               owner.GetName(),
				UID:                owner.GetUID(),
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}
			policy.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
		}

		logger.Info("Deploying network policy", "name", policy.Name, "namespace", namespace)

		if err := m.client.Create(ctx, policy); err != nil {
			if client.IgnoreAlreadyExists(err) != nil {
				return fmt.Errorf("failed to create network policy %s: %v", policy.Name, err)
			}
			logger.Info("Network policy already exists", "name", policy.Name)
		} else {
			logger.Info("Successfully created network policy", "name", policy.Name)
		}
	}

	return nil
}

// createDefaultDenyPolicy creates the default deny network policy
func (m *manager) createDefaultDenyPolicy(namespace string, namePrefix string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namePrefix + "-default-deny",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}
}

// createControllerPolicy creates the controller network policy
func (m *manager) createControllerPolicy(namespace string, namePrefix string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namePrefix + "-controller",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control-plane": "controller",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: ptr.To(corev1.ProtocolTCP),
							Port:     ptr.To(intstr.FromInt32(8443)),
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "openshift-dns",
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"dns.operator.openshift.io/daemonset-dns": "default",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: ptr.To(corev1.ProtocolUDP),
							Port:     ptr.To(intstr.FromInt32(53)),
						},
						{
							Protocol: ptr.To(corev1.ProtocolTCP),
							Port:     ptr.To(intstr.FromInt32(53)),
						},
					},
				},
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: ptr.To(corev1.ProtocolTCP),
							Port:     ptr.To(intstr.FromInt32(6443)),
						},
						{
							Protocol: ptr.To(corev1.ProtocolTCP),
							Port:     ptr.To(intstr.FromInt32(443)),
						},
					},
				},
			},
		},
	}
}

// createWebhookPolicy creates the webhook network policy
func (m *manager) createWebhookPolicy(namespace string, namePrefix string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namePrefix + "-webhook",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control-plane": "webhook-server",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: ptr.To(corev1.ProtocolTCP),
							Port:     ptr.To(intstr.FromInt32(9443)),
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: ptr.To(corev1.ProtocolTCP),
							Port:     ptr.To(intstr.FromInt32(6443)),
						},
						{
							Protocol: ptr.To(corev1.ProtocolTCP),
							Port:     ptr.To(intstr.FromInt32(443)),
						},
					},
				},
			},
		},
	}
}

// createBuildAndSignPolicy creates the build and sign network policy
func (m *manager) createBuildAndSignPolicy(namespace string, namePrefix string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namePrefix + "-build-and-sign",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "openshift.io/build.name",
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{}, // Allow all egress
			},
		},
	}
}
