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
