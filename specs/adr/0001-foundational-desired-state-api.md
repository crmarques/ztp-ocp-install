# ADR 0001: Foundational Desired State API

## Status

Accepted

## Context

Gitups needs one user-authored desired-state contract that survives provider
changes. The user wants to author the OpenShift cluster intent once and have
it apply unchanged whether the cluster runs on libvirt (with emulated BMC),
on real bare metal, on vSphere, or on OpenShift Virtualization.

A schema that mixes infrastructure facts (VIPs, load-balancer placement, DNS
placement, provider hosts) into OpenShift intent forces the user to edit
cluster intent every time the substrate changes, and forces shared install
defaults to be copy-pasted across every cluster file.

`OCPCluster` is the user-authored kind that captures OpenShift cluster
intent. It must not grow into a complete copy of the OpenShift installer
schema, but it must still let users reach installer-native fields when
needed. OpenShift agent installs consume `install-config.yaml` and
`agent-config.yaml`; Gitups renders those files instead of inventing a
parallel install contract.

## Decision

The desired-state API is four domain layers. Each layer is one user-authored
kind under `apiVersion: gitups.io/v1alpha1`. The layer above references the
layer below by name; no layer copies facts from the layer it references.

| Layer | Kind | Owns |
| --- | --- | --- |
| Global UX | `Environment` | base domain, OpenShift install mode (`ocpInstallType`), shared secret sources, OpenShift release defaults, component image pins |
| Substrate | `InfrastructureProvider` | shared host pool, capability sub-blocks (`machine` / `loadBalancer` / `nameResolution`) — each independently optional |
| Cluster infra | `ClusterInfrastructure` | provider composition (`providerRefs` list), machines (with substrate-typed placement), per-cluster networks (with substrate-typed sub-blocks), endpoints (api / api-int / ingress with VIPs), load-balancer endpoint binds |
| Cluster intent | `OCPCluster` | role, topology, install method/overrides, networking (clusterNetwork / serviceNetwork), OCP node identity |

`InfrastructureProvider` is **capability-oriented**. Capabilities
(`machine`, `loadBalancer`, `nameResolution`) are independently optional
sub-blocks; a provider declares only what it supplies. The host pool
(`spec.hosts`) is a shared resource referenced by capabilities that need
SSH-reachable Linux hosts.

Per-cluster network instances live on `ClusterInfrastructure` because they
exist *for a particular cluster*. The pattern mirrors machine placement: a
structural sub-block on `ClusterInfrastructure.spec.networks.<name>.{libvirt
| vsphere | …}` realises the network on the substrate.

`ClusterInfrastructure.spec.providerRefs` is a list. A cluster may compose
machines from one provider and load balancing from another — the union of
referenced providers' capabilities must contribute at most one supplier per
capability. This preserves R1: an LB-only `InfrastructureProvider` (e.g.
BigIP, future) can be added without editing the cluster's machine provider.

`OCPCluster` is provider-agnostic. Swapping libvirt with emulated BMC for
real bare metal — or for vSphere — touches `InfrastructureProvider` and
`ClusterInfrastructure` only; `Environment` and `OCPCluster` files are
byte-identical across the swap. CI asserts this invariant by diffing the
`OCPCluster` and `Environment` files across the canonical provider examples.

`OCPCluster` is a thin wrapper around generated installer assets. Gitups
renders complete installer files under
`<state-dir>/clusters-bootstrap.git/<cluster>/openshift/`. Gitups-owned fields are derived
from `Environment`, `InfrastructureProvider`, and `ClusterInfrastructure` —
that includes cluster name, base domain, VIPs, machine networks, host roles,
host interfaces, per-host NMState, root device hints, image digest mirrors,
secret references, and agent-install boot artifact wiring (minimal-ISO
selection and provider-local `bootArtifactsBaseURL` for emulated-BMC labs).
Users do not toggle those fields on `OCPCluster`.

`OCPCluster.spec.install` exposes typed fields for installer-native data
that users frequently set. Beyond those, users supply `installConfigOverrides`
and `agentConfigOverrides` for fields Gitups does not model. Overrides whose
path collides with a Gitups-owned field are rejected at validation time, and
`agentConfigOverrides.hosts` is rejected wholesale — host-level identity
flows through `ClusterInfrastructure`.

Three permanent rules govern the schema:

- **R1 Layered objects.** Replacing one layer's object must not require
  editing files in other layers.
- **R2 Reference, don't repeat.** A fact defined inside one object is
  referenced by name from others; never copied inline.
- **R3 Structural discriminators only.** Where one of N typed sub-blocks
  may be present, the presence of the sub-block is the discriminator. No
  `type`, `mode`, or `kind` discriminator string sits beside the sub-block.
  This applies to `InfrastructureProvider.spec.machine.{libvirt | baremetal
  | vsphere | kubevirt}`, `InfrastructureProvider.spec.loadBalancer.{haProxy
  | …}`, `InfrastructureProvider.spec.nameResolution.{hostsFile | …}`, the
  host connection sub-block on `InfrastructureProvider.spec.hosts.<name>.{ssh
  | …}`, per-machine placement and per-network realisation on
  `ClusterInfrastructure`, and `Environment.spec.ocpInstallType: {connected |
  disconnected}` paired with optional top-level `spec.proxy` and
  `spec.registries`. Capability sub-blocks themselves
  (`machine`, `loadBalancer`, `nameResolution`) are *independently optional*
  rather than a discriminated union — omission means the provider does not
  supply that capability.

## Consequences

- The loader rejects unknown kinds and unknown fields. Until promotion to
  `v1beta1`, breaking changes do not require migrations, aliases, or
  compatibility shims.
- Validation enforces "exactly one of {…}" structurally on every
  discriminated location. Shape and ownership rules are enumerated in
  [`state-model.md`](../state-model.md).
- The renderer reads VIPs and load-balancer bindings from
  `ClusterInfrastructure.spec.endpoints` and
  `ClusterInfrastructure.spec.loadBalancers`; `OCPCluster` carries no
  endpoint addresses.
- The normalizer applies `Environment` defaults onto each `OCPCluster`
  before validation, so per-cluster overrides remain available without
  duplicating shared facts.
- Promotion to `v1beta1` is gated on stable spec coverage of a real
  multi-provider deployment; no compatibility shims will be added before
  promotion.
