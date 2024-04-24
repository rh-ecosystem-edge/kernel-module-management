# Creating a kmod image

Kernel Module Management works with purpose-built kmod images, which are standard OCI images that contain `.ko` files.  
The location of those files must match the following pattern:
```
<prefix>/lib/modules/[kernel-version]/
```

Where:

- `<prefix>` should be equal to `/opt` in most cases, as it is the `Module` CRD's default value;
- `kernel-version` must be non-empty and equal to the kernel version the kernel modules were built for.

## `depmod`

It is recommended to run `depmod` at the end of the build process to generate `modules.dep` and map files.
This is especially useful if your kmod image contains several kernel modules and if one of the modules depend on
another.
To generate dependencies and map files for a specific kernel version, run `depmod -b /opt ${KERNEL_FULL_VERSION}`.

## Example `Dockerfile`

The example below builds a test kernel module from the KMM repository.
Please note that a Red Hat subscription is required to download the `kernel-devel` package.
If you are building your image on OpenShift, consider [using Driver Toolkit](#using-driver-toolkit--dtk-) or [using an 
entitled build](https://cloud.redhat.com/blog/how-to-use-entitled-image-builds-to-build-drivercontainers-with-ubi-on-openshift).

```dockerfile
FROM registry.redhat.io/ubi9/ubi as builder

ARG KERNEL_FULL_VERSION

RUN dnf install -y \
    gcc \
    git \
    kernel-devel-${KERNEL_FULL_VERSION} \
    make

WORKDIR /usr/src

RUN ["git", "clone", "https://github.com/rh-ecosystem-edge/kernel-module-management.git"]

WORKDIR /usr/src/kernel-module-management/ci/kmm-kmod

RUN KERNEL_SRC_DIR=/lib/modules/${KERNEL_FULL_VERSION}/build make all

FROM registry.redhat.io/ubi9/ubi-minimal

ARG KERNEL_FULL_VERSION

RUN ["microdnf", "install", "-y", "kmod"]

COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_a.ko /opt/lib/modules/${KERNEL_FULL_VERSION}/
COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_b.ko /opt/lib/modules/${KERNEL_FULL_VERSION}/

RUN depmod -b /opt ${KERNEL_FULL_VERSION}
```

## Building in cluster

KMM is able to build kmod images in cluster.
Build instructions must be provided using the `build` section of a kernel mapping.
The `Dockerfile` for your container image should be copied into a `ConfigMap` object, under the `Dockerfile` key.
The `ConfigMap` needs to be located in the same namespace as the `Module`.

KMM will first check if the image name specified in the `containerImage` field exists.
If it does, the build will be skipped.
Otherwise, KMM will create a [`Build`](https://docs.openshift.com/container-platform/4.12/cicd/builds/build-configuration.html)
object to build your image.

The following build arguments are automatically set by KMM:

| Name                  | Description                            | Example                       |
|-----------------------|----------------------------------------|-------------------------------|
| `KERNEL_FULL_VERSION` | The kernel version we are building for | `5.14.0-70.58.1.el9_0.x86_64` |
| `MOD_NAME`            | The `Module`'s name                    | `my-mod`                      |
| `MOD_NAMESPACE`       | The `Module`'s namespace               | `my-namespace`                |

Successful build pods are garbage-collected immediately, unless [`job.gcDelay`](./configure.md#jobgcdelay) is set in the
operator configuration.  
Failed build pods are always preserved and must be deleted manually by the administrator for the build to be restarted.

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

!!! warning "OpenShift's internal container registry is not enabled by default on bare metal clusters"

    A common pattern is to push kmod images to OpenShift's internal image registry once they are built.
    That registry is not enabled by default on bare metal installations of OpenShift.
    Refer to [Configuring the registry for bare metal](https://docs.openshift.com/container-platform/4.13/registry/configuring_registry_storage/configuring-registry-storage-baremetal.html)
    to enable it.

### Using Driver Toolkit (DTK)

[Driver Toolkit](https://docs.openshift.com/container-platform/4.12/hardware_enablement/psap-driver-toolkit.html) is a
convenient base image that contains most tools and libraries required to build kmod images for the OpenShift version
that is currently running in the cluster.
It is recommended to use DTK as the first stage of a multi-stage `Dockerfile` to build the kernel modules, and to copy
the `.ko` files into a smaller end-user image such as [`ubi-minimal`](https://catalog.redhat.com/software/containers/ubi9/ubi-minimal).

To leverage DTK in your in-cluster build, use the `DTK_AUTO` build argument.
The value is automatically set by KMM when creating the `Build` object.

```dockerfile
ARG DTK_AUTO

FROM ${DTK_AUTO} as builder

ARG KERNEL_FULL_VERSION

WORKDIR /usr/src

RUN ["git", "clone", "https://github.com/rh-ecosystem-edge/kernel-module-management.git"]

WORKDIR /usr/src/kernel-module-management/ci/kmm-kmod

RUN KERNEL_SRC_DIR=/lib/modules/${KERNEL_FULL_VERSION}/build make all

FROM ubi9/ubi-minimal

ARG KERNEL_FULL_VERSION

RUN ["microdnf", "install", "-y", "kmod"]

COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_a.ko /opt/lib/modules/${KERNEL_FULL_VERSION}/
COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_b.ko /opt/lib/modules/${KERNEL_FULL_VERSION}/

RUN depmod -b /opt ${KERNEL_FULL_VERSION}
```

### Depending on in-tree kernel modules

Some kernel modules depend on other kernel modules shipped with the node's distribution.
To avoid copying those dependencies into the kmod image, KMM mounts `/usr/lib/modules` into both the build and the
worker Pod's filesystems.  
By creating a symlink from `/opt/usr/lib/modules/[kernel-version]/[symlink-name]` to
`/usr/lib/modules/[kernel-version]`, `depmod` can use the in-tree kmods on the building node's filesystem to resolve
dependencies.
At runtime, the worker Pod extracts the entire image, including the `[symlink-name]` symbolic link.
That link points to `/usr/lib/modules/[kernel-version]` in the worker Pod, which is mounted from the node's filesystem.
`modprobe` can then follow that link and load the in-tree dependencies as needed.

In the example below, we use `host` as the symbolic link name under `/opt/usr/lib/modules/[kernel-version]`:

```dockerfile
ARG DTK_AUTO

FROM ${DTK_AUTO} as builder

#
# Build steps
#

FROM ubi9/ubi

ARG KERNEL_FULL_VERSION

RUN dnf update && dnf install -y kmod

COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_a.ko /opt/lib/modules/${KERNEL_FULL_VERSION}/
COPY --from=builder /usr/src/kernel-module-management/ci/kmm-kmod/kmm_ci_b.ko /opt/lib/modules/${KERNEL_FULL_VERSION}/

# Create the symbolic link
RUN ln -s /lib/modules/${KERNEL_FULL_VERSION} /opt/lib/modules/${KERNEL_FULL_VERSION}/host

RUN depmod -b /opt ${KERNEL_FULL_VERSION}
```

!!! warning
    `depmod` will generate dependency files based on the kernel modules present on the node that runs the kmod image
    build.  
    On the node on which KMM loads the kernel modules, `modprobe` will expect the files to be present under
    `/usr/lib/modules/[kernel-version]`, and the same filesystem layout.  
    It is highly recommended that the build and the target nodes share the same distribution and release.
