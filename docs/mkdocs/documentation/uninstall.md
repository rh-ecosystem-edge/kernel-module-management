# Uninstalling

## If installed from the Red Hat catalog

To uninstall KMM, either use the OpenShift console under "Operators" --> "Installed Operators" (preferred), or delete
the `Subscription` resource in the KMM namespace.

## If installed using `oc`

```shell
oc delete -k https://github.com/rh-ecosystem-edge/kernel-module-management/config/default
```
