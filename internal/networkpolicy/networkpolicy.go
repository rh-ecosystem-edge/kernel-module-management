package networkpolicy

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/pod"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

//go:generate mockgen -source=networkpolicy.go -package=networkpolicy -destination=mock_networkpolicy.go

type NetworkPolicy interface {
	CreateOrAddOwnerReference(ctx context.Context, np *networkingv1.NetworkPolicy, owner metav1.Object) error
	WorkerPodNetworkPolicy(namespace string) *networkingv1.NetworkPolicy
	BuildSignPodsNetworkPolicy(namespace string) *networkingv1.NetworkPolicy
	PullPodNetworkPolicy(namespace string) *networkingv1.NetworkPolicy
}

type networkPolicy struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewNetworkPolicy(client client.Client, scheme *runtime.Scheme) NetworkPolicy {
	return &networkPolicy{
		client: client,
		scheme: scheme,
	}
}

const (
	workerPodNetworkPolicyName     = "kmm-worker"
	buildSignPodsNetworkPolicyName = "kmm-build-and-sign"
	pullPodNetworkPolicyName       = "kmm-pull"
	defaultNamespace               = "default"
)

// normalizeNamespace returns the provided namespace or "default" if the namespace is empty.
// This is incase the user decides to create Module/MCM without a namesapce in the metadata.
func normalizeNamespace(namespace string) string {
	if namespace == "" {
		return defaultNamespace
	}
	return namespace
}

func (np *networkPolicy) CreateOrAddOwnerReference(ctx context.Context, networkPolicy *networkingv1.NetworkPolicy, owner metav1.Object) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Creating or patching NetworkPolicy", "name", networkPolicy.Name, "namespace", networkPolicy.Namespace, "owner", owner.GetName())

	existing := &networkingv1.NetworkPolicy{}
	key := client.ObjectKey{Name: networkPolicy.Name, Namespace: networkPolicy.Namespace}

	err := np.client.Get(ctx, key, existing)
	if err == nil {
		return np.ensureOwnerReference(ctx, existing, owner)
	}

	if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to get NetworkPolicy %s/%s: %v", networkPolicy.Namespace, networkPolicy.Name, err)
	}

	if err := controllerutil.SetOwnerReference(owner, networkPolicy, np.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference on NetworkPolicy %s/%s: %v", networkPolicy.Namespace, networkPolicy.Name, err)
	}

	if err := np.client.Create(ctx, networkPolicy); err != nil {
		return fmt.Errorf("failed to create NetworkPolicy %s/%s: %v", networkPolicy.Namespace, networkPolicy.Name, err)
	}

	logger.Info("Successfully created NetworkPolicy", "name", networkPolicy.Name, "namespace", networkPolicy.Namespace, "owner", owner.GetName())
	return nil
}

func (np *networkPolicy) ensureOwnerReference(ctx context.Context, policy *networkingv1.NetworkPolicy, owner metav1.Object) error {
	logger := log.FromContext(ctx)

	for _, existingOwner := range policy.GetOwnerReferences() {
		if existingOwner.UID == owner.GetUID() {
			logger.V(1).Info("Owner already exists on NetworkPolicy", "policy", policy.Name, "owner", owner.GetName())
			return nil
		}
	}

	logger.V(1).Info("Adding owner reference to existing NetworkPolicy", "policy", policy.Name, "owner", owner.GetName())
	existingCopy := policy.DeepCopy()
	if err := controllerutil.SetOwnerReference(owner, policy, np.scheme); err != nil {
		return err
	}

	if err := np.client.Patch(ctx, policy, client.MergeFrom(existingCopy)); err != nil {
		return fmt.Errorf("failed to update NetworkPolicy owner references %s/%s: %v", policy.Namespace, policy.Name, err)
	}

	logger.Info("Successfully updated NetworkPolicy owner references", "name", policy.Name, "namespace", policy.Namespace, "owner", owner.GetName())
	return nil
}

func (np *networkPolicy) WorkerPodNetworkPolicy(namespace string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerPodNetworkPolicyName,
			Namespace: normalizeNamespace(namespace),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": "worker",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}
}

func (np *networkPolicy) BuildSignPodsNetworkPolicy(namespace string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildSignPodsNetworkPolicyName,
			Namespace: normalizeNamespace(namespace),
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
		},
	}
}

func (np *networkPolicy) PullPodNetworkPolicy(namespace string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullPodNetworkPolicyName,
			Namespace: normalizeNamespace(namespace),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      pod.PullPodTypeLabelKey,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}
}
