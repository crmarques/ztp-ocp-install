---
title: Gitups
---

# Gitups

Gitups provisions OpenShift fleets from declared desired state. You write a
small set of YAML resources; the CLI validates them, renders deterministic
artifacts, and applies ordered phases for infrastructure, hub bootstrap, and
GitOps publication.

## Choose A Path

| Goal | Read |
| --- | --- |
| Run the single-host QEMU/Redfish lab | [Quickstart](quickstart.md) |
| Learn the main terms | [Concepts](concepts.md) |
| Write your own YAML | [Desired State](desired-state.md) |

The docs are intentionally practical. The binding contracts live in
[`/specs/`](../specs/index.md), including the desired-state schema, CLI
surface, security rules, adapter boundaries, and ADRs.

## Current Shape

- Hub: OpenShift single-node hub.
- Managed clusters: declared as fleet intent and reconciled by the hub through
  ACM and OpenShift GitOps.
- Lab provider: QEMU/KVM nodes with Redfish BMC emulation.
- Provider boundary: real bare metal, vSphere, and OpenShift Virtualization
  remain first-class targets behind the same desired-state model.
