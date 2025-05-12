package resource

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/mitchellh/hashstructure/v2"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/module"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const dtkBuildArg = "DTK_AUTO"

type TemplateData struct {
	FilesToSign   []string
	SignImage     string
	UnsignedImage string
}

//go:embed templates
var templateFS embed.FS

var tmpl = template.Must(
	template.ParseFS(templateFS, "templates/Dockerfile.gotmpl"),
)

func (rm *resourceManager) buildSpec(mld *api.ModuleLoaderData, dockerfileData, destinationImg string,
	pushImage bool) (*buildv1.BuildSpec, error) {

	buildConfig := mld.Build

	overrides := []kmmv1beta1.BuildArg{
		{Name: "KERNEL_VERSION", Value: mld.KernelVersion},
		{Name: "KERNEL_FULL_VERSION", Value: mld.KernelVersion},
		{Name: "MOD_NAME", Value: mld.Name},
		{Name: "MOD_NAMESPACE", Value: mld.Namespace},
	}
	if strings.Contains(dockerfileData, dtkBuildArg) {
		dtkImage, err := rm.kernelOsDtkMapping.GetImage(mld.KernelVersion)
		if err != nil {
			return nil, fmt.Errorf("could not get DTK image for kernel %v: %v", mld.KernelVersion, err)
		}
		overrides = append(overrides, kmmv1beta1.BuildArg{Name: dtkBuildArg, Value: dtkImage})
	}
	buildArgs := rm.buildArgOverrider.ApplyBuildArgOverrides(
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

func signSpec(mld *api.ModuleLoaderData, dockerfileData string, pushImage bool) buildv1.BuildSpec {

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

func (rm *resourceManager) getBuildHashAnnotationValue(ctx context.Context, dockerfileData string) (uint64, error) {

	sourceConfig := buildv1.BuildSource{
		Dockerfile: &dockerfileData,
		Type:       buildv1.BuildSourceDockerfile,
	}

	sourceConfigHash, err := hashstructure.Hash(sourceConfig, hashstructure.FormatV2, nil)
	if err != nil {
		return 0, fmt.Errorf("could not hash build's Buildsource template: %v", err)
	}

	return sourceConfigHash, nil
}

func (rm *resourceManager) getSignHashAnnotationValue(ctx context.Context, privateSecret, publicSecret, namespace string,
	signSpec *buildv1.BuildSpec) (uint64, error) {

	privateKeyData, err := rm.getSecretData(ctx, privateSecret, constants.PrivateSignDataKey, namespace)
	if err != nil {
		return 0, fmt.Errorf("failed to get private secret %s for signing: %v", privateSecret, err)
	}
	publicKeyData, err := rm.getSecretData(ctx, publicSecret, constants.PublicSignDataKey, namespace)
	if err != nil {
		return 0, fmt.Errorf("failed to get public secret %s for signing: %v", publicSecret, err)
	}

	dataToHash := struct {
		SignSpec       *buildv1.BuildSpec
		PrivateKeyData []byte
		PublicKeyData  []byte
	}{
		SignSpec:       signSpec,
		PrivateKeyData: privateKeyData,
		PublicKeyData:  publicKeyData,
	}
	hashValue, err := hashstructure.Hash(dataToHash, hashstructure.FormatV2, nil)
	if err != nil {
		return 0, fmt.Errorf("could not hash sign's spec template and signing keys: %v", err)
	}

	return hashValue, nil
}

func (rm *resourceManager) getSecretData(ctx context.Context, secretName, secretDataKey, namespace string) ([]byte, error) {
	secret := v1.Secret{}
	namespacedName := types.NamespacedName{Name: secretName, Namespace: namespace}
	err := rm.client.Get(ctx, namespacedName, &secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get Secret %s: %v", namespacedName, err)
	}
	data, ok := secret.Data[secretDataKey]
	if !ok {
		return nil, fmt.Errorf("invalid Secret %s format, %s key is missing", namespacedName, secretDataKey)
	}
	return data, nil
}

func resourceLabels(modName, targetKernel string, resourceType kmmv1beta1.BuildOrSignAction) map[string]string {

	labels := moduleKernelLabels(modName, targetKernel, resourceType)

	labels["app.kubernetes.io/name"] = "kmm"
	labels["app.kubernetes.io/component"] = string(resourceType)
	labels["app.kubernetes.io/part-of"] = "kmm"

	return labels
}

func filterResourcesByOwner(resources []buildv1.Build, owner metav1.Object) []buildv1.Build {
	ownedResources := []buildv1.Build{}
	for _, obj := range resources {
		if metav1.IsControlledBy(&obj, owner) {
			ownedResources = append(ownedResources, obj)
		}
	}
	return ownedResources
}

func moduleKernelLabels(moduleName, targetKernel string, resourceType kmmv1beta1.BuildOrSignAction) map[string]string {
	labels := moduleLabels(moduleName, resourceType)
	labels[constants.TargetKernelTarget] = targetKernel
	return labels
}

func moduleLabels(moduleName string, resourceType kmmv1beta1.BuildOrSignAction) map[string]string {
	return map[string]string{
		constants.ModuleNameLabel: moduleName,
		constants.ResourceType:    string(resourceType),
	}
}

func (rm *resourceManager) getResources(ctx context.Context, namespace string, labels map[string]string) ([]buildv1.Build, error) {
	resourceList := buildv1.BuildList{}
	opts := []client.ListOption{
		client.MatchingLabels(labels),
		client.InNamespace(namespace),
	}
	if err := rm.client.List(ctx, &resourceList, opts...); err != nil {
		return nil, fmt.Errorf("could not list resources: %v", err)
	}

	return resourceList.Items, nil
}

func (rm *resourceManager) makeBuildTemplate(ctx context.Context, mld *api.ModuleLoaderData, owner metav1.Object,
	pushImage bool) (metav1.Object, error) {

	// if build AND sign are specified, then we will build an intermediate image
	// and let sign produce the final image specified in spec.moduleLoader.container.km.containerImage
	containerImage := mld.ContainerImage
	if module.ShouldBeSigned(mld) {
		containerImage = module.IntermediateImageName(mld.Name, mld.Namespace, containerImage)
	}

	dockerfileData, err := rm.getDockerfileData(ctx, mld.Build, mld.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get dockerfile data from configmap: %v", err)
	}

	buildSpec, err := rm.buildSpec(mld, dockerfileData, containerImage, pushImage)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Build spec: %v", err)
	}
	sourceConfigHash, err := rm.getBuildHashAnnotationValue(ctx, dockerfileData)
	if err != nil {
		return nil, fmt.Errorf("failed to get build annotation value: %v", err)
	}

	build := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mld.Name + "-build-",
			Namespace:    mld.Namespace,
			Labels:       resourceLabels(mld.Name, mld.KernelNormalizedVersion, kmmv1beta1.BuildImage),
			Annotations:  map[string]string{constants.ResourceHashAnnotation: fmt.Sprintf("%d", sourceConfigHash)},
			Finalizers:   []string{constants.GCDelayFinalizer, constants.JobEventFinalizer},
		},
		Spec: *buildSpec,
	}

	if err := controllerutil.SetControllerReference(owner, build, rm.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return build, nil
}

func (rm *resourceManager) makeSignTemplate(ctx context.Context, mld *api.ModuleLoaderData, owner metav1.Object,
	pushImage bool) (metav1.Object, error) {

	signConfig := mld.Sign

	var buf bytes.Buffer

	td := TemplateData{
		FilesToSign: mld.Sign.FilesToSign,
		SignImage:   os.Getenv("RELATED_IMAGE_SIGN"),
	}

	imageToSign := ""
	if module.ShouldBeBuilt(mld) {
		imageToSign = module.IntermediateImageName(mld.Name, mld.Namespace, mld.ContainerImage)
	}

	if imageToSign != "" {
		td.UnsignedImage = imageToSign
	} else if signConfig.UnsignedImage != "" {
		td.UnsignedImage = signConfig.UnsignedImage
	} else {
		return nil, fmt.Errorf("no image to sign given")
	}

	if err := tmpl.Execute(&buf, td); err != nil {
		return nil, fmt.Errorf("could not execute template: %v", err)
	}
	dockerfileData := buf.String()

	signSpec := signSpec(mld, dockerfileData, pushImage)
	signSpecHash, err := rm.getSignHashAnnotationValue(ctx, signConfig.KeySecret.Name,
		signConfig.CertSecret.Name, mld.Namespace, &signSpec)
	if err != nil {
		return nil, fmt.Errorf("could not hash resource's definitions: %v", err)
	}

	sign := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mld.Name + "-sign-",
			Namespace:    mld.Namespace,
			Labels:       resourceLabels(mld.Name, mld.KernelNormalizedVersion, kmmv1beta1.SignImage),
			Annotations: map[string]string{
				constants.ResourceHashAnnotation: fmt.Sprintf("%d", signSpecHash),
			},
			Finalizers: []string{constants.GCDelayFinalizer, constants.JobEventFinalizer},
		},
		Spec: signSpec,
	}

	if err = controllerutil.SetControllerReference(owner, sign, rm.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return sign, nil
}

func (rm *resourceManager) getDockerfileData(ctx context.Context, buildConfig *kmmv1beta1.Build, namespace string) (string, error) {
	dockerfileCM := &v1.ConfigMap{}
	namespacedName := types.NamespacedName{Name: buildConfig.DockerfileConfigMap.Name, Namespace: namespace}
	err := rm.client.Get(ctx, namespacedName, dockerfileCM)
	if err != nil {
		return "", fmt.Errorf("failed to get dockerfile ConfigMap %s: %v", namespacedName, err)
	}
	data, ok := dockerfileCM.Data[constants.DockerfileCMKey]
	if !ok {
		return "", fmt.Errorf("invalid Dockerfile ConfigMap %s format, %s key is missing", namespacedName, constants.DockerfileCMKey)
	}
	return data, nil
}
