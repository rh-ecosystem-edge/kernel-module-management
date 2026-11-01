package pod

import (
	"fmt"
	"path"
	"strings"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	inspectTerminationLog = "/dev/termination-log"

	extractModulesCommand = `if [ ! -f "$1/modules.dep" ]; then
    printf '%s' '{"error":"missing_modules.dep"}' > "$3"
    exit 1
fi
mkdir -p "$2"
cp -a -- "$1/." "$2/"
`
)

type InspectJobInput struct {
	KernelVersion string
	Image         string
}

//go:generate mockgen -source=inspectjob.go -package=pod -destination=mock_inspectjob.go

type InspectJobManager interface {
	// ConfigureJob fills a new Job, preserving its supplied name and namespace.
	ConfigureJob(job *batchv1.Job, irm *kmmv1beta1.InitramfsModule, input InspectJobInput) error
}

type inspectJobManager struct {
	client      client.Client
	scheme      *runtime.Scheme
	parserImage string
}

func NewInspectJobManager(cl client.Client, scheme *runtime.Scheme, parserImage string) InspectJobManager {
	return &inspectJobManager{client: cl, scheme: scheme, parserImage: parserImage}
}

func (m *inspectJobManager) ConfigureJob(job *batchv1.Job, irm *kmmv1beta1.InitramfsModule, input InspectJobInput) error {
	if input.KernelVersion == "" || input.Image == "" {
		return fmt.Errorf("inspect Job requires a kernel version and kmod image")
	}
	if err := ctrl.SetControllerReference(irm, job, m.scheme); err != nil {
		return fmt.Errorf("set inspect Job owner: %w", err)
	}

	kernel := strings.ReplaceAll(input.KernelVersion, "+", "-")

	dir := irm.Spec.DirName
	if dir == "" {
		dir = "/opt"
	}
	source := path.Join("/", dir, "lib", "modules", kernel)
	modulesRoot := path.Join(sharedFilesDir, source)
	pullSecrets := []v1.LocalObjectReference(nil)
	if irm.Spec.ImageRepoSecret != nil {
		pullSecrets = append(pullSecrets, *irm.Spec.ImageRepoSecret)
	}
	args := []string{"--modules-root", modulesRoot}
	for _, name := range irm.Spec.ModuleNames {
		args = append(args, "--modules-names", name)
	}
	for _, name := range irm.Spec.Omit {
		args = append(args, "--omit", name)
	}
	job.Spec = batchv1.JobSpec{
		BackoffLimit: ptr.To(int32(3)),
		Template: v1.PodTemplateSpec{
			Spec: v1.PodSpec{
				RestartPolicy:                v1.RestartPolicyNever,
				AutomountServiceAccountToken: ptr.To(false),
				ImagePullSecrets:             pullSecrets,
				Volumes: []v1.Volume{{Name: volNameTmp, VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{},
				}}},
				InitContainers: []v1.Container{{
					Name:                     "extract",
					Image:                    input.Image,
					ImagePullPolicy:          v1.PullAlways,
					Command:                  []string{"/bin/sh", "-ec", extractModulesCommand, "extract"},
					Args:                     []string{source, modulesRoot, inspectTerminationLog},
					VolumeMounts:             []v1.VolumeMount{{Name: volNameTmp, MountPath: sharedFilesDir}},
					TerminationMessagePath:   inspectTerminationLog,
					TerminationMessagePolicy: v1.TerminationMessageReadFile,
				}},
				Containers: []v1.Container{{
					Name:                     "parse",
					Image:                    m.parserImage,
					ImagePullPolicy:          v1.PullAlways,
					Command:                  []string{"/usr/local/bin/kmod-image-parse"},
					Args:                     args,
					VolumeMounts:             []v1.VolumeMount{{Name: volNameTmp, MountPath: sharedFilesDir, ReadOnly: true}},
					TerminationMessagePath:   inspectTerminationLog,
					TerminationMessagePolicy: v1.TerminationMessageReadFile,
				}},
			},
		},
	}
	return nil
}
