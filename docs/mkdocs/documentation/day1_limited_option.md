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

## In-tree module replacement

The Day 1 functionality always tries to replace the in-tree kernel module with the OOT one.
If the in-tree kernel module is not loaded, the flow is not affected;, the service will proceed and load the OOT kernel module.

## MCO yaml creation

KMM provides an API to create an MCO YAML manifest for the Day 1 functionality:

```go
ProduceMachineConfig(machineConfigName, machineConfigPoolRef, kernelModuleImage, kernelModuleName string) (string, error)
```

The returned output is a string representation of the MCO YAML manifest to be applied.
It is up to the customer to apply this YAML.

The parameters are:

- `machineConfigName`: the name of the MCO YAML manifest. It will be set as the `name` parameter of the metadata of MCO YAML manifest.
- `machineConfigPoolRef`: the `MachineConfigPool` name that will be used in order to identify the targeted nodes
- `kernelModuleImage`: the name of the container image that includes the OOT kernel module.
- `kernelModuleName`: the name of the OOT kernel module. This parameter will be used both to unload the in-tree kernel module
   (if loaded into the kernel) and to load the OOT kernel module.

The API is located under `pkg/mcproducer` package of hte KMM source code.
There is no need to KMM operator to be running to use the Day 1 functionality.
Users only need to import the `pkg/mcproducer` package into their operator/utility code, call the API and to apply the produced
MCO YAML to the cluster.
