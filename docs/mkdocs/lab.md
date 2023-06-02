# KMM on OpenShift Lab

KMM Labs is a series of examples and use cases of Kernel Module Management Operator running on an Openshift cluster.

### Installing KMM

Refer to the [Installing](documentation/install.md) page of the documentation.

### Driver Toolkit

In the case of in-cluster builds we leverage the existence of the
[Driver Toolkit](https://github.com/openshift/driver-toolkit), a container image that includes all the necessary
packages and tools to build a kernel module in an OpenShift cluster.

Driver Toolkit image version should correspond to our Openshift server version.
Check the [DTK documentation](https://github.com/openshift/driver-toolkit#finding-the-driver-toolkit-image-url-in-the-payload)
for further information on this.

## In-cluster build from sources (multi-stage)

Create a Module called `kmm-ci-a` which will build a kernel module from sources in a remote git repository if kernel
version in the node matches the kernel version set in the literal/regexp field.
Once the module is built, copy it in a next step and create the definitive image which will be uploaded to the OpenShift
internal registry and used as the Module Loader in the selected nodes.
This multi-stage example could be useful if you need the use of extra software which is not present at DTK image but it
is in the destination image on which the module files are copied.

All `Dockerfile` steps needed in order to build a module from sources should be configured in a `ConfigMap` defined
previous to the Module object creation.

<details>
<summary>Full example</summary>

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: build-module-multi
data:
  dockerfile: |
    FROM quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:82faeb6a8caa174d9df3d259945ca161311fe6231d628e34ee0f1c8528371229 AS builder
    ARG KERNEL_VERSION
    WORKDIR /build
    RUN git clone https://github.com/rh-ecosystem-edge/kernel-module-management.git
    WORKDIR /build/kernel-module-management/ci/kmm-kmod
    RUN make

    FROM registry.redhat.io/ubi9/ubi-minimal
    ARG KERNEL_VERSION

    RUN ["microdnf", "-y", "install", "kmod"]

    COPY --from=builder /build/kernel-module-management/ci/kmm-kmod/*.ko /opt/lib/modules/${KERNEL_VERSION}/
    RUN depmod -b /opt
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: kmm-ci-a
spec:
  moduleLoader:
    container:
      modprobe:
        moduleName: kmm-ci-a
      kernelMappings:
        - literal: 4.18.0-372.19.1.el8_6.x86_64
          containerImage: image-registry.openshift-image-registry.svc:5000/default/kmm-kmod:4.18.0f
          build:
            dockerfileConfigMap:
              name: build-module-multi

  selector:
    feature.kmm.lab: 'true'
```

</details>

## In-cluster build from sources (Single step)

Create a Module called `kmm-ci-a` which will build a kernel module from sources in a remote git repository if kernel
version in the node matches the kernel version set in the literal/regexp field.
Once the module is built using the same DTK image as a base the definitive image will be uploaded to the OpenShift
internal registry and used as the Module Loader in the selected nodes.
As in previous example, we are in-cluster building a module from sources so we need to define a `ConfigMap` that
contains the `Dockerfile`.

<details>
<summary>Full example</summary>

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: build-module-single
data:
  dockerfile: |
    FROM quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:82faeb6a8caa174d9df3d259945ca161311fe6231d628e34ee0f1c8528371229 AS builder
    ARG KERNEL_VERSION
    WORKDIR /build
    RUN git clone https://github.com/rh-ecosystem-edge/kernel-module-management.git
    WORKDIR /build/kernel-module-management/ci/kmm-kmod
    RUN make

    RUN mkdir -p /opt/lib/modules/${KERNEL_VERSION} && \
        cp /build/kmm-kmod/*.ko /opt/lib/modules/${KERNEL_VERSION}/
    RUN depmod -b /opt
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: kmm-ci-a
spec:
  moduleLoader:
    container:
      modprobe:
        moduleName: kmm-ci-a
      kernelMappings:
        - literal: 4.18.0-372.19.1.el8_6.x86_64
          containerImage: image-registry.openshift-image-registry.svc:5000/default/kmm-kmod:4.18.0single
          build:
            dockerfileConfigMap:
              name: build-module-single
  selector:
    feature.kmm.lab: 'true'
```
</details>

## Load a Pre-built driver image

Set an existing image which include the prebuilt module files to load if kernel version and selector settings match the
KernelMappings regular expression and the selector label/s.

<details>
<summary>Full example</summary>

```yaml
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: kmm-ci-a
spec:
  moduleLoader:
    container:
      modprobe:
        moduleName: kmm-ci-a
      kernelMappings:
        - literal: 4.18.0-372.19.1.el8_6.x86_64
          containerImage: image-registry.openshift-image-registry.svc:5000/default/kmm-kmod:4.18.1single
  selector:
    feature.kmm.lab: 'true'
```
</details>

## Use a Device Plugin

Set an existing image which include software to manage specific hardware in cluster.
In our example we will be using an image of a device plugin based on
[this project](https://github.com/redhat-nfvpe/k8s-dummy-device-plugin) which sets different values on environment
variables faking them as "dummy devices".

```shell
$ oc exec -it dummy-pod -- printenv | grep DUMMY_DEVICES
DUMMY_DEVICES=dev_3,dev_4
```

<details>
<summary>Full example</summary>

```yaml
---
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: kmm-ci-a
spec:
  devicePlugin:
    container:
      image: "quay.io/<org>/oc-dummy-device-plugin:0.1"
  moduleLoader:
    container:
      modprobe:
        moduleName: kmm-ci-a
      kernelMappings:
        - literal: 4.18.0-372.19.1.el8_6.x86_64
          containerImage: image-registry.openshift-image-registry.svc:5000/default/kmm-kmod:4.18.0single
  selector:
    feature.kmm.lab: 'true'
```
</details>

## Troubleshooting

A straightforward way to check if our module is effectively loaded is check within the Module Loader pod:
```shell
[enrique@ebelarte crds]$ oc get po
NAME                                READY   STATUS      RESTARTS   AGE
kmm-ci-a-hk472-1-build              0/1     Completed   0          106s
kmm-ci-a-p4qx8-tmpch                1/1     Running     0          24s
[enrique@ebelarte crds]$ oc exec -it kmm-ci-a-p4qx8-tmpch -- lsmod | grep kmm
kmm_ci_a               16384  0
[enrique@ebelarte crds]$
```

Refer to the [Troubleshooting page](documentation/troubleshooting.md) for further steps.
