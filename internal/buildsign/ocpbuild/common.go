package ocpbuild

import (
	"context"
	"embed"
	"fmt"
	"html/template"

	"github.com/mitchellh/hashstructure/v2"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	v1 "k8s.io/api/core/v1"
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

func buildVolumesFromBuildSecrets(secrets []v1.LocalObjectReference) []buildv1.BuildVolume {
	if secrets == nil {
		return nil
	}

	vols := make([]buildv1.BuildVolume, 0, len(secrets))

	for _, s := range secrets {
		bv := buildv1.BuildVolume{
			Name: "secret-" + s.Name,
			Source: buildv1.BuildVolumeSource{
				Type: buildv1.BuildVolumeSourceTypeSecret,
				Secret: &v1.SecretVolumeSource{
					SecretName: s.Name,
					Optional:   ptr.To(false),
				},
			},
			Mounts: []buildv1.BuildVolumeMount{
				{DestinationPath: "/run/secrets/" + s.Name},
			},
		}

		vols = append(vols, bv)
	}

	return vols
}

func (omi *ocpbuildManagerImpl) hash(ctx context.Context, buildSpec *buildv1.BuildSpec, namespace, keySecretName,
	certSecretName string) (uint64, error) {

	dataToHash := struct {
		BuildSpec      *buildv1.BuildSpec
		PrivateKeyData []byte
		PublicKeyData  []byte
	}{
		BuildSpec: buildSpec,
	}

	var err error

	dataToHash.PublicKeyData, err = omi.getSecretBytes(ctx, types.NamespacedName{Namespace: namespace, Name: certSecretName}, "cert.pem")
	if err != nil {
		return 0, fmt.Errorf("could not get cert bytes: %v", err)
	}

	dataToHash.PrivateKeyData, err = omi.getSecretBytes(ctx, types.NamespacedName{Namespace: namespace, Name: keySecretName}, "key.pem")
	if err != nil {
		return 0, fmt.Errorf("could not get key bytes: %v", err)
	}

	hashValue, err := hashstructure.Hash(dataToHash, hashstructure.FormatV2, nil)
	if err != nil {
		return 0, fmt.Errorf("could not hash build spec and secrets: %v", err)
	}
	return hashValue, nil
}

func (omi *ocpbuildManagerImpl) getSecretBytes(ctx context.Context, secretObjectKey types.NamespacedName,
	keyName string) ([]byte, error) {

	secret := v1.Secret{}

	if err := omi.client.Get(ctx, secretObjectKey, &secret); err != nil {
		return nil, fmt.Errorf("error while getting secret %s: %v", secretObjectKey, err)
	}

	return secret.Data[keyName], nil
}
