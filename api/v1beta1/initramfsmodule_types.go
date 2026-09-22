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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InitramfsModuleSpec describes out-of-tree kernel modules to place into initramfs.
type InitramfsModuleSpec struct {
	// Selector limits which nodes receive the generated initramfs.
	// Empty or omitted targets worker nodes.
	// +optional
	Selector map[string]string `json:"selector,omitempty"`

	// ModuleNames are the OOT kernel modules to extract and load
	// (.ko basename without .ko).
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	// +kubebuilder:validation:items:Pattern=`^[A-Za-z0-9_-]+$`
	ModuleNames []string `json:"moduleNames"`

	// Omit is the list of in-tree kernel module names passed to dracut omit_drivers.
	// +optional
	// +listType=set
	// +kubebuilder:validation:items:Pattern=`^[A-Za-z0-9_-]+$`
	Omit []string `json:"omit,omitempty"`

	// ContainerImage is the OOT kmod image without the kernel-version suffix:
	// <repository>:<partialTag>. The image pulled or built for a node is
	// <repository>:<partialTag>-<kernelVersion>.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ContainerImage string `json:"containerImage"`

	// FirmwarePath is the firmware directory inside the image.
	// +optional
	FirmwarePath string `json:"firmwarePath,omitempty"`

	// ModulesPath is the directory in the image that contains modules.* files.
	// +optional
	ModulesPath string `json:"modulesPath,omitempty"`

	// DirName is the in-image directory that contains the module tree.
	// The default is /opt
	// +optional
	DirName string `json:"dirName,omitempty"`

	// Build triggers an in-cluster build of ContainerImage when the tagged image is missing.
	// +optional
	Build *Build `json:"build,omitempty"`

	// Sign signs the modules using the same key/cert flow as Module.
	// +optional
	Sign *Sign `json:"sign,omitempty"`

	// ImageRepoSecret is an optional pull secret for ContainerImage.
	// +optional
	ImageRepoSecret *v1.LocalObjectReference `json:"imageRepoSecret,omitempty"`

	// Rollback, when true, reverts targeted nodes to the original in-tree initramfs.
	// +optional
	Rollback bool `json:"rollback,omitempty"`
}

type InitramfsNodeState string

const (
	// InitramfsPending means initramfs generation has not completed yet.
	InitramfsPending InitramfsNodeState = "Pending"
	// InitramfsCreated means the OOT initramfs was generated but is not yet applied.
	InitramfsCreated InitramfsNodeState = "Created"
	// InitramfsApplied means the node has rebooted into the OOT initramfs.
	InitramfsApplied InitramfsNodeState = "Applied"
	// InitramfsFailed means initramfs generation or apply failed.
	InitramfsFailed InitramfsNodeState = "Failed"
)

// InitramfsNodeStatus is the observed initramfs state for one node.
type InitramfsNodeStatus struct {
	Name string `json:"name"`

	// +optional
	KernelVersion string `json:"kernelVersion,omitempty"`

	// +kubebuilder:validation:Enum=Pending;Created;Applied;Failed
	State InitramfsNodeState `json:"state"`

	// Reason is set when State is Failed, for example MissingFirmware or BuildFailed.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// InitramfsModuleStatus defines the observed state of InitramfsModule.
type InitramfsModuleStatus struct {
	// +optional
	Nodes []InitramfsNodeStatus `json:"nodes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// InitramfsModule describes OOT kernel modules to include in initramfs.
// +kubebuilder:resource:path=initramfsmodules,scope=Namespaced,shortName=irm
// +operator-sdk:csv:customresourcedefinitions:displayName="Initramfs Module"
type InitramfsModule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InitramfsModuleSpec   `json:"spec,omitempty"`
	Status InitramfsModuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InitramfsModuleList contains a list of InitramfsModule.
type InitramfsModuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InitramfsModule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InitramfsModule{}, &InitramfsModuleList{})
}
