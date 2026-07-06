# CLAUDE.md — Kernel Module Management Operator

## Project Overview

KMM is an OpenShift operator that manages out-of-tree kernel modules and associated
device plugins or DRA resources. It handles the full lifecycle:
building kernel module images in-cluster, signing them for Secure Boot, deploying worker pods
that load kernel modules on nodes, and managing upgrades across kernel version changes.

The operator runs in two modes:
- **Spoke** — standalone operator managing modules on a single cluster
- **Hub** — multi-cluster orchestrator using ACM (Advanced Cluster Management) ManifestWorks
  to distribute module configurations to spoke clusters

**Module path:** `github.com/rh-ecosystem-edge/kernel-module-management`

## Quick Reference

Before committing, always run:
```bash
make generate manifests bundle bundle-hub fmt lint unit-test
```

Build and deploy to a connected cluster:
```bash
IMG=localhost/kmm:$(date +%Y%m%d-%H%M%S) make docker-build docker-push deploy
```

For the hub operator, use `docker-build-hub`, `docker-push-hub`, `deploy-hub` targets instead.

Run `make help` for a full list of targets.

Dependencies are vendored — after changing `go.mod`, run `go mod tidy && go mod vendor`.

PRs must be a single commit - CI enforces this.

## Code Layout

- **CRD API types**: `api/` (spoke), `api-hub/` (hub)
- **Controllers**: `internal/controllers/` (spoke), `internal/controllers/hub/` (hub)
- **Webhooks**: `internal/webhook/` (spoke), `internal/webhook/hub/` (hub) — served by `cmd/webhook-server`
- **Binary entry points**: `cmd/` (manager, manager-hub, webhook-server, worker, day1-utility)
- **Kustomize manifests**: `config/`
- **OLM bundles**: `bundle/` (spoke), `bundle-hub/` (hub)

## Architecture & Design Patterns

- **Separate webhook process**: The webhook server runs independently from the controller manager
  for isolation and independent scaling
- **Worker pods**: Module loading happens via short-lived worker pods (not DaemonSets) managed
  by `NodeModulesConfig`, allowing fine-grained per-node control
- **Hub-spoke via ManifestWork**: Multi-cluster support uses ACM ManifestWorks rather than
  direct API calls, leveraging ACM's delivery guarantees
- **Day-1 boot-time modules**: `BootModuleConfig` + `day1-utility` enable kernel module loading
  at boot via MachineConfig, before the operator runs - see `docs/mkdocs/day1_limited_option.md`
- **Preflight validation**: Validates module compatibility against a target kernel *before*
  an upgrade happens, preventing broken nodes
- **Ordered upgrade**: Coordinates kernel module upgrades across nodes - see `docs/mkdocs/ordered_upgrade.md`

## Testing

- **Ginkgo v2** + **Gomega** for BDD-style test specs
- Every package has a `suite_test.go` bootstrapping Ginkgo and co-located `*_test.go` files
- All business logic interfaces have `//go:generate mockgen` directives producing `mock_*.go` files — run `make generate` to regenerate
- No `envtest` — all unit tests use mocked interfaces
- E2E tests are in `ci/e2e/` and `ci/e2e-hub/` (run by Prow, not locally)

## CI/CD

CI runs on **Prow** (OpenShift CI). Job definitions live in `ci/prow/`.
E2E tests run on real OpenShift clusters and are not meant to be run locally.
**gitStream** (GitHub Actions) syncs changes from upstream `kubernetes-sigs/kernel-module-management`.

## Generated Files — Never Edit by Hand
- `api/**/zz_generated.deepcopy.go`
- `internal/**/mock_*.go`
- `config/crd/bases/*.yaml`
- `config/rbac/role.yaml`
- `bundle/` and `bundle-hub/` contents
