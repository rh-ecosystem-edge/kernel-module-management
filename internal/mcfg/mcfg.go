package mcfg

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"text/template"

	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	apioperatorv1 "github.com/openshift/api/operator/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
)

const (
	kernelModuleImageFilepath = "/var/lib/image_file_day1.tar"
	workerConfigFilepath      = "/var/lib/kmm_day1_config.yaml"
	pullImageSystemdService   = "pull-kernel-module-image.service"
	replaceKmodSystemdService = "replace-kernel-module.service"
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
	UpdateDisruptionPolicies(mc *apioperatorv1.MachineConfiguration, bmc *kmmv1beta1.BootModuleConfig)
	RemoveDisruptionPolicies(mc *apioperatorv1.MachineConfiguration, bmc *kmmv1beta1.BootModuleConfig, removeAll bool)
	UpdateMachineConfig(mc *mcfgv1.MachineConfig, bmc *kmmv1beta1.BootModuleConfig) error
	GenerateIgnition(kernelModuleImage, kernelModuleName, inTreeModuleToRemove, firmwareFilesPath, workerImage, servicePrefix string) ([]byte, string, error)
}

type mcfgImpl struct {
	currentWorkerImage string
}

func NewMCFG(currentWorkerImage string) MCFG {
	return &mcfgImpl{
		currentWorkerImage: currentWorkerImage,
	}
}

func (m *mcfgImpl) UpdateDisruptionPolicies(mc *apioperatorv1.MachineConfiguration, bmc *kmmv1beta1.BootModuleConfig) {
	addSystemdToDisruptionPolicies(mc, bmc.Spec.MachineConfigName+"-"+pullImageSystemdService)
	addSystemdToDisruptionPolicies(mc, bmc.Spec.MachineConfigName+"-"+replaceKmodSystemdService)
	addSystemdToDisruptionPolicies(mc, "crio-wipe.service")
	addFileToDisruptionPolicies(mc, "/usr/local/bin/replace-kernel-module.sh")
	addFileToDisruptionPolicies(mc, "/usr/local/bin/pull-kernel-module-image.sh")
	addFileToDisruptionPolicies(mc, "/usr/local/bin/wait-for-dispatcher.sh")
}

func (m *mcfgImpl) RemoveDisruptionPolicies(mc *apioperatorv1.MachineConfiguration, bmc *kmmv1beta1.BootModuleConfig, removeAll bool) {
	removeSystemdFromDisruptionPolicies(mc, bmc.Spec.MachineConfigName+"-"+pullImageSystemdService)
	removeSystemdFromDisruptionPolicies(mc, bmc.Spec.MachineConfigName+"-"+replaceKmodSystemdService)
	if removeAll {
		removeSystemdFromDisruptionPolicies(mc, "crio-wipe.service")
		removeFileFromDisruptionPolicies(mc, "/usr/local/bin/replace-kernel-module.sh")
		removeFileFromDisruptionPolicies(mc, "/usr/local/bin/pull-kernel-module-image.sh")
		removeFileFromDisruptionPolicies(mc, "/usr/local/bin/wait-for-dispatcher.sh")
	}
}

func (m *mcfgImpl) UpdateMachineConfig(mc *mcfgv1.MachineConfig, bmc *kmmv1beta1.BootModuleConfig) error {
	updateMachineConfigLabels(mc, bmc)
	return m.updateMachineConfigIgnition(mc, bmc)
}

func (m *mcfgImpl) GenerateIgnition(kernelModuleImage, kernelModuleName, inTreeModuleToRemove, firmwareFilesPath,
	workerImage, servicePrefix string) ([]byte, string, error) {
	if workerImage == "" {
		workerImage = m.currentWorkerImage
	}
	templateParams := map[string]any{
		"FirmwareFilesPath":                 firmwareFilesPath,
		"KernelModuleImage":                 kernelModuleImage,
		"KernelModule":                      kernelModuleName,
		"KernelModuleImageFilepath":         kernelModuleImageFilepath,
		"InTreeModuleToRemove":              inTreeModuleToRemove,
		"WorkerImage":                       workerImage,
		"WorkerConfigFilepath":              workerConfigFilepath,
		"ServicePrefix":                     servicePrefix,
		"PullKernelModuleSystemdService":    pullImageSystemdService,
		"ReplaceKernelModuleSystemdService": replaceKmodSystemdService,
		"ReplaceInTreeDriverContents":       base64.StdEncoding.EncodeToString([]byte(scriptReplaceKmod)),
		"PullKernelModuleContents":          base64.StdEncoding.EncodeToString([]byte(scriptPullImage)),
		"WaitForNetworkDispatcherContents":  base64.StdEncoding.EncodeToString([]byte(scriptWaitForNetworkDispatcher)),
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

func (m *mcfgImpl) updateMachineConfigIgnition(mc *mcfgv1.MachineConfig, bmc *kmmv1beta1.BootModuleConfig) error {
	ignition, _, err := m.GenerateIgnition(bmc.Spec.KernelModuleImage, bmc.Spec.KernelModuleName, bmc.Spec.InTreeModuleToRemove,
		bmc.Spec.FirmwareFilesPath, bmc.Spec.WorkerImage, bmc.Spec.MachineConfigName)
	if err != nil {
		return fmt.Errorf("failed to update runtime BMC object %s: %v", bmc.Name, err)
	}
	mc.Spec.Config.Raw = ignition
	return nil
}

func addSystemdToDisruptionPolicies(mc *apioperatorv1.MachineConfiguration, systemdName string) {
	dpSystemdName := apioperatorv1.NodeDisruptionPolicyServiceName(systemdName)
	for _, unit := range mc.Spec.NodeDisruptionPolicy.Units {
		if unit.Name == dpSystemdName {
			return
		}
	}
	dpUnit := apioperatorv1.NodeDisruptionPolicySpecUnit{
		Name: dpSystemdName,
		Actions: []apioperatorv1.NodeDisruptionPolicySpecAction{
			{
				Type: apioperatorv1.NoneSpecAction,
			},
		},
	}
	mc.Spec.NodeDisruptionPolicy.Units = append(mc.Spec.NodeDisruptionPolicy.Units, dpUnit)
}

func removeSystemdFromDisruptionPolicies(mc *apioperatorv1.MachineConfiguration, systemdName string) {
	dpSystemdName := apioperatorv1.NodeDisruptionPolicyServiceName(systemdName)
	for i, unit := range mc.Spec.NodeDisruptionPolicy.Units {
		if unit.Name == dpSystemdName {
			mc.Spec.NodeDisruptionPolicy.Units = append(mc.Spec.NodeDisruptionPolicy.Units[:i], mc.Spec.NodeDisruptionPolicy.Units[i+1:]...)
			return
		}
	}
}

func addFileToDisruptionPolicies(mc *apioperatorv1.MachineConfiguration, filePath string) {
	for _, file := range mc.Spec.NodeDisruptionPolicy.Files {
		if file.Path == filePath {
			return
		}
	}

	dpFile := apioperatorv1.NodeDisruptionPolicySpecFile{
		Path: filePath,
		Actions: []apioperatorv1.NodeDisruptionPolicySpecAction{
			{
				Type: apioperatorv1.NoneSpecAction,
			},
		},
	}
	mc.Spec.NodeDisruptionPolicy.Files = append(mc.Spec.NodeDisruptionPolicy.Files, dpFile)
}

func removeFileFromDisruptionPolicies(mc *apioperatorv1.MachineConfiguration, filePath string) {
	for i, file := range mc.Spec.NodeDisruptionPolicy.Files {
		if file.Path == filePath {
			mc.Spec.NodeDisruptionPolicy.Files = append(mc.Spec.NodeDisruptionPolicy.Files[:i], mc.Spec.NodeDisruptionPolicy.Files[i+1:]...)
			return
		}
	}
}

func updateMachineConfigLabels(mc *mcfgv1.MachineConfig, bmc *kmmv1beta1.BootModuleConfig) {
	if bmc.Spec.MachineConfigPoolName != "" {
		if mc.Labels != nil {
			mc.Labels["machineconfiguration.openshift.io/role"] = bmc.Spec.MachineConfigPoolName
		} else {
			mc.Labels = map[string]string{"machineconfiguration.openshift.io/role": bmc.Spec.MachineConfigPoolName}
		}
	}
}
