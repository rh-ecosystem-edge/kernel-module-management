package mcproducer

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/mcfg"
)

const (
	defaultWorkerImage = "quay.io/edge-infrastructure/kernel-module-management-worker:latest"
)

var (
	//go:embed templates
	templateFS embed.FS

	machineConfigTemplate = template.Must(
		template.New("machine-config.gotmpl").
			Funcs(sprig.TxtFuncMap()).
			ParseFS(templateFS, "templates/machine-config.gotmpl"),
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

	mcfgAPI := mcfg.NewMCFG()
	_, ignition, err := mcfgAPI.GenerateIgnition(kernelModuleImage, kernelModuleName, inTreeModuleToRemove, firmwareFilesPath, workerImageToUse, machineConfigName)
	if err != nil {
		return "", fmt.Errorf("failed to create ignition string: %v", err)
	}

	templateParams := map[string]any{
		"Ignition":             ignition,
		"MachineConfigPoolRef": machineConfigPoolRef,
		"MachineConfigName":    machineConfigName,
	}

	var machineConfig bytes.Buffer

	if err = machineConfigTemplate.Execute(&machineConfig, templateParams); err != nil {
		return "", fmt.Errorf("could not render the MachineConfig: %v", err)
	}

	return strings.TrimSpace(machineConfig.String()) + "\n", nil
}

func verifyKernelModuleImage(image string) error {
	_, err := name.ParseReference(image)
	if err != nil {
		return fmt.Errorf("image %s is in incorrect format: %v", image, err)
	}
	return nil
}
