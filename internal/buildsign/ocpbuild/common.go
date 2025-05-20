package ocpbuild

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"strings"

	"github.com/mitchellh/hashstructure/v2"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

const dtkBuildArg = "DTK_AUTO"

type TemplateData struct {
	FilesToSign   []string
	SignImage     string
	UnsignedImage string
}

var (
	//go:embed templates
	templateFS embed.FS

	tmpl = template.Must(
		template.ParseFS(templateFS, "templates/Dockerfile.gotmpl"),
	)
)

func envVarsFromKMMBuildArgs(args []kmmv1beta1.BuildArg) []v1.EnvVar {
	if args == nil {
		return nil
	}

	ev := make([]v1.EnvVar, 0, len(args))

	for _, ba := range args {
		ev = append(ev, v1.EnvVar{Name: ba.Name, Value: ba.Value})
	}

	return ev
}

func (omi *ocpbuildManagerImpl) getSecretData(ctx context.Context, secretName, secretDataKey, namespace string) ([]byte, error) {

	secret := v1.Secret{}
	namespacedName := types.NamespacedName{Name: secretName, Namespace: namespace}

	if err := omi.client.Get(ctx, namespacedName, &secret); err != nil {
		return nil, fmt.Errorf("error while getting secret %s: %v", namespacedName, err)
	}

	data, ok := secret.Data[secretDataKey]
	if !ok {
		return nil, fmt.Errorf("invalid Secret %s format, %s key is missing", namespacedName, secretDataKey)
	}

	return data, nil
}

func filterOCPBuildsByOwner(builds []buildv1.Build, owner metav1.Object) []buildv1.Build {
	ownedBuilds := []buildv1.Build{}
	for _, build := range builds {
		if metav1.IsControlledBy(&build, owner) {
			ownedBuilds = append(ownedBuilds, build)
		}
	}
	return ownedBuilds
}

func moduleKernelLabels(modName, kernelVersion, ocpbuildType string) map[string]string {
	labels := moduleLabels(modName, ocpbuildType)
	labels[constants.TargetKernelTarget] = kernelVersion
	return labels
}

func moduleLabels(modName, ocpbuildType string) map[string]string {
	return map[string]string{
		constants.ModuleNameLabel: modName,
		constants.BuildTypeLabel:  ocpbuildType,
	}
}

func (omi *ocpbuildManagerImpl) getDockerfileData(ctx context.Context, buildConfig *kmmv1beta1.Build, namespace string) (string, error) {
	dockerfileCM := &v1.ConfigMap{}
	namespacedName := types.NamespacedName{Name: buildConfig.DockerfileConfigMap.Name, Namespace: namespace}
	err := omi.client.Get(ctx, namespacedName, dockerfileCM)
	if err != nil {
		return "", fmt.Errorf("failed to get dockerfile ConfigMap %s: %v", namespacedName, err)
	}
	data, ok := dockerfileCM.Data[constants.DockerfileCMKey]
	if !ok {
		return "", fmt.Errorf("invalid Dockerfile ConfigMap %s format, %s key is missing", namespacedName, constants.DockerfileCMKey)
	}
	return data, nil
}

func (omi *ocpbuildManagerImpl) ocpbuildAnnotations(hash uint64) map[string]string {
	return map[string]string{constants.HashAnnotation: fmt.Sprintf("%d", hash)}
}

func (omi *ocpbuildManagerImpl) getBuildHashAnnotationValue(ctx context.Context, dockerfileData string) (uint64, error) {

	sourceConfig := buildv1.BuildSource{
		Dockerfile: &dockerfileData,
		Type:       buildv1.BuildSourceDockerfile,
	}

	sourceConfigHash, err := hashstructure.Hash(sourceConfig, hashstructure.FormatV2, nil)
	if err != nil {
		return 0, fmt.Errorf("could not hash Build's Buildsource template: %v", err)
	}

	return sourceConfigHash, nil
}

func (omi *ocpbuildManagerImpl) getSignHashAnnotationValue(ctx context.Context, privateSecret, publicSecret, namespace string,
	buildSpec *buildv1.BuildSpec) (uint64, error) {

	publicKeyData, err := omi.getSecretData(ctx, publicSecret, constants.PublicSignDataKey, namespace)
	if err != nil {
		return 0, fmt.Errorf("could not get cert bytes: %v", err)
	}

	privateKeyData, err := omi.getSecretData(ctx, privateSecret, constants.PrivateSignDataKey, namespace)
	if err != nil {
		return 0, fmt.Errorf("could not get key bytes: %v", err)
	}

	dataToHash := struct {
		BuildSpec      *buildv1.BuildSpec
		PrivateKeyData []byte
		PublicKeyData  []byte
	}{
		BuildSpec:      buildSpec,
		PrivateKeyData: privateKeyData,
		PublicKeyData:  publicKeyData,
	}

	hashValue, err := hashstructure.Hash(dataToHash, hashstructure.FormatV2, nil)
	if err != nil {
		return 0, fmt.Errorf("could not hash Build's spec and signing keys: %v", err)
	}

	return hashValue, nil
}

func (omi *ocpbuildManagerImpl) buildSpec(mld *api.ModuleLoaderData, dockerfileData, destinationImg string,
	pushImage bool) (*buildv1.BuildSpec, error) {

	kernelVersion := mld.KernelVersion
	buildConfig := mld.Build

	overrides := []kmmv1beta1.BuildArg{
		{Name: "KERNEL_VERSION", Value: kernelVersion},
		{Name: "KERNEL_FULL_VERSION", Value: kernelVersion},
		{Name: "MOD_NAME", Value: mld.Name},
		{Name: "MOD_NAMESPACE", Value: mld.Namespace},
	}
	if strings.Contains(dockerfileData, dtkBuildArg) {
		dtkImage, err := omi.kernelOsDtkMapping.GetImage(kernelVersion)
		if err != nil {
			return nil, fmt.Errorf("could not get DTK image for kernel %v: %v", kernelVersion, err)
		}
		overrides = append(overrides, kmmv1beta1.BuildArg{Name: dtkBuildArg, Value: dtkImage})
	}
	buildArgs := omi.combiner.ApplyBuildArgOverrides(
		buildConfig.BuildArgs,
		overrides...,
	)
	envArgs := envVarsFromKMMBuildArgs(buildArgs)

	buildTarget := buildv1.BuildOutput{}
	if pushImage {
		buildTarget = buildv1.BuildOutput{
			To: &v1.ObjectReference{
				Kind: "DockerImage",
				Name: destinationImg,
			},
			PushSecret: mld.ImageRepoSecret,
		}
	}

	selector := mld.Selector
	if len(mld.Build.Selector) != 0 {
		selector = mld.Build.Selector
	}

	volumes := makeBuildResourceVolumes(buildConfig)

	spec := &buildv1.BuildSpec{
		CommonSpec: buildv1.CommonSpec{
			ServiceAccount: constants.OCPBuilderServiceAccountName,
			Source: buildv1.BuildSource{
				Dockerfile: &dockerfileData,
				Type:       buildv1.BuildSourceDockerfile,
			},
			Strategy: buildv1.BuildStrategy{
				Type: buildv1.DockerBuildStrategyType,
				DockerStrategy: &buildv1.DockerBuildStrategy{
					BuildArgs:  envArgs,
					Volumes:    volumes,
					PullSecret: mld.ImageRepoSecret,
				},
			},
			Output:         buildTarget,
			NodeSelector:   selector,
			MountTrustedCA: ptr.To(true),
		},
	}

	return spec, nil
}

func (omi *ocpbuildManagerImpl) signSpec(mld *api.ModuleLoaderData, dockerfileData string, pushImage bool) buildv1.BuildSpec {

	buildTarget := buildv1.BuildOutput{}
	if pushImage {
		buildTarget = buildv1.BuildOutput{
			To: &v1.ObjectReference{
				Kind: "DockerImage",
				Name: mld.ContainerImage,
			},
			PushSecret: mld.ImageRepoSecret,
		}
	}

	volumes := makeSignResourceVolumes(mld.Sign)

	spec := buildv1.BuildSpec{
		CommonSpec: buildv1.CommonSpec{
			ServiceAccount: constants.OCPBuilderServiceAccountName,
			Source: buildv1.BuildSource{
				Dockerfile: &dockerfileData,
				Type:       buildv1.BuildSourceDockerfile,
			},
			Strategy: buildv1.BuildStrategy{
				Type: buildv1.DockerBuildStrategyType,
				DockerStrategy: &buildv1.DockerBuildStrategy{
					Volumes:    volumes,
					PullSecret: mld.ImageRepoSecret,
				},
			},
			Output:         buildTarget,
			NodeSelector:   mld.Selector,
			MountTrustedCA: ptr.To(true),
		},
	}

	return spec
}
