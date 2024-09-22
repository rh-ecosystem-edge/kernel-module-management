package mcproducer

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"text/template"

	"github.com/google/go-containerregistry/pkg/name"
)

const (
	defaultWorkerImage        = "quay.io/edge-infrastructure/kernel-module-management-worker:latest"
	kernelModuleImageFilepath = "/var/lib/image_file_day1.tar"
	workerConfigFilepath      = "/var/lib/kmm_day1_config.yaml"
)

var (
	//go:embed scripts/pull-image.sh
	scriptPullImage string

	//go:embed scripts/replace-kernel-module.sh
	scriptReplaceKmod string

	//go:embed scripts/wait-for-dispatcher.sh
	scriptWaitForNetworkDispatcher string

	//go:embed templates
	templateFS embed.FS

	machineConfigTemplate = template.Must(
		template.ParseFS(templateFS, "templates/machine-config.gotmpl"),
	)
)

func ProduceMachineConfig(machineConfigName,
	machineConfigPoolRef,
	kernelModuleImage,
	kernelModuleName,
	inTreeModuleToRemove,
	firmwareFilesPath,
	workerImage string) (string, error) {

	err := verifyKernelModuleImage(kernelModuleImage)
	if err != nil {
		return "", fmt.Errorf("failed to verify kernel module image name %s: %v", kernelModuleImage, err)
	}

	workerImageToUse := defaultWorkerImage
	if workerImage != "" {
		workerImageToUse = workerImage
	}

	templateParams := map[string]any{
		"FirmwareFilesPath":         firmwareFilesPath,
		"KernelModuleImage":         kernelModuleImage,
		"KernelModule":              kernelModuleName,
		"MachineConfigPoolRef":      machineConfigPoolRef,
		"MachineConfigName":         machineConfigName,
		"KernelModuleImageFilepath": kernelModuleImageFilepath,
		"InTreeModuleToRemove":      inTreeModuleToRemove,
		"WorkerImage":               workerImageToUse,
		"WorkerConfigFilepath":      workerConfigFilepath,
	}

	templateParams["ReplaceInTreeDriverContents"] = base64.StdEncoding.EncodeToString([]byte(scriptReplaceKmod))
	templateParams["PullKernelModuleContents"] = base64.StdEncoding.EncodeToString([]byte(scriptPullImage))
	templateParams["WaitForNetworkDispatcherContents"] = base64.StdEncoding.EncodeToString([]byte(scriptWaitForNetworkDispatcher))

	var machineConfig bytes.Buffer

	if err = machineConfigTemplate.Execute(&machineConfig, templateParams); err != nil {
		return "", fmt.Errorf("could not render the MachineConfig: %v", err)
	}

	return machineConfig.String(), nil
}

func verifyKernelModuleImage(image string) error {
	_, err := name.ParseReference(image)
	if err != nil {
		return fmt.Errorf("image %s is in incorrect format: %v", image, err)
	}
	return nil
}
