# Firmware support

Kernel modules sometimes need to load firmware files from the filesystem.
KMM supports copying firmware files from the [Module Loader image](module_loader_image.md)
to the node's filesystem.  
The contents of `.spec.moduleLoader.container.modprobe.firmwarePath` are copied into `/var/lib/firmware` on the node
before `modprobe` is called to insert the kernel module.  
All files and empty directories are removed from that location before `modprobe -r` is called to unload the kernel
module, when the pod is terminated.

## Configuring the lookup path on nodes

On OpenShift nodes, the set of default lookup paths for firmwares does not include `/var/lib/firmware`.
That path can be added with the [Machine Config Operator](https://docs.openshift.com/container-platform/4.12/post_installation_configuration/machine-configuration-tasks.html)
by creating a `MachineConfig` resource:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: 99-worker-kernel-args-firmware-path
spec:
  kernelArguments:
    - 'firmware_class.path=/var/lib/firmware'
```

This will entail a reboot of all worker nodes.

## Building a ModuleLoader image

In addition to building the kernel module itself, include the binary firmware in the builder image.

```dockerfile
FROM registry.redhat.io/ubi8/ubi-minimal as builder

# Build the kmod

RUN ["mkdir", "/firmware"]
RUN ["curl", "-o", "/firmware/firmware.bin", "https://artifacts.example.com/firmware.bin"]

FROM registry.redhat.io/ubi8/ubi-minimal

# Copy the kmod, install modprobe, run depmod

COPY --from=builder /firmware /firmware
```

## Tuning the `Module` resource

Set `.spec.moduleLoader.container.modprobe.firmwarePath` in the `Module` CR:

```yaml
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: my-kmod
spec:
  moduleLoader:
    container:
      modprobe:
        moduleName: my-kmod  # Required

        # Optional. Will copy /firmware/* into /var/lib/firmware/ on the node.
        firmwarePath: /firmware
        
        # Add kernel mappings
  selector:
    node-role.kubernetes.io/worker: ""
```