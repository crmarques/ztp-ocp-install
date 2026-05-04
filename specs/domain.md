# Domain Spec

## Mission

Gitups is a desired-state orchestrator for OpenShift cluster fleets. The
user declares what the platform should look like; Gitups validates the
declaration and renders tool-specific inputs for `openshift-install`, `oc`,
`kubectl`, Ansible, and GitOps.

## Operating Model

- One hub OpenShift cluster (initial assumption: SNO) runs ACM and
  OpenShift GitOps and reconciles managed clusters.
- Initial managed clusters are bare metal; the lab emulates BMC-managed
  bare metal with QEMU/KVM and a Redfish emulator.
- vSphere, OpenShift Virtualization, and additional bare-metal stacks are
  first-class targets and must remain reachable through the same
  desired-state API.

## UX Principles

- The user-authored YAML is the only source of desired state. Generated
  files are never user edit points.
- Swapping `InfrastructureProvider` (and the matching per-machine placement
  on `ClusterInfrastructure`) must leave `Environment` and `OCPCluster`
  files byte-identical.
- Plaintext credentials, kubeconfigs, pull secrets, private keys, and
  tokens are never committed or logged.
- Every generated artifact is reproducible and reviewable.
- Every orchestration step is safe to re-run.

## Extensibility Guardrails

The schema must keep room for multi-hub topologies, additional providers,
and disconnected or restricted-network environments. Architecture choices
that would lock the project into hub-SNO-plus-bare-metal are rejected.
