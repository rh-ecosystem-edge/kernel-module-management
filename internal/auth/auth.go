package auth

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/kubernetes"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
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
	NewRegistryAuthGetterFrom(mod *kmmv1beta1.Module) RegistryAuthGetter
}

type registryAuthGetterFactory struct {
	client        client.Client
	coreClientSet k8s.Interface
}

func NewRegistryAuthGetterFactory(client client.Client, coreClientSet k8s.Interface) RegistryAuthGetterFactory {
	return &registryAuthGetterFactory{
		client:        client,
		coreClientSet: coreClientSet,
	}
}

func (af *registryAuthGetterFactory) newRegistryAuthGetter(namespacedName types.NamespacedName) RegistryAuthGetter {
	return &registrySecretAuthGetter{
		client:         af.client,
		namespacedName: namespacedName,
	}
}

func (af *registryAuthGetterFactory) newServiceAccountRegistryAuthGetter(namespace, serviceAccountName string) RegistryAuthGetter {
	return &serviceAccountRegistryAuthGetter{
		coreClientSet:      af.coreClientSet,
		namespace:          namespace,
		serviceAccountName: serviceAccountName,
	}
}

func (af *registryAuthGetterFactory) NewRegistryAuthGetterFrom(mod *kmmv1beta1.Module) RegistryAuthGetter {
	if mod.Spec.ImageRepoSecret != nil {
		namespacedName := types.NamespacedName{
			Name:      mod.Spec.ImageRepoSecret.Name,
			Namespace: mod.Namespace,
		}
		return af.newRegistryAuthGetter(namespacedName)
	}
	return af.newServiceAccountRegistryAuthGetter(
		mod.Namespace,
		constants.OCPBuilderServiceAccountName)
}
