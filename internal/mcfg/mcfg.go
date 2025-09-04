package mcfg

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"text/template"
)

const (
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

	ignitionTemplate = template.Must(
		template.ParseFS(templateFS, "templates/ignition.gotmpl"),
	)
)

//go:generate mockgen -source=mcfg.go -package=mcfg -destination=mock_mcfg.go

type MCFG interface {
	GenerateIgnition(kernelModuleImage, kernelModuleName, inTreeModuleToRemove, firmwareFilesPath, workerImage, servicePrefix string) ([]byte, string, error)
}

type mcfgImpl struct {
}

func NewMCFG() MCFG {
	return &mcfgImpl{}
}

func (m *mcfgImpl) GenerateIgnition(kernelModuleImage, kernelModuleName, inTreeModuleToRemove, firmwareFilesPath,
	workerImage, servicePrefix string) ([]byte, string, error) {
	templateParams := map[string]any{
		"FirmwareFilesPath":                firmwareFilesPath,
		"KernelModuleImage":                kernelModuleImage,
		"KernelModule":                     kernelModuleName,
		"KernelModuleImageFilepath":        kernelModuleImageFilepath,
		"InTreeModuleToRemove":             inTreeModuleToRemove,
		"WorkerImage":                      workerImage,
		"WorkerConfigFilepath":             workerConfigFilepath,
		"ServicePrefix":                    servicePrefix,
		"ReplaceInTreeDriverContents":      base64.StdEncoding.EncodeToString([]byte(scriptReplaceKmod)),
		"PullKernelModuleContents":         base64.StdEncoding.EncodeToString([]byte(scriptPullImage)),
		"WaitForNetworkDispatcherContents": base64.StdEncoding.EncodeToString([]byte(scriptWaitForNetworkDispatcher)),
	}

	var yamlIgnition bytes.Buffer

	if err := ignitionTemplate.Execute(&yamlIgnition, templateParams); err != nil {
		return nil, "", fmt.Errorf("could not render the ignition: %v", err)
	}

	var ignitionObj map[string]interface{}
	if err := yaml.Unmarshal(yamlIgnition.Bytes(), &ignitionObj); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal parsed ignition into yaml: %v", err)
	}

	jsonIgnition, err := json.Marshal(ignitionObj)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal yaml ignition into json: %v", err)
	}

	return jsonIgnition, yamlIgnition.String(), nil
}
