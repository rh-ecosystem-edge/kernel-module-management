package mcfg

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"text/template"

	"gopkg.in/yaml.v3"

	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	apioperatorv1 "github.com/openshift/api/operator/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	InitramfsPoolName          = "kmm-initramfs"
	InitramfsMachineConfigName = "99-kmm-initramfs"

	initramfsNodeRoleLabel             = "node-role.kubernetes.io/initramfs"
	machineConfigRoleLabel             = "machineconfiguration.openshift.io/role"
	InitramfsModuleNamespaceAnnotation = "kmm.sigs.x-k8s.io/initramfsmodule-namespace"
	InitramfsModuleNameAnnotation      = "kmm.sigs.x-k8s.io/initramfsmodule-name"
	InitramfsModuleUIDAnnotation       = "kmm.sigs.x-k8s.io/initramfsmodule-uid"

	kernelModuleImageFilepath = "/var/lib/image_file_day1.tar"
	workerConfigFilepath      = "/var/lib/kmm_day1_config.yaml"
	pullImageSystemdService   = "pull-kernel-module-image.service"
	replaceKmodSystemdService = "replace-kernel-module.service"
)

// InspectSoftLink is one pre-udev symlink for an initramfs kernel.
type InspectSoftLink struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

// KernelInspectLists is the in-tree module and symlink list for one kernel.
type KernelInspectLists struct {
	InTreeModules []string          `json:"inTreeModules"`
	SoftLinks     []InspectSoftLink `json:"softLinks"`
}

var (
	//go:embed scripts/pull-image.sh
	scriptPullImage string

	//go:embed scripts/replace-kernel-module.sh
	scriptReplaceKmod string

	//go:embed scripts/wait-for-dispatcher.sh
	scriptWaitForNetworkDispatcher string

	//go:embed scripts/kmm-initramfs.service
	scriptInitramfsService string

	//go:embed scripts/kmm-pre-udev.sh
	scriptPreUdev string

	//go:embed scripts/initramfs.sh
	scriptInitramfs string

	//go:embed templates
	templateFS embed.FS

	ignitionTemplate = template.Must(
		template.ParseFS(templateFS, "templates/ignition.gotmpl"),
	)

	initramfsServiceTemplate = template.Must(
		template.New("kmm-initramfs.service").Option("missingkey=error").Parse(scriptInitramfsService),
	)
	initramfsPreUdevTemplate = template.Must(
		template.New("kmm-pre-udev.sh").Option("missingkey=error").Parse(scriptPreUdev),
	)
	initramfsIgnitionTemplate = template.Must(
		template.New("initramfs-ignition.gotmpl").Option("missingkey=error").ParseFS(
			templateFS, "templates/initramfs-ignition.gotmpl",
		),
	)
)

//go:generate mockgen -source=mcfg.go -package=mcfg -destination=mock_mcfg.go

type MCFG interface {
	UpdateDisruptionPolicies(mc *apioperatorv1.MachineConfiguration, bmc *kmmv1beta1.BootModuleConfig)
	RemoveDisruptionPolicies(mc *apioperatorv1.MachineConfiguration, bmc *kmmv1beta1.BootModuleConfig, removeAll bool)
	UpdateMachineConfig(mc *mcfgv1.MachineConfig, bmc *kmmv1beta1.BootModuleConfig) error
	GenerateIgnition(kernelModuleImage, kernelModuleName, firmwareFilesPath, workerImage, servicePrefix string,
		inTreeModulesToRemove []string) ([]byte, string, error)
	StampedForInitramfsModule(meta metav1.Object, irm *kmmv1beta1.InitramfsModule) bool
	UpdateInitramfsPool(pool *mcfgv1.MachineConfigPool, irm *kmmv1beta1.InitramfsModule)
	UpdateInitramfsMachineConfig(mc *mcfgv1.MachineConfig, irm *kmmv1beta1.InitramfsModule, kernels map[string]KernelInspectLists) error
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

func (m *mcfgImpl) GenerateIgnition(kernelModuleImage, kernelModuleName, firmwareFilesPath,
	workerImage, servicePrefix string, inTreeModulesToRemove []string) ([]byte, string, error) {
	if workerImage == "" {
		workerImage = m.currentWorkerImage
	}
	templateParams := map[string]any{
		"FirmwareFilesPath":                 firmwareFilesPath,
		"KernelModuleImage":                 kernelModuleImage,
		"KernelModule":                      kernelModuleName,
		"KernelModuleImageFilepath":         kernelModuleImageFilepath,
		"InTreeModulesToRemove":             inTreeModulesToRemove,
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
	ignition, _, err := m.GenerateIgnition(bmc.Spec.KernelModuleImage, bmc.Spec.KernelModuleName,
		bmc.Spec.FirmwareFilesPath, bmc.Spec.WorkerImage, bmc.Spec.MachineConfigName, bmc.Spec.InTreeModulesToRemove)
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

func (m *mcfgImpl) StampedForInitramfsModule(meta metav1.Object, irm *kmmv1beta1.InitramfsModule) bool {
	ann := meta.GetAnnotations()
	return ann[InitramfsModuleNamespaceAnnotation] == irm.Namespace &&
		ann[InitramfsModuleNameAnnotation] == irm.Name
}

func (m *mcfgImpl) UpdateInitramfsPool(pool *mcfgv1.MachineConfigPool, irm *kmmv1beta1.InitramfsModule) {
	setInitramfsStamp(pool, irm)
	unavailable := intstr.FromInt(1)
	pool.Spec.NodeSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{initramfsNodeRoleLabel: ""},
	}
	pool.Spec.MachineConfigSelector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      machineConfigRoleLabel,
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{"worker", InitramfsPoolName},
		}},
	}
	pool.Spec.MaxUnavailable = &unavailable
}

func (m *mcfgImpl) UpdateInitramfsMachineConfig(mc *mcfgv1.MachineConfig, irm *kmmv1beta1.InitramfsModule, _ map[string]KernelInspectLists) error {
	raw, err := renderInitramfsIgnition(irm)
	if err != nil {
		return err
	}
	setInitramfsStamp(mc, irm)
	labels := mc.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[machineConfigRoleLabel] = InitramfsPoolName
	mc.SetLabels(labels)
	mc.Spec.Config.Raw = raw
	return nil
}

type initramfsScriptData struct {
	Rollback       bool
	ContainerImage string
	ModulesPath    string
	DirName        string
	SpecHash       string
	ModuleName     string
}

type initramfsIgnitionParams struct {
	UnitContents    string
	InitramfsScript string
	PreUdevScript   string
}

func renderInitramfsIgnition(irm *kmmv1beta1.InitramfsModule) ([]byte, error) {
	moduleName, err := singleInitramfsModuleName(irm.Spec.ModuleNames)
	if err != nil {
		return nil, err
	}
	data := initramfsScriptData{
		Rollback:       irm.Spec.Rollback,
		ContainerImage: irm.Spec.ContainerImage,
		ModulesPath:    irm.Spec.ModulesPath,
		DirName:        irm.Spec.DirName,
		ModuleName:     moduleName,
	}
	data.SpecHash, err = initramfsSpecHash(data)
	if err != nil {
		return nil, err
	}

	var serviceBuf, hookBuf bytes.Buffer
	if err = initramfsServiceTemplate.Execute(&serviceBuf, data); err != nil {
		return nil, fmt.Errorf("failed to render kmm-initramfs.service: %v", err)
	}
	if err = initramfsPreUdevTemplate.Execute(&hookBuf, data); err != nil {
		return nil, fmt.Errorf("failed to render kmm-pre-udev.sh: %v", err)
	}

	var yamlIgnition bytes.Buffer
	if err = initramfsIgnitionTemplate.Execute(&yamlIgnition, initramfsIgnitionParams{
		UnitContents:    serviceBuf.String(),
		InitramfsScript: base64.StdEncoding.EncodeToString([]byte(scriptInitramfs)),
		PreUdevScript:   base64.StdEncoding.EncodeToString(hookBuf.Bytes()),
	}); err != nil {
		return nil, fmt.Errorf("failed to render initramfs ignition: %v", err)
	}

	var ignitionObj map[string]interface{}
	if err = yaml.Unmarshal(yamlIgnition.Bytes(), &ignitionObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal initramfs ignition: %v", err)
	}
	raw, err := json.Marshal(ignitionObj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal initramfs ignition: %v", err)
	}
	return raw, nil
}

func singleInitramfsModuleName(names []string) (string, error) {
	if len(names) != 1 {
		return "", fmt.Errorf("initramfs MachineConfig requires exactly one moduleNames entry, got %d", len(names))
	}
	return names[0], nil
}

func initramfsSpecHash(data initramfsScriptData) (string, error) {
	raw, err := json.Marshal(struct {
		Rollback       bool   `json:"rollback"`
		ContainerImage string `json:"containerImage"`
		ModulesPath    string `json:"modulesPath"`
		DirName        string `json:"dirName"`
		ModuleName     string `json:"moduleName"`
	}{
		Rollback:       data.Rollback,
		ContainerImage: data.ContainerImage,
		ModulesPath:    data.ModulesPath,
		DirName:        data.DirName,
		ModuleName:     data.ModuleName,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal initramfs spec hash: %v", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func setInitramfsStamp(obj metav1.Object, irm *kmmv1beta1.InitramfsModule) {
	ann := obj.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann[InitramfsModuleNamespaceAnnotation] = irm.Namespace
	ann[InitramfsModuleNameAnnotation] = irm.Name
	obj.SetAnnotations(ann)
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
