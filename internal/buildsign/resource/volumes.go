package resource

import (
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func makeBuildResourceVolumes(buildConfig *kmmv1beta1.Build) []buildv1.BuildVolume {

	if buildConfig.Secrets == nil {
		return nil
	}

	volumes := make([]buildv1.BuildVolume, 0, len(buildConfig.Secrets))

	for _, s := range buildConfig.Secrets {
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
				{
					DestinationPath: "/run/secrets/" + s.Name,
				},
			},
		}

		volumes = append(volumes, bv)
	}

	return volumes
}

func makeSignResourceVolumes(signConfig *kmmv1beta1.Sign) []buildv1.BuildVolume {

	volumes := []buildv1.BuildVolume{
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
				{
					DestinationPath: "/run/secrets/key",
				},
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
				{
					DestinationPath: "/run/secrets/cert",
				},
			},
		},
	}

	return volumes
}
