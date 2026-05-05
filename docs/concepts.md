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
  such as QEMU/KVM hosts, Redfish emulation, bare-metal BMC defaults, or
  provider credentials.
- `ClusterInfrastructure`: one cluster's realised infrastructure on a
  provider: networks, machines, endpoints, managed load balancers, and managed
  name resolution.
- `OCPCluster`: provider-neutral OpenShift intent: hub or managed role,
  topology, install method, cluster networking, and node identity.

## Provider Swap

Provider-specific facts stay out of `OCPCluster`. Moving a cluster from the
QEMU/Redfish lab example to real bare metal changes only
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

## Hub And Managed Clusters

The hub runs ACM and OpenShift GitOps. Managed clusters are workload clusters
whose intent is published to the hub-watched Git repository.

## Phases

`gitups apply <scope>` runs idempotent phases through explicit workflow scopes:

1. `infra`: provider infrastructure, BMC emulation, load balancing, DNS or host
   name resolution.
2. `hub`: hub OpenShift install plus hub-side operators.
3. `clusters`: reserved for managed-cluster GitOps/ACM publication through the
   hub; the command exists but returns a not-implemented status until that
   workflow is real.

## Rendered Output

Rendering is internal to `plan`, `preflight`, `apply`, and `status --diff`.
Gitups writes deterministic output under `--state-dir`, including effective
state, installer assets, Ansible inventory and variables, and the embedded
Ansible bundle.

## Secrets

Desired state references secret names. Secret bytes live outside the repo under
`<gitups-home>/secrets` by default. See
[`/specs/security.md`](../specs/security.md).
