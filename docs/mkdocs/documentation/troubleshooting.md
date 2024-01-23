# Troubleshooting

## Using `oc adm must-gather`

The `oc adm must-gather` is the preferred way to collect a support bundle and provide debugging information to Red Hat
support.

### For KMM

```shell
export MUST_GATHER_IMAGE=$(oc get deployment -n openshift-kmm kmm-operator-controller -ojsonpath='{.spec.template.spec.containers[?(@.name=="manager")].env[?(@.name=="RELATED_IMAGE_MUST_GATHER")].value}')
oc adm must-gather --image="${MUST_GATHER_IMAGE}" -- /usr/bin/gather
```

### For KMM-Hub

```shell
export MUST_GATHER_IMAGE=$(oc get deployment -n openshift-kmm-hub kmm-operator-hub-controller -ojsonpath='{.spec.template.spec.containers[?(@.name=="manager")].env[?(@.name=="RELATED_IMAGE_MUST_GATHER")].value}')
oc adm must-gather --image="${MUST_GATHER_IMAGE}" -- /usr/bin/gather -u
```

### Different namespace

Use the `-n NAMESPACE` switch to specify a namespace if you installed KMM in a custom namespace.

## Reading operator logs

| Component | Command                                                                 |
|-----------|-------------------------------------------------------------------------|
| KMM       | `oc logs -fn openshift-kmm deployments/kmm-operator-controller`         |
| KMM-Hub   | `oc logs -fn openshift-kmm-hub deployments/kmm-operator-hub-controller` |

## Observing events

### Build & Sign

KMM publishes events whenever it starts a kmod image build or observes its outcome.  
Those events are attached to `Module` objects and are available at the very end of `kubectl describe module`:

```text
$> kubectl describe modules.kmm.sigs.x-k8s.io kmm-ci-a
[...]
Events:
  Type    Reason          Age                From  Message
  ----    ------          ----               ----  -------
  Normal  BuildCreated    2m29s              kmm   Build created for kernel 6.6.2-201.fc39.x86_64
  Normal  BuildSucceeded  63s                kmm   Build job succeeded for kernel 6.6.2-201.fc39.x86_64
  Normal  SignCreated     64s (x2 over 64s)  kmm   Sign created for kernel 6.6.2-201.fc39.x86_64
  Normal  SignSucceeded   57s                kmm   Sign job succeeded for kernel 6.6.2-201.fc39.x86_64
```

### Module load or unload

KMM publishes events whenever it successfully loads or unloads a kernel module on a node.  
Those events are attached to `Node` objects and are available at the very end of `kubectl describe node`:

```text
$> kubectl describe node my-node
[...]
Events:
  Type    Reason          Age    From  Message
  ----    ------          ----   ----  -------
[...]
  Normal  ModuleLoaded    4m17s  kmm   Module default/kmm-ci-a loaded into the kernel
  Normal  ModuleUnloaded  2s     kmm   Module default/kmm-ci-a unloaded from the kernel
```
