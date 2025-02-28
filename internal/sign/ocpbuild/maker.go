package ocpbuild

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"text/template"

	"github.com/mitchellh/hashstructure/v2"
	buildv1 "github.com/openshift/api/build/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	ocpbuildutils "github.com/rh-ecosystem-edge/kernel-module-management/internal/utils/ocpbuild"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

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

const BuildType = "sign"

//go:generate mockgen -source=maker.go -package=ocpbuild -destination=mock_maker.go Maker

type Maker interface {
	MakeBuildTemplate(
		ctx context.Context,
		mld *api.ModuleLoaderData,
		imageToSign string,
		pushImage bool,
		owner metav1.Object,
	) (*buildv1.Build, error)
}

type maker struct {
	client    client.Client
	scheme    *runtime.Scheme
	signImage string
}

func NewMaker(client client.Client, signImage string, scheme *runtime.Scheme) Maker {
	return &maker{
		client:    client,
		scheme:    scheme,
		signImage: signImage,
	}
}

func (m *maker) MakeBuildTemplate(
	ctx context.Context,
	mld *api.ModuleLoaderData,
	imageToSign string,
	pushImage bool,
	owner metav1.Object) (*buildv1.Build, error) {

	signConfig := mld.Sign

	var buf bytes.Buffer

	td := TemplateData{
		FilesToSign: mld.Sign.FilesToSign,
		SignImage:   m.signImage,
	}

	if imageToSign != "" {
		td.UnsignedImage = imageToSign
	} else if signConfig.UnsignedImage != "" {
		td.UnsignedImage = signConfig.UnsignedImage
	} else {
		return nil, fmt.Errorf("no image to sign given")
	}

	if err := tmpl.Execute(&buf, td); err != nil {
		return nil, fmt.Errorf("could not render Dockerfile: %v", err)
	}

	dockerfile := buf.String()

	buildTarget := buildv1.BuildOutput{
		To: &v1.ObjectReference{
			Kind: "DockerImage",
			Name: mld.ContainerImage,
		},
		PushSecret: mld.ImageRepoSecret,
	}
	if !pushImage {
		buildTarget = buildv1.BuildOutput{}
	}

	sourceConfig := buildv1.BuildSource{
		Dockerfile: &dockerfile,
		Type:       buildv1.BuildSourceDockerfile,
	}

	spec := buildv1.BuildSpec{
		CommonSpec: buildv1.CommonSpec{
			ServiceAccount: constants.OCPBuilderServiceAccountName,
			Source:         sourceConfig,
			Strategy: buildv1.BuildStrategy{
				Type: buildv1.DockerBuildStrategyType,
				DockerStrategy: &buildv1.DockerBuildStrategy{
					Volumes: []buildv1.BuildVolume{
						{
							Name: "key",
							Source: buildv1.BuildVolumeSource{
								Type: buildv1.BuildVolumeSourceTypeSecret,
								Secret: &v1.SecretVolumeSource{
									SecretName: signConfig.KeySecret.Name,
									Optional:   ptr.To(false),
								},
							},
							Mounts: []buildv1.BuildVolumeMount{
								{DestinationPath: "/run/secrets/key"},
							},
						},
						{
							Name: "cert",
							Source: buildv1.BuildVolumeSource{
								Type: buildv1.BuildVolumeSourceTypeSecret,
								Secret: &v1.SecretVolumeSource{
									SecretName: signConfig.CertSecret.Name,
									Optional:   ptr.To(false),
								},
							},
							Mounts: []buildv1.BuildVolumeMount{
								{DestinationPath: "/run/secrets/cert"},
							},
						},
					},
				},
			},
			Output:         buildTarget,
			NodeSelector:   mld.Selector,
			MountTrustedCA: ptr.To(true),
			Tolerations:    mld.Tolerations,
		},
	}

	hash, err := m.hash(ctx, &spec, mld.Namespace, signConfig.KeySecret.Name, signConfig.CertSecret.Name)
	if err != nil {
		return nil, fmt.Errorf("could not hash Build spec: %v", err)
	}

	build := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mld.Name + "-sign-",
			Namespace:    mld.Namespace,
			Labels:       ocpbuildutils.GetOCPBuildLabels(mld, BuildType),
			Annotations:  ocpbuildutils.GetOCPBuildAnnotations(hash),
			Finalizers:   []string{constants.GCDelayFinalizer, constants.JobEventFinalizer},
		},
		Spec: spec,
	}

	if err := controllerutil.SetControllerReference(owner, build, m.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return build, nil
}

type hashData struct {
	BuildSpec      *buildv1.BuildSpec
	PrivateKeyData []byte
	PublicKeyData  []byte
}

func (m *maker) hash(ctx context.Context, buildSpec *buildv1.BuildSpec, namespace, keySecretName, certSecretName string) (uint64, error) {
	dataToHash := hashData{BuildSpec: buildSpec}

	var err error

	dataToHash.PublicKeyData, err = m.getSecretBytes(ctx, types.NamespacedName{Namespace: namespace, Name: certSecretName}, "cert.pem")
	if err != nil {
		return 0, fmt.Errorf("could not get cert bytes: %v", err)
	}

	dataToHash.PrivateKeyData, err = m.getSecretBytes(ctx, types.NamespacedName{Namespace: namespace, Name: keySecretName}, "key.pem")
	if err != nil {
		return 0, fmt.Errorf("could not get key bytes: %v", err)
	}

	hashValue, err := hashstructure.Hash(dataToHash, hashstructure.FormatV2, nil)
	if err != nil {
		return 0, fmt.Errorf("could not hash build spec and secrets: %v", err)
	}
	return hashValue, nil
}

func (m *maker) getSecretBytes(ctx context.Context, secretObjectKey types.NamespacedName, keyName string) ([]byte, error) {
	secret := v1.Secret{}

	if err := m.client.Get(ctx, secretObjectKey, &secret); err != nil {
		return nil, fmt.Errorf("error while getting secret %s: %v", secretObjectKey, err)
	}

	return secret.Data[keyName], nil
}
