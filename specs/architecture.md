# Architecture Spec

## Pipeline

User YAML flows through Gitups in fixed stages:

```text
desired state → load → normalize → validate → render → orchestrate
```

- **desired state** — user-authored YAML files containing the four kinds.
- **load** — parses YAML and rejects unknown kinds and unknown fields.
- **normalize** — fills defaults from `Environment` into the layers below.
- **validate** — schema, layer ownership, and cross-reference checks.
- **render** — deterministic generation of installer assets, Ansible
  inventory and variables, GitOps manifests, lock file, and effective
  state.
- **orchestrate** — phased Ansible execution that converges actual state
  to desired state.

## Layers

The desired-state schema is four layers. Each layer references the layer
below by name. Replacing an object in one layer must not require editing
files in other layers.

| Layer | Kind |
| --- | --- |
| Global UX | `Environment` |
| Substrate | `InfrastructureProvider` |
| Cluster infra | `ClusterInfrastructure` |
| Cluster intent | `OCPCluster` |

`OCPCluster` is provider-agnostic. A provider swap (libvirt with emulated
BMC → real bare metal → vSphere) edits `InfrastructureProvider` and
`ClusterInfrastructure` only. CI asserts the swap invariant by diffing the
`OCPCluster` and `Environment` files across the canonical provider examples.

## Schema Rules

- **R1 Layered objects.** A fact defined inside one object is owned by
  that object only. Replacing one layer's object must not require editing
  files in other layers.
- **R2 Reference, don't repeat.** Container image refs, base domains,
  secret refs, provider host names, and machine names live in their owning
  object and are referenced by name from others. Validation rejects
  schemas that re-declare an attribute already present in a referenced
  object.
- **R3 Structural discriminators only.** Where one of N typed sub-blocks
  may be present, the presence of the sub-block is the discriminator.
  Spec objects do not carry `type`, `mode`, or `kind` discriminator
  strings beside the sub-block. Validation enforces "exactly one of {…}"
  on:
  - `InfrastructureProvider.spec.machine.{libvirt | baremetal | vsphere | kubevirt}`
  - `InfrastructureProvider.spec.loadBalancer.{haProxy | …}`
  - `InfrastructureProvider.spec.nameResolution.{hostsFile | …}`
  - `InfrastructureProvider.spec.hosts.<name>.{ssh | …}` (host connection)
  - per-machine placement on `ClusterInfrastructure.spec.machines.<name>`
  - per-network realisation on `ClusterInfrastructure.spec.networks.<name>`
  - `Environment.spec.ocpInstall.{connected | restricted | disconnected}`

`InfrastructureProvider` is **capability-oriented**: the top-level
sub-blocks (`machine`, `loadBalancer`, `nameResolution`) are
*independently optional* — at least one must be set, but a provider may
supply any subset. The host pool (`spec.hosts`) is a shared resource;
appliance-style capabilities (BigIP, vCenter API) embed their endpoint
inline and need no host pool.

`ClusterInfrastructure.spec.providerRefs` is a list. The closure of all
referenced providers' capabilities must contain at most one contributor
per capability. The renderer materialises this as a `ProviderClosure`:
a typed view that exposes the merged hosts pool plus the
machine/loadBalancer/nameResolution suppliers. Anything that exists
*because a particular cluster needs it* — networks, machines, endpoints,
load-balancer endpoint binds — is declared on `ClusterInfrastructure`
with a substrate-typed sub-block when the realisation is provider-specific.

## Provider Adapters

Provider-specific code sits behind explicit interfaces. Bare metal,
vSphere, OpenShift Virtualization, and lab libvirt support do not leak
into shared business logic except through typed capabilities and the
structural provider sub-blocks. New providers add a new structural
sub-block on `InfrastructureProvider.spec`, a matching sub-block on
`ClusterInfrastructure.spec.machines.<name>`, and (when networks need
provider-specific realisation) a matching sub-block on
`ClusterInfrastructure.spec.networks.<name>`. Cross-cutting code stays
provider-neutral.

## Future: Multi-Cluster Topology

Forward-looking architecture leaves room for one cluster to host ACM,
OpenShift GitOps, cluster provisioning assets, and placement / policy
intent — reconciling additional clusters whose intent is published as
fleet GitOps content. That publication path is not implemented today.
Adapters and inventory contracts must not hard-code single-cluster
assumptions, so a future split into provisioning-control plane plus
reconciled workload clusters remains possible without schema rework.

## Orchestration Rules

- Every phase is safe to re-run.
- Long-running operations expose status and failure reason.
- Generated artifacts are reproducible from the same input.
- External commands have explicit inputs, outputs, and error handling.

## Ansible Organization

- Playbooks describe workflows; roles describe reusable capabilities.
- Inventory and variables are generated from desired state. Users do not
  maintain inventory, `group_vars`, or `host_vars` as source-of-truth
  configuration.
- Controller setup and provider-host convergence are separate workflows.
  Controller-local Ansible runs as the invoking user by default. Provider,
  cluster, and OCP apply/destroy playbooks target provider hosts and execute
  through root escalation, even when the provider host address is
  `localhost`.
- Repository-owned Ansible content lives under `/ansible` and is embedded
  into the `gitups` binary via `internal/embedded` (build-time copy into
  `internal/embedded/bundle/`, captured by `//go:embed`). At runtime the
  CLI materialises the tree under `<state-dir>/ansible-bundle/`. Roles and
  collections paths are passed to `ansible-playbook` via
  `ANSIBLE_ROLES_PATH` and `ANSIBLE_COLLECTIONS_PATH` so the binary is
  independent of the user's working directory.
- Tasks must be idempotent. Prefer modules to shell. Shell tasks declare
  `changed_when` and `failed_when` where needed. Sensitive values use
  `no_log`. Long waits have explicit timeouts and clear failure output.

### Role taxonomy

Every role name encodes layer, concern, and (when applicable) provider
kind. ADR 0002 records the contract.

| Prefix | Layer | Hosts |
| --- | --- | --- |
| `host_*` | provider-agnostic OS prep | `gitups_infra_hosts`, `gitups_provider_hosts` |
| `network_*` | provider-agnostic networking | varies |
| `cluster_*` | per-cluster substrate | `gitups_infra_hosts` |
| `provider_*` | provider-scoped shared services | `gitups_provider_hosts` |
| `ocp_*` | openshift-install agent install / boot / destroy | `gitups_ocp_hosts` |

Within `cluster_substrate_*`, `provider_bmc_*`, and `ocp_boot_*` the
suffix is the provider kind: `libvirt`, `baremetal`, `vsphere`,
`kubevirt` for substrates; `emulated`, `redfish`, `ipmi`, `none` for
BMCs.

`provider_mirror_registry` runs a docker/distribution server on the
provider host that supplies `spec.registry.mirrorRegistry`. It executes in
`provider-prepare.yml` **before** `network_lb_managed` so any
local-mirrored image (HAProxy, future workloads) is reachable when its
consumer pulls it on subsequent applies.

### Provider dispatch

The render layer projects three discriminator fields onto the per-cluster
and per-provider Ansible vars. They drive dynamic role-name dispatch:

| Var | Drives |
| --- | --- |
| `provider.kind` | machine flavor on the closure (`libvirt \| baremetal \| vsphere \| kubevirt`) |
| `provider.substrateRole` | `role: cluster_substrate_<substrateRole>` |
| `provider.bmcRole` | `role: provider_bmc_<bmcRole>` and `include_role: ocp_boot_<bmcRole>` |
| `provider.bootArtifactsHttp.{enabled,bindAddress,port}` | gates `provider_boot_artifacts_http` |

The kind→role mapping is one switch in `render.providerDispatch`. Every
kind resolves to a real role; substrates with no external BMC use
`provider_bmc_none` and `ocp_boot_none` so dispatch never fails to
resolve. Adding a new provider is four role files plus one switch case;
no playbook edits.

## GitOps Output (forward-looking)

When fleet publication is implemented, generated GitOps content represents
the desired fleet state consumed by the cluster running ACM and OpenShift
GitOps. Today no GitOps publication runs; this section describes the
constraints that future content must satisfy.

- Deterministic from the same input.
- Reviewable before it is applied.
- Ownership boundaries visible in directory layout and, when useful, file
  headers.
- No runtime status mixed with declared intent.

Expected areas: bootstrap applications, ACM and OpenShift GitOps operator
configuration, managed-cluster definitions, placement / policy / day-2
configuration, and environment overlays. Prefer Kubernetes/OpenShift
native formats. Use Kustomize, Helm, or templating only where the tool
has a clear ownership boundary in the generated tree.

## Testing

- Schema and validation tests.
- Template rendering golden tests.
- Provider adapter contract tests.
- Ansible role syntax and idempotency tests.
- GitOps manifest validation.
- Lab end-to-end provisioning tests under `test/e2e/<case>/`. Lab
  emulation uses Redfish over libvirt-managed VMs so the test path
  stays close to real bare-metal workflows.
- Fast validation must run without a real cluster. Cluster-dependent
  tests are isolated, documented, and opt-in until automation is reliable.

E2E case fixtures are test assets, not canonical UX examples. Case names
describe substrate, host layout, and fleet shape, for example
`libvirt-1-host-1-sno-hub` or `libvirt-3-hosts-1-sno-hub-2-ocp-fleet`; OCP install
mode (connected vs. disconnected) is documented in each case's `README.md`
rather than encoded in the directory name. The canonical UX examples live under `examples/`. Cross-case operator
guidance lives in `test/README.md`; per-case detail lives in
`test/e2e/<case>/README.md`.
