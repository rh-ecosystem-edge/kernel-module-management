# Kernel Module Management Operator

The Kernel Module Management Operator manages the deployment of out-of-tree kernel modules and
associated device plug-ins in Kubernetes. Along with deployment it also manages the lifecycle of
the kernel modules for new incoming kernel versions attached to upgrades.

[![Go Report Card](https://goreportcard.com/badge/github.com/rh-ecosystem-edge/kernel-module-management)](https://goreportcard.com/report/github.com/rh-ecosystem-edge/kernel-module-management)
[![codecov](https://codecov.io/gh/rh-ecosystem-edge/kernel-module-management/branch/main/graph/badge.svg?token=OMIRXMN03W)](https://codecov.io/gh/rh-ecosystem-edge/kernel-module-management)
[![Go Reference](https://pkg.go.dev/badge/github.com/rh-ecosystem-edge/kernel-module-management.svg)](https://pkg.go.dev/github.com/rh-ecosystem-edge/kernel-module-management)

## Getting started
Install the bleeding edge KMMO in one command:
```shell
kubectl apply -k https://github.com/rh-ecosystem-edge/kernel-module-management/config/default
```
