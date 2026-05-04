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
| Substrate | `InfrastructureProvider` | provider capabilities and connections (`qemuKVM` / `bareMetal` / `vmware` / `openShiftVirtualization`), provider hosts, BMC / Redfish service settings, reusable machine profiles |
| Cluster infra | `ClusterInfrastructure` | per-cluster network instances (with provider-typed sub-blocks), machines (with provider-typed placement), endpoints (api / api-int / ingress with VIPs), load balancers, managed name-resolution placement |
| Cluster intent | `OCPCluster` | role, topology, install method/overrides, networking (clusterNetwork / serviceNetwork), OCP node identity |

`InfrastructureProvider` declares only what the provider *is and exposes*:
how to reach it, which hosts and capabilities it carries, and reusable
templates. Anything created or attached *for a particular cluster* —
networks, machines, endpoints, load balancers — belongs on
`ClusterInfrastructure`.

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
      version: 4.21.10
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
  `proxy`, `registries`, and `trustBundle` blocks. `disconnected`
  additionally requires registry mirrors and trust material; the validator
  rejects `disconnected` without both.
- Registry trust material is referenced by name. Lab environments may request
  `trustBundle.generatedSelfSigned`, which creates local secret files at
  apply/generate time rather than inlining certificate bytes in YAML.
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
  qemuKVM:
    hosts: {}
    bmcEmulation: {}
    machineProfiles: {}
```

Rules:

- Provider type is structural: exactly one of `qemuKVM`, `bareMetal`,
  `vmware`, `openShiftVirtualization`. There is no `spec.type` field.
- Owns provider hosts, provider endpoints, provider credentials, BMC /
  Redfish service settings, and reusable machine profiles.
- Must not own per-cluster network instances (bridge names, portgroups,
  CIDRs), per-machine placement, OpenShift role, release, install config,
  OCP node roles, cluster VIPs, or cluster endpoint definitions.

## `ClusterInfrastructure`

```yaml
apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
    name: hub
spec:
  providerRef:
    name: qemu-redfish-provider
  networks:
    primary:
      cidr: 192.168.130.0/24
      gateway: 192.168.130.1
      dnsServers:
        - 192.168.130.1
      qemuKVM:
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
      qemuKVM:
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
      placement:
        providerHostRef:
          name: qemu-host
      endpoints:
        - api
        - apiInt
        - ingress
  nameResolution:
    managed:
      providerHostRefs:
        - name: qemu-host
```

Rules:

- Per-cluster network instances live here. Each entry under `spec.networks`
  carries the IP layer (CIDR, gateway, DNS) plus a provider-typed sub-block
  that realises the network on the provider (`qemuKVM.bridge`,
  `vmware.portgroup`, …). The sub-block must match the provider's
  `spec.<provider>` sub-block.
- VIPs, endpoint addresses, load-balancer bindings, and DNS / name-resolution
  placement live here.
- Standard OpenShift load-balancer ports are implied by endpoint names
  (`api` → 6443, `apiInt` → 22623, `ingress` → 80/443) unless an entry
  explicitly overrides them.
- A default load balancer may bind all standard endpoints by name.
- Provider-specific machine placement (`qemuKVM.hostRef`, `bareMetal.bmc`,
  `vmware.{datastore,folder,template}`) lives here because it allocates
  machines on a provider; the placement sub-block must match the
  provider's `spec.<provider>` sub-block.
- `nameResolution` carries exactly one of `managed` or `external`.
  `external` is the empty selection (the operator owns DNS); `managed`
  delegates `/etc/hosts` placement on listed provider hosts.
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
- Reject OpenShift intent in `InfrastructureProvider`: role, topology,
  release, install overrides, OCP node roles.
- Reject per-cluster instance facts on `InfrastructureProvider`: per-cluster
  networks (bridges, portgroups), endpoints, load balancers.
- Reject provider definitions copied into `ClusterInfrastructure`:
  provider host addresses and credentials.
- Reject network sub-blocks on `ClusterInfrastructure.spec.networks` whose
  provider kind disagrees with the referenced `InfrastructureProvider`.
- Reject `Environment.spec.ocpInstall.disconnected` without registry
  mirror and trust material.
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

The user-facing CLI is six verbs:

| Command | Reads input? | Mutates? | Purpose |
| --- | --- | --- | --- |
| `validate` | yes | no | Schema, defaults, cross-reference validation. `--check-host` adds local tooling discovery. |
| `render` | yes | local only | Writes deterministic artifacts under `--state-dir`. |
| `apply` | yes | yes | Converges the host toward desired state, phase by phase. |
| `destroy` | yes | yes | Reverses `apply` in reverse phase order. Requires `--yes` (or `--dry-run`). |
| `status` | yes | no | Reports desired counts, rendered artifact presence, phase table, and drift against `--state-dir` (`--diff` for full drift output). |
| `secrets` | optional | yes (writes secrets) | `pull-secret set`, `bmc set`, `generate` — the only writers into `<gitups-home>/secrets`. |

`apply` runs an automatic `validate --check-host` pass before mutating
anything. Users do not need a separate top-level host-check command.

Phases (apply order):

1. `infra` — provider infrastructure (hosts, VMs, BMC, LB, DNS).
2. `hub` — hub OCP install plus ACM and OpenShift GitOps operators.
3. `gitops-publish` — publish managed cluster intent to the hub-watched
   Git repo.

Spoke OCP installs are reconciled by the hub via ACM/GitOps after
`gitops-publish`; they are out of scope for `gitups apply`. New phases
extend the ordered list in code; they do not become new top-level verbs.
