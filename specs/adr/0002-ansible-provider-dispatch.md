# ADR 0002: Ansible Provider Dispatch

## Status

Accepted

## Context

ADR 0001 fixes the desired-state API as four layers with structural provider
discriminators (`InfrastructureProvider.spec.machine.{libvirt | baremetal |
vsphere | kubevirt}`). The render layer compiles those discriminators
into Ansible vars, and the orchestration layer (Ansible playbooks + roles)
acts on them.

The Ansible bundle was first written for the libvirt + emulated-BMC lab and
the seams showed: roles named `provider_qemu` (cluster-scoped libvirt VM
lifecycle) and `provider_bmc` (provider-scoped sushy-tools) shared a
`provider_` prefix yet sat at different layers and concerns; cross-role
coupling baked the boot-artifacts HTTP port into both `provider_qemu`'s
firewall reconciliation and `ocp_install_agent`'s loopback Redfish URL;
dispatch was a string compare on `provider.type` repeated across every
playbook with no extension surface for vSphere or OpenShift Virtualization.

## Decision

The Ansible bundle adopts a uniform layer + concern + kind taxonomy and
dispatches to roles by computed name.

**Role taxonomy.** Every role name encodes its layer first, then its
concern, then (when applicable) the provider kind:

```
host_*           # provider-agnostic OS prep / proxy / kvm packages
network_*        # provider-agnostic networking (DNS, LB, validation)
cluster_*        # per-cluster, runs on gitups_infra_hosts
provider_*       # provider-scoped, runs on gitups_provider_hosts
ocp_*            # openshift-install agent install / boot / destroy
```

Within `cluster_substrate_*`, `provider_bmc_*`, and `ocp_boot_*`, the
suffix is the provider kind: `libvirt`, `baremetal`, `vsphere`, `kubevirt`
for substrates; `emulated`, `redfish`, `ipmi`, `none` for BMCs.

**Dispatch contract.** The render layer emits three discriminator fields
on the projected provider vars:

- `provider.kind` — `libvirt | baremetal | vsphere | kubevirt`.
- `provider.substrateRole` — selects `cluster_substrate_<role>`.
- `provider.bmcRole` — selects `provider_bmc_<role>` and `ocp_boot_<role>`.
- `provider.bootArtifactsHttp.{enabled,bindAddress,port}` — gates
  `provider_boot_artifacts_http`.

The kind→role mapping lives in one Go switch (`render.providerDispatch`).
Playbooks invoke roles by dynamic name: `role: "cluster_substrate_{{
provider.substrateRole }}"`. Every kind resolves to a real role; vSphere
and OpenShift Virtualization use explicit no-op roles
(`provider_bmc_none`, `ocp_boot_none`) so dispatch never fails to resolve.

**Layer separation.** `cluster_substrate_*` owns substrate state for a
single cluster (libvirt VMs, bare-metal credential staging). `provider_*`
owns substrate state shared across all clusters of one provider (BMC
emulator stack, boot-artifacts HTTP server, managed load balancers).
`ocp_*` owns OpenShift install orchestration and is provider-neutral
except for the BMC boot handover, which delegates to `ocp_boot_<bmcRole>`.

The boot-artifacts HTTP server is its own role (`provider_boot_artifacts_http`)
rather than a side concern of either the substrate or the BMC role: the
firewall hole on the substrate's bridge belongs to the substrate role
(parameterised on `provider.bootArtifactsHttp.port`); the systemd unit
publishing the artifacts belongs to the provider-scoped HTTP role.

## Consequences

- New providers add four files and one switch case: a substrate role, a
  BMC role (or `provider_bmc_none`), an OCP-boot role, and the kind→role
  entry in `render.providerDispatch`. No playbook edits.
- The `provider.kind` string is the single discriminator the playbooks
  see; structural sub-blocks stay the source of truth in the schema, but
  Ansible templates never introspect them directly.
- Boot-artifacts HTTP is a first-class concern with one consumer
  contract; when vSphere or OpenShift Virtualization land they default to
  `enabled: false` because both have native ISO mount mechanisms.
- The `ocp_install_agent` role no longer hard-codes a sushy loopback
  Redfish URL; it includes `ocp_boot_<bmcRole>` after staging the agent
  ISO. Real-BMC hardware can plug in at the same join point.
- Existing emulated-BMC + libvirt flows are byte-equivalent at the
  Ansible level: the renamed roles do exactly what the old roles did,
  minus the cross-layer coupling.
