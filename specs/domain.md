# Domain Spec

## Mission

Gitups is a desired-state orchestrator for OpenShift cluster fleets. The
user declares what the platform should look like; Gitups validates the
declaration and renders tool-specific inputs for `openshift-install`, `oc`,
`kubectl`, Ansible, and GitOps.

## Operating Model

- Initial implementation runs `openshift-install agent` against the
  cluster nodes for every `OCPCluster` in the desired state.
- Initial workload clusters are bare metal; the lab emulates BMC-managed
  bare metal with libvirt-managed virtual machines and a Redfish emulator.
- vSphere, OpenShift Virtualization, and additional bare-metal stacks are
  first-class targets and must remain reachable through the same
  desired-state API.
- Forward-looking architecture leaves room for one cluster to host ACM
  and OpenShift GitOps and reconcile additional clusters as fleet GitOps
  content; that publication path is not implemented today.

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

The schema must keep room for multi-cluster GitOps publication topologies,
additional providers, and disconnected or restricted-network environments.
Architecture choices that would lock the project into single-node-SNO-plus-
bare-metal are rejected.
