# Creating a ModuleLoader image

Kernel Module Management works with purpose-built ModuleLoader images.
Those are standard OCI images that satisfy a few requirements:

- `.ko` files must be located under `/opt/lib/modules/${KERNEL_VERSION}`
- the `modprobe` and `sleep` binaries must be in the `$PATH`.

## `depmod`

It is recommended to run `depmod` at the end of the build process to generate `modules.dep` and map files.
This is especially useful if your ModuleLoader image contains several kernel modules and if one of the modules depend on
another.
To generate dependencies and map files for a specific kernel version, run `depmod -b /opt ${KERNEL_VERSION}`.

## Example `Dockerfile`

The example below builds a test kernel module from the KMM repository.
Please note that a Red Hat subscription is required to download the `kernel-devel` package.
If you are building your image on OpenShift, consider [using Driver Toolkit](#using-driver-toolkit--dtk-) or [using an 
entitled build](https://cloud.redhat.com/blog/how-to-use-entitled-image-builds-to-build-drivercontainers-with-ubi-on-openshift).

```dockerfile
FROM registry.redhat.io/ubi8/ubi as builder

ARG KERNEL_VERSION

RUN dnf install -y \
    gcc \
    git \
    kernel-devel-${KERNEL_VERSION} \
    make

WORKDIR /usr/src

RUN ["git", "clone", "https://github.com/rh-ecosystem-edge/kernel-module-management.git"]

WORKDIR /usr/src/kernel-module-management/ci/kmm-kmod

RUN KERNEL_SRC_DIR=/lib/modules/${KERNEL_VERSION}/build make all

FROM registry.redhat.io/ubi8/ubi-minimal

ARG KERNEL_VERSION

RUN microdnf install kmod

COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_a.ko /opt/lib/modules/${KERNEL_VERSION}/
COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_b.ko /opt/lib/modules/${KERNEL_VERSION}/

RUN depmod -b /opt ${KERNEL_VERSION}
```

## Building in cluster

KMM is able to build ModuleLoader images in cluster.
Build instructions must be provided using the `build` section of a kernel mapping.
The `Dockerfile` for your container image should be copied into a `ConfigMap` object, under the `Dockerfile` key.
The `ConfigMap` needs to be located in the same namespace as the `Module`.

KMM will first check if the image name specified in the `containerImage` field exists.
If it does, the build will be skipped.
Otherwise, KMM creates a [`Build`](https://docs.openshift.com/container-platform/4.12/cicd/builds/build-configuration.html)
object to build your image.
Once the image is built, KMM proceeds with the `Module` reconciliation.

```yaml
# ...
- regexp: '^.+$'
  containerImage: "some.registry/org/my-kmod:${KERNEL_FULL_VERSION}"
  build:
    buildArgs:  # Optional
      - name: ARG_NAME
        value: some-value
    secrets:  # Optional
      - name: some-kubernetes-secret  # Will be mounted in the build pod as /run/secrets/some-kubernetes-secret.
    baseImageRegistryTLS:
      # Optional and not recommended! If true, the build will be allowed to pull the image in the Dockerfile's
      # FROM instruction using plain HTTP.
      insecure: false
      # Optional and not recommended! If true, the build will skip any TLS server certificate validation when
      # pulling the image in the Dockerfile's FROM instruction using plain HTTP.
      insecureSkipTLSVerify: false
    dockerfileConfigMap:  # Required
      name: my-kmod-dockerfile
  registryTLS:
    # Optional and not recommended! If true, KMM will be allowed to check if the container image already exists
    # using plain HTTP.
    insecure: false
    # Optional and not recommended! If true, KMM will skip any TLS server certificate validation when checking if
    # the container image already exists.
    insecureSkipTLSVerify: false
```

### Using Driver Toolkit (DTK)

[Driver Toolkit](https://docs.openshift.com/container-platform/4.12/hardware_enablement/psap-driver-toolkit.html) is a
convenient base image that contains most tools and libraries required to build ModuleLoader images for the OpenShift
version that is currently running in the cluster.
It is recommended to use DTK as the first stage of a multi-stage `Dockerfile` to build the kernel modules, and to copy
the `.ko` files into a smaller end-user image such as [`ubi-minimal`](https://catalog.redhat.com/software/containers/ubi8/ubi-minimal).

To leverage DTK in your in-cluster build, use the `DTK_AUTO` build argument.
The value is automatically set by KMM when creating the `Build` object.

```dockerfile
ARG DTK_AUTO

FROM ${DTK_AUTO} as builder

ARG KERNEL_VERSION

WORKDIR /usr/src

RUN ["git", "clone", "https://github.com/rh-ecosystem-edge/kernel-module-management.git"]

WORKDIR /usr/src/kernel-module-management/ci/kmm-kmod

RUN KERNEL_SRC_DIR=/lib/modules/${KERNEL_VERSION}/build make all

FROM registry.redhat.io/ubi8/ubi-minimal

ARG KERNEL_VERSION

RUN microdnf install kmod

COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_a.ko /opt/lib/modules/${KERNEL_VERSION}/
COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_b.ko /opt/lib/modules/${KERNEL_VERSION}/

RUN depmod -b /opt ${KERNEL_VERSION}
```