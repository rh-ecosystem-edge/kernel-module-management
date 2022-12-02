package module

import (
	"context"
	"fmt"
	"strings"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
)

func TLSOptions(modSpec kmmv1beta1.ModuleSpec, km kmmv1beta1.KernelMapping) *kmmv1beta1.TLSOptions {
	if km.RegistryTLS != nil {
		return km.RegistryTLS
	}
	return modSpec.ModuleLoader.Container.RegistryTLS
}

// AppendToTag adds the specified tag to the image name cleanly, i.e. by avoiding messing up
// the name or getting "name:-tag"
func AppendToTag(name string, tag string) string {
	separator := ":"
	if strings.Contains(name, ":") {
		separator = "_"
	}
	return name + separator + tag
}

// IntermediateImageName returns the image name of the pre-signed module image name
func IntermediateImageName(name, namespace, targetImage string) string {
	return AppendToTag(targetImage, namespace+"_"+name+"_kmm_unsigned")
}

// ShouldBeBuilt indicates whether the specified KernelMapping of the
// Module should be built or not.
func ShouldBeBuilt(modSpec kmmv1beta1.ModuleSpec, km kmmv1beta1.KernelMapping) bool {
	return modSpec.ModuleLoader.Container.Build != nil || km.Build != nil
}

// ShouldBeBuilt indicates whether the specified KernelMapping of the
// Module should be signed or not.
func ShouldBeSigned(modSpec kmmv1beta1.ModuleSpec, km kmmv1beta1.KernelMapping) bool {
	return modSpec.ModuleLoader.Container.Sign != nil || km.Sign != nil
}

func ImageExists(
	ctx context.Context,
	authFactory auth.RegistryAuthGetterFactory,
	reg registry.Registry,
	mod kmmv1beta1.Module,
	km kmmv1beta1.KernelMapping,
	imageName string) (bool, error) {

	registryAuthGetter := authFactory.NewRegistryAuthGetterFrom(&mod)

	tlsOptions := TLSOptions(mod.Spec, km)
	exists, err := reg.ImageExists(ctx, imageName, tlsOptions, registryAuthGetter)
	if err != nil {
		return false, fmt.Errorf("could not check if the image is available: %v", err)
	}

	return exists, nil
}
