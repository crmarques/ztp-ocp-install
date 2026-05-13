---
title: Architecture
---

# Architecture

This document maps the implementation packages and Ansible roles. The binding
rules remain in [`/specs/`](../specs/index.md); this page is a contributor
orientation guide.

## Pipeline

Gitups follows the pipeline in [`specs/architecture.md`](../specs/architecture.md):

```text
desired state -> load -> normalize -> validate -> render -> orchestrate
```

Generated files are runtime artifacts. Users edit desired-state YAML,
GitOps package sets, and package catalogs, not generated inventory, vars,
installer files, locks, or state output.

## Go Packages

| Path | Responsibility |
| --- | --- |
| `api/v1alpha1/` | Versioned desired-state and GitOps schema types. Keep this package close to API shape and simple pure helpers. |
| `cmd/gitups/` | Process entrypoint. |
| `internal/cli/` | Cobra command wiring, flags, prompts, and user-facing output. Workflow and domain behavior should move out as it becomes reusable. |
| `internal/infra/` | Desired-state loading, normalization, validation, and cross-layer state checks. |
| `internal/provisioning/render/` | Deterministic projection into installer inputs, Ansible inventory, Ansible vars, locks, and effective state. |
| `internal/ansible/` | Ansible execution boundary and command construction. |
| `internal/embedded/` | Build-time and runtime Ansible bundle embedding/extraction. |
| `internal/gitops/load/` | GitOpsPackageSet and PackageDefinition loading. |
| `internal/gitops/catalog/` | Package source resolution for filesystem, OCI, and git sources. |
| `internal/gitops/resolve/` | Expansion of package-set intent into resolved package execution plans. |
| `internal/gitops/render/` | GitOps repository tree rendering. |
| `internal/gitops/push/` | Outbound git publication. |
| `internal/gitops/cluster/` | KRC/SRC CLI integration against target clusters. |
| `internal/proxy/` | Effective proxy/no-proxy derivation. |
| `internal/secretref/` | Local secret reference path resolution. |

## Ansible Layout

| Path | Responsibility |
| --- | --- |
| `ansible/playbooks/targets/` | User-facing workflows such as `infra`, `clusters`, and `all`. |
| `ansible/playbooks/layers/` | Layer-level orchestration for providers, cluster infrastructure, and OpenShift installation. |
| `ansible/roles/bastion/` | Controller-local setup. |
| `ansible/roles/shared/` | Context, host, proxy, and platform helper roles. |
| `ansible/roles/providers/` | Provider-scoped services such as BMC emulation, boot-artifact HTTP, mirror registry, managed proxy, and load balancing. |
| `ansible/roles/cluster_infra/` | Per-cluster substrate, network, VIP, and name-resolution state. |
| `ansible/roles/openshift/` | Agent installer, boot, wait, and destroy behavior. |

Playbooks own execution order. Roles own reusable capabilities. Inventory and
variables are always rendered by Go and are not user-maintained source state.

## Boundaries

- Go owns schema parsing, normalization, validation, deterministic rendering,
  workflow planning, and external command contracts.
- Ansible owns idempotent host convergence and long-running provider or
  installer actions.
- Secret bytes stay outside desired-state files, generated GitOps content, and
  logs. Go resolves local secret paths; Ansible reads secret files only when a
  host workflow needs the material.
- Provider-specific behavior must stay behind provider capability blocks,
  render dispatch, and provider role files.

## Rendered Contracts

The provisioning handoff from Go to Ansible is the rendered state directory:

| Artifact | Owner | Consumer |
| --- | --- | --- |
| `effective-state.yaml` | Go | Humans, tests, later workflow steps |
| `gitups.lock.yaml` | Go | Reproducibility and review |
| `ansible/inventory.yaml` | Go | Ansible only |
| `ansible/vars.yaml` | Go | Ansible only |
| `git-repos/clusters-bootstrap/<cluster>/openshift/install-config.yaml` | Go | Reviewable safe installer input (GitOps-publishable) |
| `git-repos/clusters-bootstrap/<cluster>/openshift/agent-config.yaml` | Go | Reviewable safe installer input (GitOps-publishable) |
| `runtime/<cluster>/installer/` | Go or Ansible apply-time materialization | Direct `openshift-install` execution (local only) |

The `git-repos/clusters-bootstrap/` tree is the declarative source intended
for publication to a Git provider; it MUST NOT contain secret bytes, build
artifacts, or installer logs. Apply-time materialization (effective
install/agent configs with resolved secrets, `.openshift_install*` logs
and state, agent ISOs, `auth/`, `boot-artifacts/`, `rendezvousIP`) lives
under a sibling `runtime/` tree with restricted permissions and is never
pushed. `git-repos/` is the umbrella for any additional bootstrap or
publishable repos that may be added later (e.g. fleet-wide GitOps content).

## Extension Points

- Add desired-state fields only after updating the relevant spec.
- Add provider flavors by extending the structural schema, validation,
  render dispatch, and matching Ansible roles.
- Add GitOps source drivers in `internal/gitops/catalog/`.
- Add GitOps renderers in `internal/gitops/render/` when the renderer has a
  clear ownership boundary and deterministic output.
