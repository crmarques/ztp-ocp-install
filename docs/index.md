---
title: Gitups
---

# Gitups

Gitups provisions OpenShift clusters from declared desired state. You write a
small set of YAML resources; the CLI validates them, renders deterministic
artifacts, and applies ordered phases for infrastructure preparation and
the openshift-install agent run.

## Choose A Path

| Goal | Read |
| --- | --- |
| Run the single-host libvirt/Redfish lab | [Quickstart](quickstart.md) |
| Learn the main terms | [Concepts](concepts.md) |
| Write your own YAML | [Desired State](desired-state.md) |
| Understand the implementation map | [Architecture](architecture.md) |

The docs are intentionally practical. The binding contracts live in
[`/specs/`](../specs/index.md), including the desired-state schema, CLI
surface, security rules, adapter boundaries, and ADRs.

## Current Shape

- OpenShift cluster install: openshift-install agent against the cluster
  nodes (single-node and multi-node both supported).
- Current apply providers: libvirt-managed virtual machines with Redfish BMC
  emulation and Redfish bare metal.
- Provider boundary: vSphere, OpenShift Virtualization, and IPMI remain
  schema targets behind the same desired-state model, but their apply
  workflows are not implemented yet.
- Future: a hub cluster running ACM and OpenShift GitOps to reconcile
  additional clusters as fleet GitOps content; not implemented yet.
