# Hub & Spoke

In Hub & Spoke scenarios, many Spoke clusters are connected to a central, powerful Hub cluster.
KMM depends on
[Red Hat Advanced Cluster Management (RHACM)](https://www.redhat.com/en/technologies/management/advanced-cluster-management)
to operate in those scenarios.

## On the Hub

Red Hat ships KMM-Hub, an edition of KMM dedicated to Hub clusters.
KMM-Hub is aware of all kernel versions running on the Spokes and determines which node on which cluster should receive
a kernel module.  
It runs all compute-intensive tasks such as image builds and kmod signing, and prepares trimmed-down `Module` to be
transferred to the Spokes via RHACM.  
KMM-Hub cannot be used to load kernel modules on the Hub cluster.
To do that, [install the regular edition of KMM](./install.md).

### Installing KMM-Hub

#### With OLM (recommended)

KMM-Hub is available to install from the Red Hat catalog.

The preferred way to install KMM-Hub is to use the Operators section of the OpenShift console.

If you want to install KMM-Hub programmatically, you can use the resources below to create the `Namespace`,
`OperatorGroup` and `Subscription` resources.

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: openshift-kmm-hub
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: kernel-module-management-hub
  namespace: openshift-kmm-hub
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: kernel-module-management-hub
  namespace: openshift-kmm-hub
spec:
  channel: stable
  installPlanApproval: Automatic
  name: kernel-module-management-hub
  source: redhat-operators
  sourceNamespace: openshift-marketplace
```

#### With `oc`

The command below installs the bleeding edge version of KMM-Hub.

```shell
oc apply -k https://github.com/rh-ecosystem-edge/kernel-module-management/config/default-hub
```

### The `ManagedClusterModule` CRD

The `ManagedClusterModule` CRD is used to configure the deployment of kernel modules on Spoke clusters.
It is cluster-scoped, wraps a `Module` spec and adds a few additional fields:

```yaml
apiVersion: hub.kmm.sigs.x-k8s.io/v1beta1
kind: ManagedClusterModule
metadata:
  name: my-mcm
  # No namespace, because this resource is cluster-scoped.
spec:
  moduleSpec:
    # Contains moduleLoader and devicePlugin sections, just like in a Module resource.
    selector:
      node-wants-my-mcm: 'true'  # Selects nodes within the ManagedCluster.

  spokeNamespace: some-namespace  # Specifies in which namespace the Module should be created

  selector:
    wants-my-mcm: 'true'  # Selects ManagedCluster objects
```

If build or signing instructions are present under `.spec.moduleSpec`, those jobs are run on the Hub cluster in the
operator's namespace.  
When the `.spec.selector` matches one or more `ManagedCluster` resources, then KMM-Hub creates a `ManifestWork` resource
in the corresponding namespace(s).
The `ManifestWork` contains a trimmed-down `Module` resource, with kernel mappings preserved but all `build` and `sign`
subsections removed.
`containerImage` fields that contain image names ending with a tag are replaced with their digest equivalent.

## On the Spokes

After the installation of KMM on the Spoke, no further action is required.
Create `ManagedClusterModule` from the Hub to deploy kernel modules on Spoke clusters.

### Running KMM on the Spoke

KMM can be installed on the Spokes cluster through a RHACM `Policy` object.
In addition to installing KMM from the Red Hat catalog and running it in a lightweight Spoke mode, the `Policy`
configures additional RBAC required for the RHACM agent to be able to manage `Module` resources.

<details>
<summary>RHACM `Policy` to install KMM on Spoke clusters</summary>

```yaml
---
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: install-kmm
spec:
  remediationAction: enforce
  disabled: false
  policy-templates:
    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: install-kmm
        spec:
          severity: high
          object-templates:
          - complianceType: mustonlyhave
            objectDefinition:
              apiVersion: v1
              kind: Namespace
              metadata:
                name: openshift-kmm
          - complianceType: mustonlyhave
            objectDefinition:
              apiVersion: operators.coreos.com/v1
              kind: OperatorGroup
              metadata:
                name: kmm
                namespace: openshift-kmm
              spec:
                upgradeStrategy: Default
          - complianceType: mustonlyhave
            objectDefinition:
              apiVersion: operators.coreos.com/v1alpha1
              kind: Subscription
              metadata:
                name: kernel-module-management
                namespace: openshift-kmm
              spec:
                channel: stable
                config:
                  env:
                    - name: KMM_MANAGED
                      value: "1"
                installPlanApproval: Automatic
                name: kernel-module-management
                source: redhat-operators
                sourceNamespace: openshift-marketplace
          - complianceType: mustonlyhave
            objectDefinition:
              apiVersion: rbac.authorization.k8s.io/v1
              kind: ClusterRole
              metadata:
                name: kmm-module-manager
              rules:
                - apiGroups: [kmm.sigs.x-k8s.io]
                  resources: [modules]
                  verbs: [create, delete, get, list, patch, update, watch]
          - complianceType: mustonlyhave
            objectDefinition:
              apiVersion: rbac.authorization.k8s.io/v1
              kind: ClusterRoleBinding
              metadata:
                name: klusterlet-kmm
              subjects:
              - kind: ServiceAccount
                name: klusterlet-work-sa
                namespace: open-cluster-management-agent
              roleRef:
                kind: ClusterRole
                name: kmm-module-manager
                apiGroup: rbac.authorization.k8s.io
---
apiVersion: apps.open-cluster-management.io/v1
kind: PlacementRule
metadata:
  name: all-managed-clusters
spec:
  clusterSelector:
    matchExpressions: []
---
apiVersion: policy.open-cluster-management.io/v1
kind: PlacementBinding
metadata:
  name: install-kmm
placementRef:
  apiGroup: apps.open-cluster-management.io
  kind: PlacementRule
  name: all-managed-clusters
subjects:
  - apiGroup: policy.open-cluster-management.io
    kind: Policy
    name: install-kmm
```

The `spec.clusterSelector` field can be customized at will to target select clusters only.
</details>
