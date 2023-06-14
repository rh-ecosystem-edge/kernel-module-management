package module

import (
	"context"
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
)

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

// ShouldBeBuilt indicates whether the specified ModuleLoaderData of the
// Module should be built or not.
func ShouldBeBuilt(mld *api.ModuleLoaderData) bool {
	return mld.Build != nil
}

// ShouldBeSigned indicates whether the specified ModuleLoaderData of the
// Module should be signed or not.
func ShouldBeSigned(mld *api.ModuleLoaderData) bool {
	return mld.Sign != nil
}

func ImageDigest(
	ctx context.Context,
	authFactory auth.RegistryAuthGetterFactory,
	reg registry.Registry,
	mld *api.ModuleLoaderData,
	imageName string) (string, error) {

	registryAuthGetter := authFactory.NewRegistryAuthGetterFrom(mld)
	digest, err := reg.GetDigest(ctx, imageName, mld.RegistryTLS, registryAuthGetter)
	if err != nil {
		return "", fmt.Errorf("could not get image digest: %v", err)
	}

	return digest, nil
}

func ImageExists(
	ctx context.Context,
	authFactory auth.RegistryAuthGetterFactory,
	reg registry.Registry,
	mld *api.ModuleLoaderData,
	imageName string) (bool, error) {

	registryAuthGetter := authFactory.NewRegistryAuthGetterFrom(mld)
	exists, err := reg.ImageExists(ctx, imageName, mld.RegistryTLS, registryAuthGetter)
	if err != nil {
		return false, fmt.Errorf("could not check if the image is available: %v", err)
	}

	return exists, nil
}
