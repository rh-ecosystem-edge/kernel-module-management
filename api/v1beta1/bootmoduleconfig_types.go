/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BootModuleConfigSpec struct {
	// machine config that is targeted by the BMC
	MachineConfigName string `json:"machineConfigName"`

	// the machine config pool that is linked to the targeted machine config
	MachineConfigPoolName string `json:"machineConfigPoolName"`

	// kernel module container image that contains the kernel module .ko file
	KernelModuleImage string `json:"kernelModuleImage"`

	// the name of the kernel module to be loaded(the name of the .ko file without the .ko)
	KernelModuleName string `json:"kernelModuleName"`

	//+optional
	// the in-tree kernel module list to remove prior to loading the OOT kernel module
	InTreeModulesToRemove []string `json:"inTreeModulesToRemove,omitempty"`

	//+optional
	// path of the firmware files in the kernel module container image
	FirmwareFilesPath string `json:"firmwareFilesPath,omitempty"`

	//+optional
	// KMM worker image. if missing, the current worker image will be used
	WorkerImage string `json:"workerImage,omitempty"`
}

type BootModuleConfigStatus struct {
	//+optional
	ConfigStatus string `json:"configStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// BootModuleConfig describes how to load a module during kernel initialization
// +kubebuilder:resource:path=bootmoduleconfigs,scope=Namespaced,shortName=bmc
// +operator-sdk:csv:customresourcedefinitions:displayName="Boot Module Config"
type BootModuleConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BootModuleConfigSpec   `json:"spec,omitempty"`
	Status BootModuleConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BootModuleConfigList is a list of BootModule objects.
type BootModuleConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of BootModuleConfig. More info:
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md
	Items []BootModuleConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BootModuleConfig{}, &BootModuleConfigList{})
}
