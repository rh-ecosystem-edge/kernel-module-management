# Installing

### Recommended namespaces

The recommended namespaces for KMM components are listed below.
All installation methods will default to those namespaces.

| Component                      | Namespace           |
|--------------------------------|---------------------|
| Kernel Module Management       | `openshift-kmm`     |
| Kernel Module Management - Hub | `openshift-kmm-hub` |

## Using OLM (recommended)

KMM is available to install from the Red Hat catalog.

The preferred way to install KMM is to use the Operators section of the OpenShift console.

If you want to install Kernel Module Management programmatically, you can use the resources below to create the
`Namespace`, `OperatorGroup` and `Subscription` resources.

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: openshift-kmm
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: kernel-module-management
  namespace: openshift-kmm
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: kernel-module-management
  namespace: openshift-kmm
spec:
  channel: stable
  installPlanApproval: Automatic
  name: kernel-module-management
  source: redhat-operators
  sourceNamespace: openshift-marketplace
```

## Using `oc`

The command below installs the bleeding edge version of KMM.

```shell
oc apply -k https://github.com/rh-ecosystem-edge/kernel-module-management/config/default
```

## OpenShift versions below 4.12

!!! note "KMM is supported on OpenShift 4.12 and above."

Installing KMM on OpenShift 4.11 does not require specific steps.

For versions 4.10 and below, some RBAC adjustments need to be made before you create the `OperatorGroup` and the
`Subscription` objects.  
Because KMM is designed to work with OpenShift's 4.12 security features, you need to create a new
`SecurityContextConstraint` object and to bind it to the operator's `ServiceAccount`.
Those steps need to happen after you have created the `Namespace`, but before you create the `OperatorGroup`, install
through the OpenShift console or run `oc apply`.

<details>
<summary>Additional RBAC for OpenShift 4.10</summary>

Save the content below under `restricted-v2.yml`:

```yaml
---
allowHostDirVolumePlugin: false
allowHostIPC: false
allowHostNetwork: false
allowHostPID: false
allowHostPorts: false
allowPrivilegeEscalation: false
allowPrivilegedContainer: false
allowedCapabilities:
  - NET_BIND_SERVICE
apiVersion: security.openshift.io/v1
defaultAddCapabilities: null
fsGroup:
  type: MustRunAs
groups: []
kind: SecurityContextConstraints
metadata:
  name: restricted-v2
priority: null
readOnlyRootFilesystem: false
requiredDropCapabilities:
  - ALL
runAsUser:
  type: MustRunAsRange
seLinuxContext:
  type: MustRunAs
seccompProfiles:
  - runtime/default
supplementalGroups:
  type: RunAsAny
users: []
volumes:
  - configMap
  - downwardAPI
  - emptyDir
  - persistentVolumeClaim
  - projected
  - secret
```

Run the following commands:
```shell
oc apply -f restricted-v2.yml
oc adm policy add-scc-to-user restricted-v2 -z kmm-operator-controller -n openshift-kmm
```
</details>
