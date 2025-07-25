# Deploying kernel modules

KMM watches `Node` and `Module` resources in the cluster to determine if a kernel module should be loaded on or unloaded
from a node.  
To be eligible for a `Module`, a `Node` must:

- have labels that match the `Module`'s `.spec.selector` field;
- run a kernel version matching one of the items in the `Module`'s `.spec.moduleLoader.container.kernelMappings`;
- if [ordered upgrade](ordered_upgrade.md) is configured in the `Module`, have a label that matches its
  `.spec.moduleLoader.container.version` field.

When KMM needs to reconcile nodes with the desired state as configured in the `Module` resource, it creates worker Pods
on the target node(s) to run the necessary action.  
The operator monitors the outcome of those Pods and records that information.
It uses it to label `Node` objects when the module was successfully loaded, and to run the device plugin (if
configured).

### Worker Pods

Worker pods run the KMM `worker` binary that

- pulls the kmod image configured in the `Module` resource;
- extract it in the Pod's filesystem;
- runs `modprobe` with the right arguments to perform the necessary action.

kmod images are standard OCI images that contains `.ko` files.
Learn more about [how to build a kmod image](kmod_image.md).

### Device plugin

If `.spec.devicePlugin` is configured in a `Module`, then KMM will create a [device plugin](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/device-plugins/)
DaemonSet in the cluster.
That DaemonSet will target nodes:

- that match the `Module`'s `.spec.selector`;
- on which the kernel module is loaded.

There is also support for running an init-container as part of the device-plugin by setting `.spec.devicePlugin.initContainer`.

For example, for some devices, a successful load of the kernel module into the kernel might not constitute the indication of
successful loading. For those devices, the indication is the appearance of the device file under /dev filesystem.
For those cases, using an init-container looping over files verification instead of adding that verification to the device-plugin
image is preferable for debuggability and code re-usage reasons.

## `Module` CRD

The `Module` Custom Resource Definition represents a kernel module that should be loaded on all or select nodes in the
cluster, through a kmod image.
A Module specifies one or more kernel versions it is compatible with, as well as a node selector.

The compatible versions for a `Module` are listed under `.spec.moduleLoader.container.kernelMappings`.
A kernel mapping can either match a `literal` version, or use `regexp` to match many of them at the same time.

The reconciliation loop for `Module` runs the following steps:

1. list all nodes matching `.spec.selector`;
2. build a set of all kernel versions running on those nodes;
3. for each kernel version:
    1. go through `.spec.moduleLoader.container.kernelMappings` and find the appropriate container image name.
       If the kernel mapping has `build` or `sign` defined and the container image does not already exist, run the build
       and / or signing pod as required;
    2. create a worker Pod pulling the container image determined at the previous step and running `modprobe`;
    3. if `.spec.devicePlugin` is defined, create a device plugin `DaemonSet` using the configuration specified under
       `.spec.devicePlugin.container`;
4. garbage-collect:
    1. obsolete device plugin `DaemonSets` that do not target any node;
    2. successful build pods;
    3. successful signing pods.

### Soft dependencies between kernel modules

Some setups may require that several kernel modules be loaded in a specific order to work properly, although the modules
do not directly depend on each other through symbols.
`depmod` is usually not aware of those dependencies, and they do not appear in the files it produces.  
If `mod_a` has a soft dependency on `mod_b`, `modprobe mod_a` will not load `mod_b`.

Soft dependencies can be declared in the `Module` CRD via the
`.spec.moduleLoader.container.modprobe.modulesLoadingOrder` field:

```yaml
modulesLoadingOrder:  # optional
  - mod_a
  - mod_b
```

With the configuration above:

- the loading order will be `mod_b`, then `mod_a`;
- the unloading order will be `mod_a`, then `mod_b`.

The first value in the list, to be loaded last, must be equivalent to the `moduleName`.

### Replacing an in-tree module

Some modules loaded by KMM may replace in-tree modules already loaded on the node.  
To unload an in-tree module before loading your module, set the `.spec.moduleLoader.container.inTreeModuleToRemove`:

```yaml
spec:
  moduleLoader:
    container:
      modprobe:
        moduleName: mod_a
        # ...

      # Other fields removed for brevity
      inTreeModuleToRemove: mod_b
```

The worker Pod will first try to unload the in-tree `mod_b` before loading `mod_a` from the kmod image.  
When the worker Pod is terminated and `mod_a` is unloaded, `mod_b` will not be loaded again.

### Supporting Modules without OOT kmods
In some cases, there is a need to configure the KMM Module to avoid loading an out-of-tree kernel module and
instead use the in-tree one, running only the device plugin.
In such cases, the moduleLoader can be omitted from the Module custom resource, leaving only the devicePlugin section.

```yaml
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: my-kmod
spec:
  selector:
    node-role.kubernetes.io/worker: ""
  devicePlugin:
    container:
      image: some.registry/org/my-device-plugin:latest
```

### Example resource

Below is an annotated `Module` example with most options set.
More information about specific features is available in the dedicated pages:

- [in-cluster builds](kmod_image.md#building-in-cluster)
- [kernel module signing](secure_boot.md)

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

        dirName: /opt  # Optional

        # Optional. Will copy /firmware/* on the node into the path specified
        # in the `kmm-operator-manager-config` at `worker.setFirmwareClassPath`
        # before `modprobe` is called to insert the kernel module..
        firmwarePath: /firmware
        
        parameters:  # Optional
          - param=1

        modulesLoadingOrder:  # optional
          - my-kmod
          - my_dep_a
          - my_dep_b

      imagePullPolicy: Always  # optional

      inTreeModuleToRemove: my-kmod-intree  # optional

      kernelMappings:  # At least one item is required
        - literal: 5.14.0-70.58.1.el9_0.x86_64
          containerImage: some.registry/org/my-kmod:5.14.0-70.58.1.el9_0.x86_64

        # For each node running a kernel matching the regexp below,
        # KMM will create a DaemonSet running the image specified in containerImage
        # with ${KERNEL_FULL_VERSION} replaced with the kernel version.
        - regexp: '^.+\el9\.x86_64$'
          containerImage: "some.other.registry/org/my-kmod:${KERNEL_FULL_VERSION}"

        # For any other kernel, build the image using the Dockerfile in the my-kmod ConfigMap.
        - regexp: '^.+$'
          containerImage: "some.registry/org/my-kmod:${KERNEL_FULL_VERSION}"
          inTreeModuleToRemove: my-other-kmod-intree  # optional
          build:
            buildArgs:  # Optional
              - name: ARG_NAME
                value: some-value
            secrets:  # Optional
              - name: some-kubernetes-secret  # Will be available in the build environment at /run/secrets/some-kubernetes-secret.
            baseImageRegistryTLS:
              # Optional and not recommended! If true, the build will be allowed to pull the image in the Dockerfile's
              # FROM instruction using plain HTTP.
              insecure: false
              # Optional and not recommended! If true, the build will skip any TLS server certificate validation when
              # pulling the image in the Dockerfile's FROM instruction using plain HTTP.
              insecureSkipTLSVerify: false
            dockerfileConfigMap:  # Required
              name: my-kmod-dockerfile
          sign:
            certSecret:
              name: cert-secret  # Required
            keySecret:
              name: key-secret  # Required
            filesToSign:
              - /opt/lib/modules/${KERNEL_FULL_VERSION}/my-kmod.ko
          registryTLS:
            # Optional and not recommended! If true, KMM will be allowed to check if the container image already exists
            # using plain HTTP.
            insecure: false
            # Optional and not recommended! If true, KMM will skip any TLS server certificate validation when checking if
            # the container image already exists.
            insecureSkipTLSVerify: false

    serviceAccountName: sa-module-loader  # Optional

  devicePlugin:  # Optional
    container:
      image: some.registry/org/device-plugin:latest  # Required if the devicePlugin section is present

      env:  # Optional
        - name: MY_DEVICE_PLUGIN_ENV_VAR
          value: SOME_VALUE

      volumeMounts:  # Optional
        - mountPath: /some/mountPath
          name: device-plugin-volume

    volumes:  # Optional
      - name: device-plugin-volume
        configMap:
          name: some-configmap

    serviceAccountName: sa-device-plugin  # Optional

  imageRepoSecret:  # Optional. Used to pull kmod and device plugin images
    name: secret-name

  selector:
    node-role.kubernetes.io/worker: ""
```

#### Variable substitution

The following `Module` fields support shell-like variable substitution:

- `.spec.moduleLoader.container.containerImage`;
- `.spec.moduleLoader.container.kernelMappings[*].containerImage`;
- `.spec.moduleLoader.container.sign.filesToSign`;
- `.spec.moduleLoader.container.kernelMappings[*].sign.filesToSign`;

The following variables will be substituted:

| Name                  | Description                            | Example                       |
|-----------------------|----------------------------------------|-------------------------------|
| `KERNEL_FULL_VERSION` | The kernel version we are building for | `5.14.0-70.58.1.el9_0.x86_64` |
| `MOD_NAME`            | The `Module`'s name                    | `my-mod`                      |
| `MOD_NAMESPACE`       | The `Module`'s namespace               | `my-namespace`                |

### Unloading the kernel module

To unload a module loaded with KMM from nodes, simply delete the corresponding `Module` resource.
KMM will then create worker Pods where required to run `modprobe -r` and unload the kernel module from nodes.

!!! warning
    To create unloading worker Pods, KMM needs all the resources it used when loading the kernel module.
    This includes the `ServiceAccount` that are referenced in the `Module` as well as any RBAC you may have defined to
    allow privileged KMM worker Pods to run.
    It also includes any pull secret referenced in `.spec.imageRepoSecret`.  
    To avoid situations where KMM is unable to unload the kernel module from nodes, make sure those resources are not
    deleted while the `Module` resource is still present in the cluster in any state, including `Terminating`.  
    KMM ships with a validating admission webhook that rejects the deletion of namespaces that contain at least one
    `Module` resource.

### Kernel modules events on Nodes
Due to an event anti-spam mechanism embedded in Kubernetes,
some events may not necessarily be shown when loading or unloading kernel modules in quick succession.

## Security and permissions

Loading kernel modules is a highly sensitive operation.
Once loaded, kernel modules have all possible permissions to do any kind of operation on the node.

### `ServiceAccounts` and `SecurityContextConstraints`

[Pod Security admission](https://docs.openshift.com/container-platform/4.12/authentication/understanding-and-managing-pod-security-admission.html) and `SecurityContextConstraints` (SCCs) restrict privileged workload in most namespaces by default;
namespaces that are part of the [cluster payload](https://docs.openshift.com/container-platform/4.12/authentication/understanding-and-managing-pod-security-admission.html#security-context-constraints-psa-opting_understanding-and-managing-pod-security-admission)
are an exception to that rule.
In namespaces where Pod Security admission and SCC synchronization are enabled, the KMM workload needs to be manually
allowed through RBAC.
This is done by configuring a ServiceAccount that is allowed to use the `privileged` SCC in the `Module`.
The authorization model depends on the `Module`'s namespace, as well as its spec:

- if the `.spec.moduleLoader.serviceAccountName` or `.spec.devicePlugin.serviceAccountName` fields are set, they are
  always used;
- if those fields are not set, then:
    - if the `Module` is created in the operator's namespace (`openshift-kmm` by default), then KMM will use its
      default, powerful `ServiceAccounts` to run the worker and device plugin Pods;
    - if the `Module` is created in any other namespace, then KMM will run the Pods with the namespace's `default`
      `ServiceAccount`, which cannot run privileged workload unless you manually allow it to use the `privileged` SCC.

!!! warning "`openshift-kmm` and some other namespaces that are part of the [cluster payload](https://docs.openshift.com/container-platform/4.12/authentication/understanding-and-managing-pod-security-admission.html#security-context-constraints-psa-opting_understanding-and-managing-pod-security-admission) are considered trusted namespaces"

    When setting up RBAC permissions, keep in mind that any user or ServiceAccount creating a `Module` resource in the
    `openshift-kmm` namespace will result in KMM automatically running privileged workload on potentially all nodes in
    cluster.

To allow any `ServiceAccount` to use the `privileged` SCC and hence to run worker and / or device plugin Pods, use the
following command:

```shell
oc adm policy add-scc-to-user privileged -z "${serviceAccountName}" [ -n "${namespace}" ]
```

### Pod Security standards

OpenShift runs a [synchronization mechanism](https://docs.openshift.com/container-platform/4.12/authentication/understanding-and-managing-pod-security-admission.html)
that sets the namespace's Pod Security level automatically based on the security contexts in use.
No action is needed.
