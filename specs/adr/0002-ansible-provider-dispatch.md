# ADR 0002: Layered Ansible Bundle and Provider Dispatch

## Status

Accepted

## Context

ADR 0001 fixes the desired-state API as four layers with structural provider
discriminators (`InfrastructureProvider.spec.machine.{libvirt | baremetal |
vsphere | kubevirt}`). The render layer compiles those discriminators into
Ansible vars, and the orchestration layer acts on them.

The initial Ansible layout grew from the libvirt + emulated-BMC lab. A flat
`roles/` directory made new-reader navigation harder because role names had
to carry both layer and concern, while playbooks also held repeated context
selection and some resource teardown logic.

## Decision

The Ansible bundle is organized by Bootwright layers:

```text
ansible/playbooks/
  targets/       public CLI target wrappers
  layers/        executable layer workflows
  checks/        read-only Ansible checks
ansible/roles/
  bastion/       controller-local setup
  shared/        context and host helper roles
  providers/     provider-scoped shared services
  cluster_infra/ per-cluster substrate and network state
  openshift/     openshift-install agent workflows
```

Target playbooks are thin wrappers. `targets/infra/apply.yml` imports
`layers/providers/apply.yml` and then `layers/cluster_infra/apply.yml`;
destroy runs cluster infrastructure before provider-scoped services.
`targets/clusters/apply.yml` imports the OpenShift install layer.

Role dispatch remains computed from rendered provider vars, but dynamic role
names are local to their layer:

- `provider.substrateRole` selects `substrate_<role>` from
  `roles/cluster_infra/`.
- `provider.bmcRole` selects `bmc_<role>` from `roles/providers/` and
  `boot_<role>` from `roles/openshift/`.
- `provider.bootArtifactsHttp.{enabled,bindAddress,port}` gates
  `boot_artifacts_http`.

The kind-to-role mapping lives in one Go switch (`render.providerDispatch`).
Every kind resolves to a real role; vSphere and OpenShift Virtualization use
explicit no-op roles (`bmc_none`, `boot_none`) so dispatch never fails to
resolve.

Shared context roles (`context_provider`, `context_cluster`) own the
host-local selection of `bootwright_current_provider`,
`bootwright_current_cluster`, and related runtime views. They also publish
structured context facts (`bootwright_provider_ctx`, `bootwright_cluster_ctx`) while
preserving the existing facts consumed by current roles.

Provider roles own provider-scoped apply and destroy logic. Cluster
infrastructure roles own per-cluster substrate and network state. OpenShift
roles own installer execution and BMC boot handoff only.

## Consequences

- Public CLI commands stay stable: `bootwright check/apply bastion|infra|clusters|all`.
- Adding a provider still requires a substrate role, a BMC role or no-op, an
  OpenShift boot role or no-op, and one `render.providerDispatch` case.
- The embedded bundle passes multiple role search paths to Ansible instead of
  one flat `roles/` directory.
- Playbooks become easier to scan because workflows are expressed by layer
  imports and role calls, while resource details live in owning roles.
