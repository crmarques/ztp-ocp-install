# Desired State Spec

The desired-state model is the user API. Four kinds, four layers, one
source of truth per fact.

- API group / version: `gitups.io/v1alpha1`
- Kinds: `Environment`, `InfrastructureProvider`, `ClusterInfrastructure`,
  `OCPCluster`
- Generated outputs (Ansible inventory and vars, installer assets, lock
  file, effective state) are not user edit points.

`v1alpha1` is unstable. Breaking changes ship without migrations,
aliases, or compatibility shims; promotion to `v1beta1` is gated on
stable spec coverage of a real multi-provider deployment.

## Layer Ownership

| Layer | Kind | Owns |
| --- | --- | --- |
| Global UX | `Environment` | base domain, OpenShift install mode (typed sub-blocks: `connected` / `restricted` / `disconnected`), shared secret refs, OpenShift release defaults, component image pins |
| Substrate | `InfrastructureProvider` | provider hosts (shared pool with structural connection sub-block), capability sub-blocks (`machine` / `loadBalancer` / `nameResolution`) — each independently optional |
| Cluster infra | `ClusterInfrastructure` | provider composition (`providerRefs` list), per-cluster network instances (with provider-typed sub-blocks), machines (with provider-typed placement), endpoints (api / api-int / ingress with VIPs), load-balancer endpoint binds |
| Cluster intent | `OCPCluster` | role, topology, install method/overrides, networking (clusterNetwork / serviceNetwork), OCP node identity |

`InfrastructureProvider` is **capability-oriented**. Each top-level
capability sub-block (`machine`, `loadBalancer`, `nameResolution`) is
**independently optional**: a provider declares only what it supplies. At
least one capability must be set. `spec.hosts` is also optional — capabilities
that need an SSH-reachable Linux host reference an entry by name; capabilities
that talk to an appliance via API embed their endpoint inline.

`ClusterInfrastructure.spec.providerRefs` is a **list**: a cluster may
compose machines from one provider and load balancing from another. The
union of referenced providers' capabilities must contain at most one
contributor per capability.

## `Environment`

```yaml
apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: connected-fleet
spec:
  baseDomain: example.test
  ocpInstall:
    connected: {}
  secrets:
    pullSecretRef:
      name: openshift-pull-secret
    clusterSSHKeyRef:
      name: cluster-admin-key
  openshift:
    release:
      channel: stable-4.21
      version: 4.21.12
  componentImages: {}
```

Rules:

- `ocpInstall` scopes the connected/restricted/disconnected selection to
  OpenShift install material (release payload, mirror registry, trust
  bundles). It does not describe the lab host's substrate connectivity.
- `ocpInstall` carries exactly one of `connected`, `restricted`, or
  `disconnected`. The presence of the sub-block is the configuration; no
  `mode` or `type` string sits beside it.
- When `ocpInstall` is omitted, the normalizer treats it as `connected: {}`.
- `ocpInstall.connected` is an empty struct. No proxy, mirror, or trust
  material may be declared inside it.
- `ocpInstall.restricted` and `ocpInstall.disconnected` carry typed
  `proxy` and `registries` blocks. The mirror's CA is referenced via
  `registries.mirror.trustBundleRef.name`. `disconnected` additionally
  requires registry mirrors and a non-empty `trustBundleRef`; the
  validator rejects `disconnected` without both.
- Every secret name lives under `Environment.spec.keys[name]`, with
  exactly one source set: `file:` for operator-supplied material on
  disk, or `generated:` for material gitups produces (a
  `username:password\n` credentials file or a self-signed cert/key
  pair). `gitups secrets generate -f` materializes only generated keys;
  file-sourced keys must exist at their declared paths or be written by the
  dedicated secret writer commands. The bytes never appear in YAML.
- `Environment` owns proxy, registry mirrors, trust bundles, secret refs,
  OpenShift defaults, and component image pins.
- `Environment` must not define machines, provider hosts, BMC settings,
  VIPs, load-balancer placement, DNS placement, or per-cluster topology.

## `InfrastructureProvider`

```yaml
apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: qemu-redfish-provider
spec:
  hosts:
    qemu-host:
      ssh:
        address: 192.168.10.11
        keyRef:
          name: qemu-host-ssh
      capabilities:
        - libvirt
        - hosts-file
  machine:
    libvirt:
      hostRefs:
        - name: qemu-host
      bmcEmulation: {}
      machineProfiles: {}
  loadBalancer:
    haProxy:
      hostRef:
        name: qemu-host
  nameResolution:
    hostsFile:
      hostRefs:
        - name: qemu-host
  registry:
    mirrorRegistry:
      hostRef:
        name: qemu-host
      port: 5000
```

Rules:

- `spec` is capability-oriented. Each top-level sub-block is independently
  optional: `machine` (substrate flavors `libvirt | baremetal | vsphere |
  kubevirt`), `loadBalancer` (flavors `haProxy`, …), `nameResolution`
  (flavors `hostsFile`, …), `registry` (flavors `mirrorRegistry`, …). At
  least one capability must be set.
- `spec.hosts` is the shared host pool. Each entry carries a structural
  connection sub-block — v1 ships only `ssh`. Capabilities reference hosts
  by name (`hostRef` / `hostRefs`); appliance-style capabilities embed the
  endpoint inline and need no host pool.
- `spec.hosts.<name>.ssh.user` defaults to the invoking controller user when
  omitted. Gitups still treats the host as a provider-host target and uses
  Ansible root escalation for mutating provider, cluster, and OCP workflows,
  including `ssh.address: localhost`.
- Each capability sub-block (`machine`, `loadBalancer`, `nameResolution`,
  `registry`) is itself a structural-discriminator union: exactly one
  flavor sub-block is set. There is no `type` / `mode` / `kind`
  discriminator string.
- Omitting `loadBalancer`, `nameResolution`, or `registry` means
  **external** — the operator owns that concern for clusters bound to
  this provider.
- `registry.mirrorRegistry` requires its `hostRef` host to list the
  `mirror-registry` capability. The URL, credentials, and trust material
  remain on `Environment.spec.ocpInstall.{disconnected,restricted}.registries.mirror`;
  the provider only contributes placement.
- Owns: provider host pool with capabilities, machine substrate (with
  BMC service settings and reusable machine profiles for libvirt), load
  balancer placement, name resolution placement, mirror registry
  placement.
- Must not own per-cluster network instances (bridge names, portgroups,
  CIDRs), per-machine placement, OpenShift role, release, install config,
  OCP node roles, cluster VIPs, or cluster endpoint definitions, or the
  registry URL/credentials/trust material.

## `ClusterInfrastructure`

```yaml
apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
    name: hub
spec:
  providerRefs:
    - name: qemu-redfish-provider
  networks:
    primary:
      cidr: 192.168.130.0/24
      gateway: 192.168.130.1
      dnsServers:
        - 192.168.130.1
      libvirt:
        bridge: vbr-hub
  machines:
    master-0:
      profileRef:
        name: sno
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.130.20
          macAddress: 52:54:00:21:11:10
      rootDeviceHints:
        deviceName: /dev/vda
      libvirt:
        hostRef:
          name: qemu-host
  endpoints:
    api:
      address: 192.168.130.10
    apiInt:
      address: 192.168.130.10
    ingress:
      address: 192.168.130.11
  loadBalancers:
    default:
      endpoints:
        - api
        - apiInt
        - ingress
```

Rules:

- `providerRefs` is a non-empty list. The closure of all referenced
  providers' capabilities supplies what the cluster needs; at most one
  contributor per capability (`machine`, `loadBalancer`, `nameResolution`)
  is allowed in the closure. A bare-metal `machine` provider can be
  composed with an haProxy `loadBalancer` provider on a separate host.
- Per-cluster network instances live here. Each entry under `spec.networks`
  carries the IP layer (CIDR, gateway, DNS) plus a substrate-typed sub-block
  that realises the network (`libvirt.bridge`, `vsphere.portgroup`, …). The
  sub-block must match the closure-supplied machine flavor.
- VIPs, endpoint addresses, and load-balancer endpoint binds live here.
  Load-balancer **placement** lives on the provider's
  `loadBalancer.<flavor>` capability — clusters declare which endpoints to
  bind, not where the LB runs.
- Standard OpenShift load-balancer ports are implied by endpoint names
  (`api` → 6443, `apiInt` → 22623, `ingress` → 80/443) unless an entry
  explicitly overrides them.
- A default load balancer may bind all standard endpoints by name. Omitting
  `loadBalancers` entirely means external (operator-owned).
- Provider-specific machine placement (`libvirt.hostRef`, `baremetal.bmc`,
  `vsphere.{datastore,folder,template}`) lives here because it allocates
  machines on a provider; the placement sub-block must match the closure's
  machine flavor.
- Name resolution placement is **not** declared here — it lives on the
  supplying provider's `nameResolution.hostsFile` capability. Omission of
  the capability on every referenced provider means external DNS.
- Must not copy provider host addresses, registry URLs, base domain, or
  OpenShift release.

## `OCPCluster`

```yaml
apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: hub
spec:
  role: hub
  topology: single-node
  infrastructureRef:
    name: hub
  install:
    method: agent
  networking:
    clusterNetwork:
      - cidr: 10.128.0.0/14
        hostPrefix: 23
    serviceNetwork:
      - 172.30.0.0/16
  nodes:
    master-0:
      role: control-plane
```

Rules:

- Must not contain VIPs, load-balancer refs, DNS placement, BMC settings,
  MAC addresses, or any provider-typed sub-block.
- `role` is `hub` or `managed`. When omitted, the normalizer treats clusters
  named `hub` or ending in `-hub` as `hub`, and every other cluster as
  `managed`.
- `nodes.<name>.machineRef` defaults to `<name>`; specify it only when the
  OCP node name differs from the machine name.
- Release and install defaults inherit from `Environment`; per-cluster
  overrides remain allowed on `OCPCluster.spec.install`.
- Agent-install boot artifact wiring (minimal-ISO selection and provider-
  local `bootArtifactsBaseURL`) is Gitups-derived from `Environment`
  `ocpInstall`, the referenced provider, and `ClusterInfrastructure`.
  Users do not set those fields; validation rejects them.
- For disconnected installs, Gitups derives OpenShift release payload
  `imageDigestSources` for `ocp-release` and `ocp-v4.0-art-dev`, defaults
  their `sourcePolicy` to `NeverContactSource`, and rejects mirror refs
  that do not use the configured local registry.
- Installer override blocks (`installConfigOverrides`,
  `agentConfigOverrides`) are reserved for installer-native fields Gitups
  does not own. Override keys whose value Gitups derives from
  `Environment`, `ClusterInfrastructure`, or `InfrastructureProvider` are
  rejected by validation.

## References

- All cross-object references use a single-field shape with `name`:
  `LocalObjectReference { name }` for object refs, `SecretRef { name }`
  for secret material refs.

## Validation Rules

The validator enforces:

- Reject infrastructure fields in `OCPCluster`: VIPs, load-balancer refs,
  DNS placement, BMC, MACs, provider-typed blocks, network sub-blocks.
- Reject OpenShift intent in `InfrastructureProvider`: topology, release,
  install overrides, OCP node roles.
- Reject per-cluster instance facts on `InfrastructureProvider`: per-cluster
  networks (bridges, portgroups), endpoints, load balancers.
- Reject provider definitions copied into `ClusterInfrastructure`:
  provider host addresses and credentials.
- Reject network sub-blocks on `ClusterInfrastructure.spec.networks` whose
  provider kind disagrees with the referenced `InfrastructureProvider`.
- Reject `Environment.spec.ocpInstall.disconnected` without registry
  mirror and trust material.
- Reject `Environment.spec.ocpInstall.disconnected` when no
  `InfrastructureProvider` in the loaded set supplies
  `spec.registry.mirrorRegistry`. Omission means external; for
  disconnected, an external mirror is not assumed.
- Reject duplicated facts across layers when a referenced lower layer
  owns the fact.
- Resolve every upper-layer reference to the correct lower-layer object
  or child object.
- Reject `OCPCluster.spec.install.{installConfigOverrides, agentConfigOverrides}`
  keys whose value Gitups owns, including `apiVersion`, `metadata`,
  `baseDomain`, `pullSecret`, `sshKey`, `additionalTrustBundle`,
  `controlPlane`, `compute`, `networking.machineNetwork`,
  `imageDigestSources`, platform `apiVIPs` / `ingressVIPs` blocks,
  `agentConfig.minimalISO`, `agentConfig.bootArtifactsBaseURL`, and the
  entire `agentConfig.hosts` array.

## CLI Contract

The user-facing CLI is organized by verb. Provisioning targets are `bastion`,
`infra`, `clusters`, `hub`, and `all`. The GitOps authoring group remains
`gitups gitops *` until the GitOps command shape is redesigned.

| Command | Reads input? | Mutates? | Purpose |
| --- | --- | --- | --- |
| `init-repo --cluster-name <name> --provider <provider>` | no | local only | Creates `<state-dir>/clusters-bootstrap.git/<cluster>/{gitups,openshift}` and scaffolds `environment.yaml`, `provider.yaml`, `infra.yaml`, and `cluster.yaml` under `gitups/`. Providers: `vsphere`, `bare-metal`, `emulated-bare-metal`. |
| `secrets` | optional | yes (writes secrets) | `generate`, `pull-secret set`, and credential writers — the only writers into `<gitups-user-dir>/secrets`. |
| `check bastion` | optional | no | Controller prerequisite checks for the selected desired state. |
| `check infra` | yes | no | Local + Ansible read-only checks for provider hosts and per-cluster substrate. |
| `check clusters [--scope a,b]` | yes | no | Local + Ansible read-only checks for OpenShift cluster installation. |
| `check hub` | yes | no | Validates that exactly one cluster is selected for the hub role. Hub component readiness is reserved until the hub component schema lands. |
| `check all` | yes | no | Runs bastion, infra, cluster, and hub selection checks. |
| `render installer [--scope a,b]` | yes | local only | Renders installer assets under `<state-dir>/clusters-bootstrap.git/<cluster>/openshift/`. |
| `apply bastion [--dry-run]` | optional | yes | Installs pinned controller-local dependencies, defaulting to the user-owned Gitups-managed Ansible venv. |
| `apply infra [--dry-run]` | yes | yes | Converges `InfrastructureProvider` and `ClusterInfrastructure`: provider services plus per-cluster substrate. |
| `apply clusters [--scope a,b] [--dry-run]` | yes | yes | Runs `openshift-install agent` for selected clusters. |
| `apply hub [--dry-run]` | yes | reserved | Reserved for hub components installed onto the cluster declaring `role: hub`; today it validates the hub selection and reports no component schema. |
| `apply all [--dry-run]` | yes | yes | Runs `apply infra`, cluster installation, and the reserved hub component step. |

Common flags accepted by provisioning target commands:

- `--file` / `-f` — desired-state YAML file or directory; may be repeated.
  Defaults to `<state-dir>/clusters-bootstrap.git/*/gitups`.
- `--state-dir` — generated state directory (env: `GITUPS_STATE_DIR`).
- `--secrets-dir` — local install secret material directory (env: `GITUPS_SECRETS_DIR`).
- `--scope` — comma-separated `OCPCluster.metadata.name` list, accepted by
  `clusters` and `installer` targets.

Configuration env vars:

- `GITUPS_USER_DIR` — overrides the default `~/.gitups` user directory.
- `GITUPS_STATE_DIR` — overrides the default `<user-dir>/state`.
- `GITUPS_SECRETS_DIR` — overrides the default `<user-dir>/secrets`.

Multi-cluster fleet GitOps publication (one cluster running ACM/OpenShift
GitOps to reconcile additional clusters) is forward-looking architecture and
not implemented today. `apply clusters` installs every selected cluster,
including the cluster declaring `role: hub`. `apply hub` is reserved for
post-provisioning hub components and currently only validates that exactly one
hub cluster is selected.

## Gitops authoring artifacts (peer kind)

`GitOpsPackageSet` is a gitops authoring artifact. It is **not** a member of
the cluster-infra `State` aggregate above; it is loaded by a separate path
(`internal/gitops/load`) and consumed by the `gitups gitops *` command group.
Its ownership rules, minimal and expanded profiles, catalog source drivers
(`filesystem`, `oci`, `git`), and renderer priority are specified in
[gitops.md](gitops.md).

The two loaders are disjoint: `infra.LoadNormalizeValidate` continues to fail
on unknown kinds, and `internal/gitops/load.PackageSet` accepts only GitOps
kinds. A user-authored YAML carries either set, never both.
