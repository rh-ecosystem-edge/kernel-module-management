# Day 1 kernel module loading support

KMM is a Day 2 operator: kernel modules are loaded only after the complete initialization of a Linux (RCHOS) server.
Sometimes though, kernel module needs to be loaded at a much earlier stage.
KMM's Day 1 functionality allows customer to load kernel module during Linux systemd initialization stage, using the [Machine Config Operator (MCO)](https://docs.openshift.com/container-platform/4.13/post_installation_configuration/machine-configuration-tasks.html).

## Supported use-cases

KMM's Day 1 functionality supports only a limited number of use cases. Its main use case is to allow loading out-of-tree kernel modules
prior to NetworkManager service initialization. It does not support loading kernel module at initramfs stage.
The following are the conditions needed by Day 1 functionality:

1. kernel module is currently not loaded in the kernel
2. the in-tree kernel module is loaded into the kernel, but can be unloaded and replaced by the OOT kernel module.
   This means that the in-tree module is not referenced by any other kernel modules
3. in order for the Day 1 functionlity to work, the node must have an functional network interface, meaning: an in-tree kernel driver for that interface.
   The OOT kernel module can be a network driver that will replace the functional network driver.

## Day 1 OOT kernel module loading flow

The loading of OOT kernel module leverages MCO. The flow sequence is as follows:

1. Apply an MCO YAML manifest to the existing running cluster. In order to identify the necessary nodes that need to be
   updated, you must create an appropriate `MachineConfigPool` resource.
2. MCO applies reboots node by node. On any rebooted node, 2 new systemd services are deployed: pull service and load service
3. Load service is configured to run prior to NetworkConfiguration service. It tries to pull a predefined kernel module image
   and then, using that image, to unload an in-tree module and load an OOT kernel module. 
4. Pull service is configured to run after NetworkManager service. It checks if the pre-configured kernel module image is located
   on the node's filesystem. If it is - then the service exists normally, and server continues with the boot process.
   If not - it pulls the image onto the node, and reboots the node afterwards.

## Kernel Module Image

The Day 1 functionality uses the same DTK based image that Day 2 KMM builds can leverage.
OOT kernel module should be located under `/opt/lib/modules/${kernelVersion}`.
The tag of the kernel module image should be equal to kernel version on the node: for example,
if the kernel version on the node is `5.14.0-284.59.1.el9_2.x86_64`, then the image and tag should be:
`repo/image:5.14.0-284.59.1.el9_2.x86_64`

## In-tree module replacement

The Day 1 functionality will try to replace in-tree kernel module only if requested (see parameter to the MC creation).
If the in-tree kernel module is not loaded, but was requested to be unloaded,  the flow is not affected;
the service will proceed and load the OOT kernel module.

## MCO yaml creation

KMM provides 2 ways to create an MCO YAML manifest for the Day 1 functionality:
1. API to be called by from GO code
2. Linux executable that can be called manually with appropriate parameters 

### API

```go
ProduceMachineConfig(machineConfigName, machineConfigPoolRef, kernelModuleImage, kernelModuleName, inTreeModulesToRemove,
    firmwareFilesPath, workerImage string) (string, error)
```

The returned output is a string representation of the MCO YAML manifest to be applied.
It is up to the customer to apply this YAML.

The parameters are:

- `machineConfigName`: the name of the MCO YAML manifest. It will be set as the `name` parameter of the metadata of MCO YAML manifest.
- `machineConfigPoolRef`: the `MachineConfigPool` name that will be used in order to identify the targeted nodes
- `kernelModuleImage`: the name of the container image that includes the OOT kernel module without the tag
- `kernelModuleName`: the name of the OOT kernel module. This parameter will be used both to unload the in-tree kernel module
   (if loaded into the kernel) and to load the OOT kernel module.
- `inTreeModulesToRemove`: optional parameter. A list of in-tree kernel modules to unload prior to loading OOT kernel module.
    In case this parameter is not passed, day1 functionality will not try to unload any in-tree modules.
- `workerImage`: optional parameter. The worker image to use. In case this parameter is not passed, the default worker image 
    will be used: quay.io/edge-infrastructure/kernel-module-management-worker:latest.
- `firmwareFilesPath`:` optional parameter. In case there is a need to also use firmware,
    this parameter should hold the path to the directory containing those files as a string format.
                                       

The API is located under `pkg/mcproducer` package of the KMM source code.
There is no need to KMM operator to be running to use the Day 1 functionality.
Users only need to import the `pkg/mcproducer` package into their operator/utility code, call the API and to apply the produced
MCO YAML to the cluster.

### Utility
`day1-utility` can be called from a shell. day1-utility executable is not a part of KMM GitHub repo.
In order to build it the following commands needs to be run:
`make day1-utility`

Utility uses the following flags:
- `machine-config <string>`: name of the machine config to create.
- `machine-config-pool <string>`: name of the machine config pool to use.
- `image <string>`: container image that contains kernel module .ko file.
- `kernel-module <string>`: name of the OOT module to load.
- `in-tree-modules-to-remove <string>`: in-tree kernel modules (comma-separated) that should be removed prior to loading the oot module.
- `worker-image <string>`: kernel-management worker image to use. If not passed, a default value will be used.
- `firmware-files-path <string>`: path to the firmware files inside the module image.

The first 4 flags are mandatory, but the last 2 are optional. They correspond to the parameters of the API

### MachineConfigPool

MachineConfigPool is used to identify a collection of nodes that will be affected by the applied MCO.

```yaml
kind: MachineConfigPool
metadata:
  name: sfc
spec:
  machineConfigSelector:
    matchExpressions:
      - {key: machineconfiguration.openshift.io/role, operator: In, values: [worker, sfc]}
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/sfc: ""
  paused: false
  maxUnavailable: 1
```

`machineConfigSelector` will match the labels in the MachineConfig, while `nodeSelector` will match the labels
on the node.

There are already predefined MachineConfigPools in the OCP cluster:

- `worker`: targets all worker nodes in the cluster
- `master`: targets all master nodes in the cluster

Defining a MachineConfig that has:
```yaml
metadata:
  labels:
    machineconfiguration.opensfhit.io/role: master
```
will target the master MachineConfigPool, while defining MachineConfig:
```yaml
metadata:
  labels:
    machineconfiguration.opensfhit.io/role: worker
```
will target the worker MachineConfigPool

A detailed description of MachineConfig and MachineConfigPool can be found in [MachineConfigPool explanation](https://www.redhat.com/en/blog/openshift-container-platform-4-how-does-machine-config-pool-work) for more information.

## Cluster Upgrade support
Using kernel version as a tag for kernel module image, allows supporting cluster upgrade. Pull service will determine the kernel version of the 
node and then use this value as a tag for kernel module image. This way, all the customer needs to do prior to upgrading the cluster, is to create a kernel module image
with the appropriate tag, without any need to update day1 MC. Once the node is rebooted, pull service will pull the correct image

## kmod Upgrade support
Kmod installed using the day1-utility can be managed by the KMM operator for full lifecycle management.

1. A kmod was installed using the day-utility and a `MachineConfig` is present in the cluster.
2. One can create a `Module` in the cluster targetting the same kmod and kernel as the `MachineConfig` did.
3. The KMM operator will try to load the kmod - nothing will happen since it is already loaded in the kernel.
4. From now on, upgrades can be done like day2 operations by updating the `Module` CR in the cluster.

The main caveat of this approach is that upon a sudden node reboot, the node will be rebooted with the kmod from the `MachineConfig` and not the one from the `Module` in case a kmod upgrade was performed.

To overcome this issue, we have introduced the `BootMachineConfig` (BMC) CRD.
When a day1 kmod was "transitioned" to the KMM operator using a `Module`, a BMC will also need to be created in the cluster to address the sudden reboot issues by ensuring that the `MachineConfig` will be updated will the correct values without triggering a node reboot.

### BootMachineConfig CRD
```yaml
 apiVersion: kmm.sigs.x-k8s.io/v1beta1
 kind: BootModuleConfig
 metadata:
  name: example-bmc
  namespace: openshift-machine-config-operator
 spec:
  machineConfigName: worker-kmod-config
  machineConfigPoolName: worker
  kernelModuleImage: quay.io/example/kmod
  kernelModuleName: my_module
  inTreeModulesToRemove:
    - intree_module_1
    - intree_module_2
 status:
  conditions: []
```

* `machineConfigName`: the machineConfig that is targeted by the BMC
* `machineConfigPoolName`: the machineConfig pool that is linked to the targeted machineConfig
* `kernelModuleImage`: kernel module container image that contains the kernel module .ko file without the tag
The pull service will determine the kernel version of the node and then use this value as a tag for kernel module image. This way, all the customer needs to do prior to upgrading the cluster, is to create a kernel module image with the appropriate tag, without any need to update day1 MC. Once the node is rebooted, pull service will pull the correct image.
* `kernelModuleName`: the name of the kernel module to be loaded (the name of the .ko file without the .ko)
* `inTreeModulesToRemove`: Optional; a list of the in-tree kernel module to remove prior to loading the OOT kernel module
* `firmwareFilesPath`: Optional; path of the firmware files in the kernel module container image
* `workerImage`: Optional; KMM worker image. if missing, the current worker image will be used
