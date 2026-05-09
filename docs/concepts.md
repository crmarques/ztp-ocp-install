---
title: Concepts
---

# Concepts

Short definitions for reading examples and command output. The full contract is
in [`/specs/`](../specs/index.md).

## Desired State

The YAML you author. It uses `apiVersion: gitups.io/v1alpha1` and is the only
source of declared platform intent. Generated files are outputs, not edit
points.

## The Four Kinds

- `Environment`: shared environment defaults such as base domain,
  OpenShift install mode, secret refs, OpenShift release, and component
  image pins.
- `InfrastructureProvider`: provider connections and reusable capabilities,
  such as libvirt hosts, Redfish emulation, bare-metal BMC defaults, or
  provider credentials.
- `ClusterInfrastructure`: one cluster's realised infrastructure on a
  provider: networks, machines, endpoints, managed load balancers, and managed
  name resolution.
- `OCPCluster`: provider-neutral OpenShift intent: topology, install method,
  cluster networking, and node identity.

## Provider Swap

Provider-specific facts stay out of `OCPCluster`. Moving a cluster from the
libvirt/Redfish lab example to real bare metal changes only
`InfrastructureProvider` and `ClusterInfrastructure`.

## OCP Install Mode

`Environment.spec.ocpInstall` selects how the OpenShift install reaches its
release content and supporting registries. It scopes only OpenShift install
material; it does not describe the lab host's substrate connectivity. One
structural selection:

- `connected: {}` for public registry access. This is the default when
  `ocpInstall` is omitted.
- `restricted: { ... }` for constrained egress with optional proxy, mirror, or
  trust material.
- `disconnected: { ... }` for local mirror usage with required registry mirror
  and trust material.

There is no `mode` field.

## Future: Multi-Cluster Topology

Forward-looking architecture leaves room for one cluster to host ACM and
OpenShift GitOps and reconcile additional clusters whose intent is published
as fleet GitOps content. That publication path is not implemented today;
every `OCPCluster` in the desired state is installed locally via the agent
installer.

## Scopes

`gitups <scope> apply` runs idempotent phases through explicit scopes:

1. `bastion`: controller-local dependencies (managed Ansible venv, OCP CLIs).
2. `provider`: provider infrastructure plus per-cluster substrate
   (`InfrastructureProvider` services and `ClusterInfrastructure` instances).
3. `clusters`: openshift-install agent against the cluster nodes plus per-cluster
   install state.
4. `hub` *(reserved)*: hub-cluster components for clusters declaring
   `role: hub`. Not implemented yet.

Each scope also exposes `<scope> check` (local + Ansible read-only checks)
and `<scope> destroy` (reverse).

## Rendered Output

Rendering is internal to every `<scope> check`, `<scope> apply`, and
`<scope> destroy`. Gitups writes deterministic output under `--state-dir`,
including effective state, installer assets, Ansible inventory and variables,
and the embedded Ansible bundle.

## Secrets

Desired state references secret names. Secret bytes live outside the repo under
`<gitups-user-dir>/secrets` by default. See
[`/specs/security.md`](../specs/security.md).
