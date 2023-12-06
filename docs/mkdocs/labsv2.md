# KMM v2.0 Labs

KMM Labs is a series of examples and use cases of Kernel Module Management Operator running on an Openshift cluster.


## Installation

If you want to learn more about KMM and how to install it, you can refer to the [documentation](https://docs.openshift.com/container-platform/4.13/hardware_enablement/kmm-kernel-module-management.html). The documentation provides detailed information about KMM and step-by-step instructions for the installation process.


## Driver Toolkit

We could use any image that includes the necessary libraries and dependencies for building a kernel module for our specific version. However, for ease, we can leverage Driver Toolkit (DTK), which is a container image used as a base image for building kernel modules.

When using DTK, we should set which version of the Driver Toolkit [image](https://github.com/openshift/driver-toolkit#finding-the-driver-toolkit-image-url-in-the-payload) we are using to match the exact kernel version running on OpenShift nodes. In our case, we are using DTK with KMM and KMM will automatically determine what DTK digest should be used if the `DTK_AUTO` build argument is specified in the Dockerfile.

We can get further info at DTK [repository](https://github.com/openshift/driver-toolkit#readme).


## Building In-Cluster Modules from Sources

In this example, we will demonstrate how to build the kernel module in our cluster using sources from a git repository. For this and next examples it is assumed that a namespace `labsv2` and a ServiceAccount `labsv2sa` allowed to use the privileged SCC have been previously created.

The process involves several steps:

1. Adding a ConfigMap: We will create a ConfigMap to store all the necessary information for the build process, including the image we are using (DTK in this case), the remote git repository containing the sources, and the compiling commands. It contains the `Dockerfile` used in the build process.

2. Creating the Module object: Next, we'll create the Module object itself. It will specify the image to be built and used, a secret for possible authentication, ConfigMap for building process, and the nodes where the pods should be scheduled.

Applying next file in an OpenShift cluster should build and load a kmm-ci-a module in the nodes labeled with a `worker` role:

`in_cluster_build.yaml`

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: build-module-labs
  namespace: labsv2 
data:
  dockerfile: |
    ARG DTK_AUTO
    ARG KERNEL_VERSION
    FROM ${DTK_AUTO} as builder
    WORKDIR /build/
    RUN git clone -b main --single-branch https://github.com/rh-ecosystem-edge/kernel-module-management.git
    WORKDIR kernel-module-management/ci/kmm-kmod/
    RUN make
    FROM docker.io/redhat/ubi8-minimal
    ARG KERNEL_VERSION
    RUN microdnf -y install kmod openssl && \
        microdnf clean all && \
        rm -rf /var/cache/yum
    RUN mkdir -p /opt/lib/modules/${KERNEL_VERSION}
    COPY --from=builder /build/kernel-module-management/ci/kmm-kmod/*.ko /opt/lib/modules/${KERNEL_VERSION}/
    RUN /usr/sbin/depmod -b /opt
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: kmm-ci-a
  namespace: labsv2
spec:
  moduleLoader:
    serviceAccountName: labsv2sa
    container:
      modprobe:
        moduleName: kmm-ci-a
      kernelMappings:
        - regexp: '^.*'
          containerImage: myrepo.xyz/kmm-labs:kmm-kmod-${KERNEL_FULL_VERSION}-v1.0
          build:
            dockerfileConfigMap:
              name: build-module-labs
  imageRepoSecret: 
    name: myrepo-pull-secret
  selector:
    node-role.kubernetes.io/worker: ""

```

## Building and Signing In-Cluster Modules from Sources

Signed kernel modules provide a mechanism for the kernel to verify the integrity of a module primarily for use with UEFI Secure Boot.


This next example is pretty much the same as above except that we will generate a private key and certificate to sign the resulting .ko file after the build process.


Generate key and certificate files and create secrets based on them:

```bash
openssl req -x509 -sha256 -nodes -days 365 -newkey rsa:2048 -keyout private.key -out certificate.crt
oc create secret generic mykey --from-file=key=private.key
oc create secret generic mycert --from-file=cert=certificate.crt
```

Build and sign resulting module file:

`build_and_sign.yaml`
```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: labs-dockerfile
  namespace: labsv2
data:
  dockerfile: |
    ARG DTK_AUTO
    ARG KERNEL_VERSION
    FROM ${DTK_AUTO} as builder
    WORKDIR /build/
    RUN git clone -b main --single-branch https://github.com/rh-ecosystem-edge/kernel-module-management.git
    WORKDIR kernel-module-management/ci/kmm-kmod/
    RUN make
    FROM docker.io/redhat/ubi8-minimal
    ARG KERNEL_VERSION
    RUN microdnf -y install kmod openssl && \
        microdnf clean all && \
        rm -rf /var/cache/yum
    RUN mkdir -p /opt/lib/modules/${KERNEL_VERSION}
    COPY --from=builder /build/kernel-module-management/ci/kmm-kmod/*.ko /opt/lib/modules/${KERNEL_VERSION}/
    RUN /usr/sbin/depmod -b /opt
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: labs-signed-module
  namespace: labsv2 
spec:
  moduleLoader:
    serviceAccountName: labsv2sa
    container:
      modprobe:
        moduleName: kmm_ci_a
      kernelMappings:
        - regexp: '^.*\.x86_64$'
          containerImage: myrepo.xyz/kmm-labs:signed-module-1.0
          build:
            dockerfileConfigMap:
              name: labs-dockerfile
          sign:
            keySecret:
              name: mykey
            certSecret:
              name: mycert
            filesToSign:
              - /opt/lib/modules/$KERNEL_FULL_VERSION/kmm_ci_a.ko
  imageRepoSecret: 
    name: myrepo-pull-secret
  selector: 
    node-role.kubernetes.io/worker: ""
```

## Load Module from an existing image

This is the simplest example in the Labs. We have an existing image of the kernel module for a specific kernel version and we want KMM to manage and load it:

`load.yaml`
```yaml
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: kmm-ci-a
  namespace: labsv2 
spec:
  moduleLoader:
    serviceAccountName: labsv2sa
    container:
      modprobe:
        moduleName: kmm-ci-a
      kernelMappings:
        - regexp: '^.*'
          containerImage: myrepo.xyz/kmm-labs:kmm-kmod-5.14.0-284.36.1.el9_2.x86_64-v1.0
  imageRepoSecret: 
    name: myrepo-pull-secret
  selector:
    node-role.kubernetes.io/worker: ""
```

## Replacing In-Tree Modules: Removing the existing Module before loading a new One

We might need to remove an existing in-tree module before loading an out-of-tree one.
Some use cases may be including the addition of more features, fixes, or patches to the out-of-tree module for the same driver, as well as encountering incompatibilities between the in-tree and out-of-tree modules.

To achieve this we can remove in-tree modules just adding `inTreeModuleToRemove: <NameoftheModule>`. In our example we wil remove `joydev` module which is the standard in-tree driver included in the Linux kernel for joysticks or similar input devices:

`remove_in-tree.yaml`
```yaml
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: kmm-ci-a
  namespace: labsv2
spec:
  moduleLoader:
    serviceAccountName: labsv2sa
    container:
      modprobe:
        moduleName: kmm-ci-a
      kernelMappings:
        - regexp: '^.*'
          containerImage: quay.io/ebelarte/kmm-labs:kmm-kmod-5.14.0-284.36.1.el9_2.x86_64-v1.0
      inTreeModuleToRemove: joydev
  imageRepoSecret: 
    name: myrepo-pull-secret
  selector:
    node-role.kubernetes.io/worker: ""
```

We can confirm that `joydev` module is not loaded and our new module is loaded now just by listing the modules in one of our nodes and check that there is no output:
```
05:19:01 root@rhcs-sign-test labsv2-test → oc debug node/ebl-worker-1.edgeinfra.cloud 
Temporary namespace openshift-debug-mr26h is created for debugging node...
Starting pod/ebl-worker-1edgeinfracloud-debug ...
To use host binaries, run `chroot /host`

Pod IP: 192.168.122.101
If you don't see a command prompt, try pressing enter.
sh-4.4# 
sh-4.4# chroot /host
sh-5.1# lsmod | grep joydev
sh-5.1# lsmod | grep kmm   
kmm_ci_a               16384  0
sh-5.1# 
```

## Device Plugins


Device plugins play a crucial role in allocating and reporting specific hardware devices to the cluster. They are used for providing user-space configuration to the hardware devices and reporting those to the Kubernetes API so later scheduling decisions can be made.

In KMM, you can load device plugin images as part of the Module object. 

For this example, we will utilize this plugin called [simple-device-plugin](https://github.com/yevgeny-shnaidman/simple-device-plugin) to simulate device plugins. This plugin provides a way to emulate device plugins and test their functionality within the cluster.


`device_plugin.yaml`
```yaml
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: kmm-ci-a
  namespace: labsv2 
spec:
  devicePlugin:
    serviceAccountName: labsv2sa
    container:
      image: myrepo.xyz/simple-device-plugin:latest
      imagePullPolicy: Always
      args:
      - "-config=config-labs.yaml"
  moduleLoader:
    serviceAccountName: labsv2sa
    container:
      modprobe:
        moduleName: kmm-ci-a
      kernelMappings:
        - regexp: '^.*'
          containerImage: myrepo.xyz/kmm-labs:kmm-kmod-5.14.0-284.36.1.el9_2.x86_64-v1.0
  imageRepoSecret:
    name: myrepo-pull-secret
  selector:
    node-role.kubernetes.io/worker: ""

```
We can check that our device plugin app has been loaded just describing one of the selected nodes and looking at the resources.

```bash
$ oc describe node mynode
...
Allocated resources:
  (Total limits may be over 100 percent, i.e., overcommitted.)
  Resource                   Requests      Limits
  --------                   --------      ------
  cpu                        749m (9%)     0 (0%)
  memory                     4406Mi (29%)  0 (0%)
  ephemeral-storage          0 (0%)        0 (0%)
  hugepages-1Gi              0 (0%)        0 (0%)
  hugepages-2Mi              0 (0%)        0 (0%)
  dummy/dummyDev             0             0
  example.com/ptemplate      0             0
  example.com/simple-device  0             0
...
```

##  Seamless upgrade of kernel Module: Orderly process without reboot

To minimize the impact on the workload running in the cluster, kernel module upgrades should be performed on a per-node basis in an order that can be defined/managed by user.

Please notice that using ordered upgrade implies that a `version` field at the original Module exists prior to the upgrade process.

First, label the specific node that requires the kernel module upgrade. Use the following format:

```
kmm.node.kubernetes.io/version-module.<module-namespace>.<module-name>=$moduleVersion
```

In our case we'll use this example:
```bash
oc label node my-node-1 kmm.node.kubernetes.io/version-module.labsv2.kmm-ci-a=0.9
```

Then we can use a new build based on previous build example by just adapting the Module (adding `version` and specific `containerImage` for that version) and reusing existing Configmap for build. Make sure you have deleted any Module object left from past examples and apply following Module:

`build_with_version.yaml`
```yaml
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: kmm-ci-a
  namespace: labsv2
spec:
  moduleLoader:
    serviceAccountName: labsv2sa
    container:
      version: "0.9"
      modprobe:
        moduleName: kmm-ci-a
      kernelMappings:
        - regexp: '^.*'
          containerImage: myrepo.xyz/kmm-labs:kmm-kmod-${KERNEL_FULL_VERSION}-v0.9
          build:
            dockerfileConfigMap:
              name: build-module-labs
  imageRepoSecret: 
    name: myrepo-pull-secret
  selector:
    node-role.kubernetes.io/worker: ""

```

A new module named `kmm-ci-a` with object version 0.9 and image `kmm-kmod-${KERNEL_FULL_VERSION}-v0.9` will be created at node `my-node-1`.
If we look back at the first example in the Lab about In-Cluster building from sources we will find that containerImage was `kmm-kmod-${KERNEL_FULL_VERSION}-v1.0` and if made all the examples we should have that v1.1 image at our registry.
Let's pretend that we want to update our actual v0.9 module to v1.0.

1. Check in advance that you have both v0.9 and v1.0 images at your registry.
2. Remove 0.9 label from the node:
```
oc label node my-node-1 kmm.node.kubernetes.io/version-module.labsv2.kmm-ci-a-
```
3. Check that unlabeling the node killed the Module pods.
4. Label with the new desired version:
```
oc label node my-node-1 kmm.node.kubernetes.io/version-module.labsv2.kmm-ci-a=1.0
```
5. Edit the module object and change both `version` and `containerImage` with the new version number and matching image 
   (In our example 1.0 for the version and `kmm-kmod-${KERNEL_FULL_VERSION}-v1.0` for the containerImage):
```
oc edit module kmm-ci-a
```
6. The new module version is loaded.


## Loading soft dependencies

Kernel modules sometimes have dependencies on other modules so these dependencies have to be loaded in advance. If module symbols exist then `modprobe` would load the dependant modules automatically but if there are no symbols and we need to load dependant modules we can set those in our `Module` object definition.

In the following example `kmm-ci-a` depends on `kmm-ci-b` so we will set `modulesLoadingOrder` and then the list with the `moduleName` as the first entry followed by all of its dependencies. In our case it is just `kmm-ci-b` but it could be a longer list with multiple dependency modules.

Apply next Module and `kmm-ci-b` will be loaded before `kmm-ci-a`:

`load_dependencies.yaml`
```yaml
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: kmm-ci-a
  namespace: labsv2 
spec:
  moduleLoader:
    serviceAccountName: labsv2sa
    container:
      modprobe:
        moduleName: kmm-ci-a
        modulesLoadingOrder:
          - kmm-ci-a
          - kmm-ci-b
      kernelMappings:
        - regexp: '^.*\.x86_64$'
          containerImage: quay.io/myrepo/kmmo-lab:kmm-kmod-${KERNEL_FULL_VERSION}
  imageRepoSecret: 
    name: myrepo-pull-secret
  selector:
    node-role.kubernetes.io/worker: ""
```

And again a simple `lsmod` at the node debug should show the loaded modules:

```bash
06:28:08 root@rhcs-sign-test labsv2-test → oc debug node/ebl-worker-1.edgeinfra.cloud 
Temporary namespace openshift-debug-72dx4 is created for debugging node...
Starting pod/ebl-worker-1edgeinfracloud-debug ...
To use host binaries, run `chroot /host`
Pod IP: 192.168.122.101
If you don't see a command prompt, try pressing enter.
sh-4.4# chroot /host
sh-5.1# lsmod | grep kmm
kmm_ci_a               16384  0
kmm_ci_b               16384  0
sh-5.1# 
```

## Troubleshoot

In addition to standard OpenShift tools such as [must-gather](https://docs.openshift.com/container-platform/4.13/support/gathering-cluster-data.html) or general system [event](https://docs.openshift.com/container-platform/4.13/nodes/clusters/nodes-containers-events.html) information which can be really helpful for debugging, other possible sources of information we may check are:

1. Operator logs:
```
oc logs -fn openshift-kmm deployments/kmm-operator-controller
```
2. Module object description:
```
oc describe module <module-name>
```
3. Node description:
```
oc describe node <node-name>
```
4. Namespace events:
```
oc get events -n <my-module-namespace> --sort-by='{.lastTimestamp}'
```
