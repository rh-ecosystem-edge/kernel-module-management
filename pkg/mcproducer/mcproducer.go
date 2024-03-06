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
	defaultWorkerImage = "quay.io/edge-infrastructure/kernel-module-management-worker:latest"
)

var (
	//go:embed templates
	templateFS embed.FS

	machineConfigTemplate = template.Must(
		template.ParseFS(templateFS, "templates/machine-config.gotmpl"),
	)

	scriptPullImage = template.Must(
		template.ParseFS(templateFS, "templates/pull-image.gotmpl"),
	)

	scriptReplaceKmod = template.Must(
		template.ParseFS(templateFS, "templates/replace-kernel-module.gotmpl"),
	)

	workerConfigMap = template.Must(
		template.ParseFS(templateFS, "templates/worker-configmap.gotmpl"),
	)
)

func ProduceMachineConfig(machineConfigName,
	machineConfigPoolRef,
	kernelModuleImage,
	kernelModuleName,
	inTreeModuleToRemove,
	workerImage string) (string, error) {
	localFilePath, err := getLocalFileName(kernelModuleImage)
	if err != nil {
		return "", fmt.Errorf("failed to get local file name for image %s: %v", kernelModuleImage, err)
	}

	workerImageToUse := defaultWorkerImage
	if workerImage != "" {
		workerImageToUse = workerImage
	}

	templateParams := map[string]any{
		"Image":                kernelModuleImage,
		"KernelModule":         kernelModuleName,
		"MachineConfigPoolRef": machineConfigPoolRef,
		"MachineConfigName":    machineConfigName,
		"LocalFilePath":        localFilePath,
		"InTreeModuleToRemove": inTreeModuleToRemove,
		"WorkerImage":          workerImageToUse,
	}

	templateParams["ReplaceInTreeDriverContents"], err = executeIntoBase64(scriptReplaceKmod, templateParams)
	if err != nil {
		return "", err
	}

	templateParams["PullKernelModuleContents"], err = executeIntoBase64(scriptPullImage, templateParams)
	if err != nil {
		return "", err
	}

	templateParams["WorkerPodConfigContents"], err = executeIntoBase64(workerConfigMap, templateParams)
	if err != nil {
		return "", err
	}

	var machineConfig bytes.Buffer

	if err = machineConfigTemplate.Execute(&machineConfig, templateParams); err != nil {
		return "", fmt.Errorf("could not render the MachineConfig: %v", err)
	}

	return machineConfig.String(), nil
}

func executeIntoBase64(tmpl *template.Template, params map[string]any) (string, error) {
	var buf bytes.Buffer

	enc := base64.NewEncoder(base64.StdEncoding, &buf)

	if err := tmpl.Execute(enc, params); err != nil {
		return "", fmt.Errorf("could not render %s: %v", tmpl.Name(), err)
	}

	if err := enc.Close(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func getLocalFileName(containerImage string) (string, error) {
	_, err := name.ParseReference(containerImage)
	if err != nil {
		return "", fmt.Errorf("failed to parse container image %s name: %v", containerImage, err)
	}

	return "/var/lib/image_file_day1.tar", nil
}
