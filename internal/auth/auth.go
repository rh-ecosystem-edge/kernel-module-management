package auth

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	k8s "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -source=auth.go -package=auth -destination=mock_auth.go

type RegistryAuthGetter interface {
	GetKeyChain(ctx context.Context) (authn.Keychain, error)
}

type registrySecretAuthGetter struct {
	client         client.Client
	namespacedName types.NamespacedName
}

func (rsag *registrySecretAuthGetter) GetKeyChain(ctx context.Context) (authn.Keychain, error) {

	secret := v1.Secret{}
	if err := rsag.client.Get(ctx, rsag.namespacedName, &secret); err != nil {
		return nil, fmt.Errorf("cannot find secret %s: %w", rsag.namespacedName, err)
	}

	keychain, err := kubernetes.NewFromPullSecrets(ctx, []v1.Secret{secret})
	if err != nil {
		return nil, fmt.Errorf("could not create a keycahin from secret %v: %w", secret, err)
	}

	return keychain, nil
}

type serviceAccountRegistryAuthGetter struct {
	coreClientSet      k8s.Interface
	namespace          string
	serviceAccountName string
}

func (sarag *serviceAccountRegistryAuthGetter) GetKeyChain(ctx context.Context) (authn.Keychain, error) {
	opts := kubernetes.Options{
		Namespace:          sarag.namespace,
		ServiceAccountName: sarag.serviceAccountName,
	}

	keychain, err := kubernetes.New(ctx, sarag.coreClientSet, opts)
	if err != nil {
		return nil, fmt.Errorf("could not get the service account's pull secrets: %v", err)
	}

	return keychain, nil
}

type RegistryAuthGetterFactory interface {
	NewRegistryAuthGetter(client client.Client, namespacedName types.NamespacedName) RegistryAuthGetter
	NewServiceAccountRegistryAuthGetter(coreClientSet k8s.Interface, namespace, serviceAccountName string) RegistryAuthGetter
}

type registryAuthGetterFactory struct{}

func NewRegistryAuthGetterFactory() RegistryAuthGetterFactory {
	return registryAuthGetterFactory{}
}

func (registryAuthGetterFactory) NewRegistryAuthGetter(client client.Client, namespacedName types.NamespacedName) RegistryAuthGetter {
	return &registrySecretAuthGetter{
		client:         client,
		namespacedName: namespacedName,
	}
}

func (registryAuthGetterFactory) NewServiceAccountRegistryAuthGetter(coreClientSet k8s.Interface, namespace, serviceAccountName string) RegistryAuthGetter {
	return &serviceAccountRegistryAuthGetter{
		coreClientSet:      coreClientSet,
		namespace:          namespace,
		serviceAccountName: serviceAccountName,
	}
}
