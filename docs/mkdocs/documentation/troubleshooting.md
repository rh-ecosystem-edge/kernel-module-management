# Troubleshooting

## Using `oc adm must-gather`

The `oc adm must-gather` is the preferred way to collect a support bundle and provide debugging information to Red Hat
support.

### For KMM

```shell
export MUST_GATHER_IMAGE=$(oc get deployment -n openshift-kmm kmm-operator-controller-manager -ojsonpath='{.spec.template.spec.containers[?(@.name=="manager")].env[?(@.name=="RELATED_IMAGES_MUST_GATHER")].value}')
oc adm must-gather --image="${MUST_GATHER_IMAGE}" -- /usr/bin/gather
```

### For KMM-Hub

```shell
export MUST_GATHER_IMAGE=$(oc get deployment -n openshift-kmm-hub kmm-operator-hub-controller-manager -ojsonpath='{.spec.template.spec.containers[?(@.name=="manager")].env[?(@.name=="RELATED_IMAGES_MUST_GATHER")].value}')
oc adm must-gather --image="${MUST_GATHER_IMAGE}" -- /usr/bin/gather -u
```

### Different namespace

Use the `-n NAMESPACE` switch to specify a namespace if you installed KMM in a custom namespace.

## Reading operator logs

| Component | Command                                                                         |
|-----------|---------------------------------------------------------------------------------|
| KMM       | `oc logs -fn openshift-kmm deployments/kmm-operator-controller-manager`         |
| KMM-Hub   | `oc logs -fn openshift-kmm-hub deployments/kmm-operator-hub-controller-manager` |

## Building and Signing work but the ModuelLoader isn't running.

In case the `ServiceAccount` used in the `Module` doesn't have enough permissions to run privileged workload,
you may result in a case in which the build and sign jobs are running correctly but the ModuleLoader pod isn't
scheduled.

In this case, you should be able to see a similar error describing the ModuleLoader's `DaemonSet`:
```
$ oc describe ds/<ds name> -n <namespace>
...
Events:
  Type     Reason        Age                   From                  Message
  ----     ------        ----                  ----                  -------
  Warning  FailedCreate  15m (x50 over 3h38m)  daemonset-controller  Error creating: pods "kmm-ci-1a953734332dedbd-" is forbidden: unable to validate against any security context constraint: [provider "anyuid": Forbidden: not usable by user or serviceaccount, spec.volumes[0]: Invalid value: "hostPath": hostPath volumes are not allowed to be used, spec.containers[0].securityContext.runAsUser: Invalid value: 0: must be in the ranges: [1000700000, 1000709999], spec.containers[0].securityContext.seLinuxOptions.level: Invalid value: "": must be s0:c26,c25, spec.containers[0].securityContext.seLinuxOptions.type: Invalid value: "spc_t": must be , spec.containers[0].securityContext.capabilities.add: Invalid value: "SYS_MODULE": capability may not be added, provider "restricted": Forbidden: not usable by user or serviceaccount, provider "nonroot-v2": Forbidden: not usable by user or serviceaccount, provider "nonroot": Forbidden: not usable by user or serviceaccount, provider "hostmount-anyuid": Forbidden: not usable by user or serviceaccount, provider "machine-api-termination-handler": Forbidden: not usable by user or serviceaccount, provider "hostnetwork-v2": Forbidden: not usable by user or serviceaccount, provider "hostnetwork": Forbidden: not usable by user or serviceaccount, provider "hostaccess": Forbidden: not usable by user or serviceaccount, provider "node-exporter": Forbidden: not usable by user or serviceaccount, provider "privileged": Forbidden: not usable by user or serviceaccount]
```

In order to solve this issue, make sure to follow [`ServiceAccounts` and `SecurityContextConstraints`](deploy_kmod.md#serviceaccounts-and-securitycontextconstraints)
