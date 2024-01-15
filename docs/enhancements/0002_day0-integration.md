# Day-0 integration with KMM

### Motivation

KMM is managing mostly day2 kmods in OCP. It also offers a library for generating
systemd files for helping customers to deploy their day-1 kmod.

What KMM doesn't offer today to customers is the deployment and management of
day-0 kmods.

Day-0 kmod are complex as they need to be present in the system at boot time,
even before OCP is fully installed and certainly before KMM is deployed to the
cluster, therefore, there is not much that KMM can do to help deploy those kmods
but it can manage their lifecycle once a customer has installed a cluster with
his day-0 kmods in it - this is what this enhancement is about.

### Day-0 cluster background

A customer wishing to deploy OCP with custom nodes will need to generate 2
artifacts using [image-builder](https://www.redhat.com/en/topics/linux/what-is-an-image-builder)
  * A custom ISO for the nodes
  * A container image encapsulating the ostree-commit that is deployed in the ISO

The ISO will be used by [assisted-installer](https://docs.openshift.com/container-platform/4.14/installing/installing_on_prem_assisted/installing-on-prem-assisted.html)
to install the cluster and the container image will be used to notify
[MCO](https://docs.openshift.com/container-platform/4.14/post_installation_configuration/machine-configuration-tasks.html#understanding-the-machine-config-operator)
that the image on the nodes is a custom image.

When a customer has installed a "day-0 cluster" as mentioned above, we can
assume that there is a `MachineConfig` in the cluster that looks something like
```
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: 99-ybettan-external-image
spec:
  osImageURL: quay.io/ybettan/rhcos:414.92.202312311229-0
```
The important piece in it is `osImageURL: quay.io/ybettan/rhcos:414.92.202312311229-0`
which tells MCO that the ostree-commit that is supposed to be running on
the node is encapsulated in `quay.io/ybettan/rhcos:414.92.202312311229-0`.

If MCO finds out that the OS on the node is different than the image specified
in `osImageURL` then it will override the node OS based on the `MachineConfig`
and reboot - we are going to leverage this mechanism for the integration with
KMM.

In a day-0-cluster scenario, the user has applied this `MachineConfig` to tell
MCO he has modified the OS on the nodes and that it shouldn't override it with
the RHCOS image from the payload.

### Setting up the integration

A user that wishes to hand over the management of his day-0 kmod to KMM will
first need to make sure the `MachineConfig` can be recognize by KMM, therefore,
he would add a `machineconfiguration.openshift.io/kmm-managed: "true"` label to
it.

KMM will watch for `MachineConfig`s in the cluster containing the
`machineconfiguration.openshift.io/kmm-managed: "true"` label and create a new
`ModuleDay0` CR in the cluster for each one of them.
We will have a different controller for this new CRD in KMM.
**There should be no more than 1 `ModuleDay0` per `MachineConfig`.
If multiple kmods are required they should all be baked in the same image and
used in the same `MachineConfig` by the user**

After the `ModuleDay0` was create by KMM, all KMM's features will be used by the
official KMM's API which is the `ModuleDay0` objects in the cluster.

In case a user will wish to stop the sync between MCO and KMM, he will need to
remove the `machineconfiguration.openshift.io/kmm-managed: "true"` label from
the `MachineConfig`; KMM will then delete the corresponding `ModuleDay0`.
Once done, if the user delete the `MachineConfig` in the cluster then MCO will
roll-back to the RHCOS image from the payload (without the kmods).

### The `ModuleDay0` CRD

```
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: ModuleDay0
spec:
  machineConfigName: 99-ybettan-external-image
  containerImage: quay.io/ybettan/rhcos:414.92.202312311229-0
```

* The `containerImage` here must be a full RHCOS image built by correct tools
  or a container image based on an RHCOS container image and not an image based
  on UBI.

When KMM create the `ModuleDay0` upon discovering a new `MachineConfig`, it will
need to get each piece of information from somewhere
  * `machineConfigName`: will be taken from the `MachineConfig` that its discover has led to the creation of the `ModuleDay0`
  * `containerImage`: will be taken from `spec.osImageURL` in the `MachineConfig`

### Modifying day-0 kmods

In day2, we have `kernelMappings` which defines what image should be use for
each kernel (or future kernel) in the cluster. Once KMM notice a new kernel on
some nodes, it will start acting on the relevant `kernelMapping`, for example,
it will load the new kmod on the node.

In day0 modules, we will take a pro-active approach - we will modify the nodes
instead of waiting for them to get updated by an external process.
The `ModuleDay0`'s output artifacts should exist **before** we modify the node.

If the `spec.containerImage` in the `ModuleDay0` is different than the
`spec.osImageURL` in the `MachineConfig` then KMM will override `spec.osImageURL`
in the `MachineConfig` with the image from the `ModuleDay0`.

MCO will then override the node with the new content and potentially reboot the
nodes.

### Building

If a customer wishes KMM to re-build his kmods, they should add a `build` section
in the `ModuleDay0` as follow
```
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: ModuleDay0
spec:
  machineConfigName: 99-ybettan-external-image
  containerImage: quay.io/ybettan/rhcos:414.92.202312311229-0
  build:
    secrets:
      - name: build-secret
    dockerfileConfigMap:
      name: kmm-kmod-dockerfile
```

The build will be triggered if all the following conditions are met
  * `quay.io/ybettan/rhcos:414.92.202312311229-0` is different from `spec.osImageURL` in the `MachineConfig`
  * `quay.io/ybettan/rhcos:414.92.202312311229-0` doesn't exist
  * A `build` section exists in the `ModuleDay0`

Once built, KMM will modify the `spec.osImageURL` in the `MachineConfig` with the new image.

The `ConfigMap` should be in the following format
```
apiVersion: v1
kind: ConfigMap
metadata:
  name: kmm-kmod-dockerfile
data:
  dockerfile: |

    ARG DTK_AUTO
    ARG OS_AUTO

    FROM ${DTK_AUTO} as builder

    ARG KERNEL_VERSION
    ...
    RUN KERNEL_SRC_DIR=/lib/modules/${KERNEL_VERSION}/build make all

    FROM ${OS_AUTO}

    ARG KERNEL_VERSION

    COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_a.ko /opt/lib/modules/${KERNEL_VERSION}/
    COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_b.ko /opt/lib/modules/${KERNEL_VERSION}/
    RUN depmod -b /opt ${KERNEL_VERSION}
```

`OS_AUTO` will fetch the `rhel-coreos` image from the release-payload of the
cluster.
The DTK `ImageStream` on the cluster will contain the DTK image that fits the
release-payload and not the running OS on the nodes (which may have a different
kernel), therefore, `DTK_AUTO` should only be used if `OS_AUTO` is used.

EXAMPLE using `OS_AUTO` and `DTK_AUTO`:
A user wants us to rebuild `kmm-kmod` for him using the Dockerfile mentioned
above.
  * KMM will fetch the `rhel-coreos` image ref from the release payload of the
    running cluster and use it to override `OS_AUTO`.
  * KMM will get the DTK image ref from `imagestream/driver-toolkit` in the
    `openshift` namespace (from the `latest` tag) and use it to override `DTK_AUTO`.
  * Finally, KMM will build the new image.


### Signing

If a customer wishes KMM to re-build his kmods, they should add a `sign` section
in the `ModuleDay0` as follow
```
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: ModuleDay0
spec:
  machineConfigName: 99-ybettan-external-image
  containerImage: quay.io/ybettan/rhcos:414.92.202312311229-0
  sign:
    unsignedImage: quay.io/ybettan/rhcos:414.92.202312311229-0-unsigned
    certSecret:
      name: kmm-kmod-signing-cert
    keySecret:
      name: kmm-kmod-signing-key
    filesToSign:
      - /opt/lib/modules/<kernel version>/kmm_ci_a.ko
```

The signing will be triggered if all the following conditions are met
  * `quay.io/ybettan/rhcos:414.92.202312311229-0` is different from `spec.osImageURL` in the `MachineConfig`
  * `quay.io/ybettan/rhcos:414.92.202312311229-0-unsigned` exist (or a `build` section exists)
  * A `sign` section exists in the `ModuleDay0`

Once signed and pushed, KMM will modify the `MachineConfig`'s `spec.osImageURL`
with the signed image.

### Firmware

No much to do here, the user can put his firmware files directly in
`/lib/firmware` on the nodes in the custom container image or set
`firmware_class.path` in their image's `sysctl.conf` so the firmware feature of
KMM is a bit useless here.

### Preflight

In day2 we need preflight because we upgrade the cluster before building the
artifacts but in day0 we first generate the artifacts and only then modify the
`MachineConfig` with the new image, therefore, if a build/sign failed we won't
reach the stage of modifying the `MachineConfig` at all and the cluster will
remain valid.

### Cluster upgrade

When a `MachineConfig` uses `osImageURL`, it is called
[image-layering](https://docs.openshift.com/container-platform/4.14/post_installation_configuration/coreos-layering.html).

When image-layering is used, the OCP layer is being "detached" from the underlying
OS running on the nodes. It means that when a cluster upgrade is triggered, for
example from `4.14` to `4.15`, the OS on the nodes remain to be the same and the
user only gets the new OCP features without the new RHCOS features.

If a user has layered the images on the cluster, it is their responsibility to
upgrade the nodes when they wish.

In the day-0 case, KMM can enhance the UX and make the customer feel like a
cluster upgrade is still upgrading the nodes.

The way we can achieve that for new OCP y-streams is as follows:

We check `clusterversion/version`'s `.status.desired.version` and
`node.status.nodeInfo.osImage` and see if the y-stream in both versions differ.

If they do, we can rebuild the container image (if the user has supplied the
Dockerfile) and using `DTK_AUTO` build-arg to find the DTK image and `OS_AUTO`
to get the final base image for the OS to contain the kmods.

Since this is not the expected behavior when image-layering is used, a user
that want such behavior will have to set it explicitly in the `ModuleDay0`
```
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: ModuleDay0
spec:
  machineConfigName: 99-ybettan-external-image
  containerImage: quay.io/ybettan/rhcos:414.92.202312311229-0
  clusterUpgradeTriggerNodeUpgrade: true
  build:
  ...
```

### Hub&Spoke

TODO: Do we want to have it at this point?
