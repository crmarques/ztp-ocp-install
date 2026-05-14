# Ansible Vars Contract

Go renders the entire variable set the embedded Ansible playbooks consume.
This file is the canonical description of that contract — what the Go
renderer emits, what Ansible roles may read, and how each fact is keyed.

**Authority:** the Go types in
[`internal/provisioning/render/vars_types.go`](../internal/provisioning/render/vars_types.go).
That file is the source of truth for field names, optionality, and shape.
This document describes the seam in human-readable form.

**Contract direction:** unidirectional. Go produces, Ansible consumes. No
Ansible role mutates or augments these facts; per-host slicing happens
through the `context_cluster` and `context_provider` shared roles, which
expose derived `bootwright_*_ctx` facts.

## Top-level facts

These are the seven root keys in the rendered `vars.yaml`, passed to
`ansible-playbook` via `-e @<path>/vars.yaml`.

| Fact | Go type | Shape | Owner roles |
|---|---|---|---|
| `bootwright_ocp_install` | `EnvironmentOCPInstallVars` | object | `install_agent`, `host_proxy` |
| `bootwright_providers` | `[]ProviderComponentVars` | array | `context_provider`, `bmc_*`, `boot_artifacts_http`, `proxy_squid`, `mirror_registry` |
| `bootwright_load_balancers` | `[]SharedLoadBalancerVars` | array | `context_provider`, `load_balancer_haproxy` |
| `bootwright_mirror_registries` | `[]MirrorRegistryRunVars` | array | `context_provider`, `mirror_registry` |
| `bootwright_forward_proxies` | `[]ForwardProxyRunVars` | array (omitempty) | `context_provider`, `proxy_squid`, `host_proxy` |
| `bootwright_clusters` | `[]ClusterVars` | array | `context_cluster`, `substrate_*`, `network_vips`, `name_resolution_hosts_file`, `install_agent`, `boot_*` |
| `bootwright_component_pins` | `[]ComponentPin` | array | (informational; recorded into `bootwright.lock.yaml`) |

## Per-host inventory contract

The inventory file Go writes alongside `vars.yaml` defines three groups
and per-host facts. Roles slice the top-level arrays above using these
host-level facts:

| Group | Per-host facts | Used by |
|---|---|---|
| `bootwright_provider_hosts` | `bootwright_provider_name`, `bootwright_host_name` | `context_provider` |
| `bootwright_infra_hosts` | `bootwright_provider_name`, `bootwright_cluster_name`, `bootwright_host_name` | `context_cluster` |
| `bootwright_ocp_hosts` | `bootwright_provider_name`, `bootwright_cluster_name`, `bootwright_host_name` | `context_cluster`, `install_agent` |

Connection facts (`ansible_host`, `ansible_user`, `ansible_ssh_private_key_file`)
are set per-host. Hosts always reach providers/clusters over SSH; the
provider host address may be `localhost` but the connection is SSH, not
local.

## Derived per-host facts (set by context roles)

After `context_cluster` (run as the first pre-task on `bootwright_infra_hosts`
and `bootwright_ocp_hosts`):

- `bootwright_current_cluster` — the selected entry from `bootwright_clusters`
- `bootwright_current_provider` — the selected entry from `bootwright_providers`
  (only when `bootwright_provider_name` is defined)
- `bootwright_cluster_ctx` — `{cluster, provider}`

After `context_provider` (run as the first pre-task on
`bootwright_provider_hosts`):

- `bootwright_current_provider` — selected entry from `bootwright_providers`
- `bootwright_current_load_balancers` — entries from `bootwright_load_balancers`
  matching the host
- `bootwright_current_mirror_registry` — single entry from
  `bootwright_mirror_registries` matching the host (or `none`)
- `bootwright_current_forward_proxy` — single entry from
  `bootwright_forward_proxies` matching the host (or `none`)
- `bootwright_provider_ctx` — `{provider, loadBalancers, mirrorRegistry, forwardProxy}`

Both context roles **assert** that their core selection resolved to a
non-empty match; a malformed render produces a loud failure instead of
silent empty results downstream.

## Top-level fact: `bootwright_ocp_install`

```yaml
bootwright_ocp_install:
  mode: connected | disconnected         # required
  disconnected: <bool>                   # required, redundant for Jinja convenience
  registry:                              # optional; only set when env.spec.registries.mirror is present
    url: <string>
    host: <hostname-from-url>
    credentialsRef: <local-path>
  proxy:                                 # optional; set when env.spec.proxy or per-cluster squid resolves
    http: <url>                          # host-facing
    https: <url>
    vmHttp: <url>                        # VM-facing (libvirt-bridge gateway)
    vmHttps: <url>
    noProxy: [<entry>, ...]
    proxyAuthRef: <local-path>
```

**Two-URL proxy model.** `http`/`https` are host-facing URLs (routable
before libvirt is up, so `host_proxy` can write a working
`HTTP(S)_PROXY` into `/etc/dnf/dnf.conf` on the first run). `vmHttp`/
`vmHttps` are the libvirt-bridge-gateway URLs embedded in
`install-config.yaml`. See
[`internal/provisioning/render/proxy.go`](../internal/provisioning/render/proxy.go).

## Top-level fact: `bootwright_providers`

One entry per `InfrastructureProvider`, regardless of whether the
provider supplies machines, load balancers, name resolution, registry,
or proxy.

```yaml
bootwright_providers:
  - name: <string>                       # provider metadata.name
    kind: libvirt | baremetal | vsphere | kubevirt
    substrateRole: libvirt | baremetal | vsphere | kubevirt
    bmcRole: emulated | redfish | ipmi | none
    bootArtifactsHttp:
      enabled: <bool>
      bindAddress: <string>              # only when enabled
      port: <int>                        # only when enabled
    infrastructureHosts: [...]           # ProviderHostVars, when spec.hosts is set
    bmc: {...}                           # ProviderBMCVars, when libvirt+BMCEmulation
```

**Provider dispatch.** `substrateRole`, `bmcRole`, and
`bootArtifactsHttp.enabled` drive dynamic role-name dispatch in
playbooks:
- `include_role: substrate_<substrateRole>` from `roles/cluster_infra/`
- `include_role: bmc_<bmcRole>` from `roles/providers/`
- `include_role: boot_<bmcRole>` from `roles/openshift/`

Every kind resolves to a real role (no-op stubs `bmc_none` /
`boot_none` exist so dispatch never fails).

## Top-level fact: `bootwright_clusters`

One entry per `ClusterInfrastructure`, materialised by the renderer with
its closure provider, OCP cluster intent, and resolved networks.

```yaml
bootwright_clusters:
  - name: <string>                       # ClusterInfrastructure metadata.name
    ocp:
      name: <string>                     # OCPCluster metadata.name
      topology: single-node | three-node | ha
      release: {channel, version}
      install:
        method: agent
        baseDomain: <string>
        pullSecretRef: <local-path>
        sshKeyRef: <local-path>
        releaseImageOverride: <image>    # optional, disconnected installs
        additionalTrustBundleRef: <local-path>
        generatedSecrets: [...]          # SelfSignedCertificate machinery
        localRegistry: {...}             # disconnected only
      installer:                         # paths inside the rendered tree
        relativeDir: <path>               # clusters-bootstrap.git/<cluster>/openshift (publishable)
        relativeInstallConfigPath: <path>
        relativeAgentConfigPath: <path>
        relativeWorkDir: <path>           # runtime/<cluster>/installer (local-only, openshift-install --dir)
      nodes: [...]                       # OCPClusterNodeVars
    provider: {...}                      # ProviderVars (per-cluster closure)
    network: {...}                       # ClusterNetworkVars
```

## Top-level facts: shared services

`bootwright_load_balancers`, `bootwright_mirror_registries`, and
`bootwright_forward_proxies` are flat arrays indexed by **(provider, host)**.
Context-provider slices them onto the host that should run each
component. Each entry carries the runtime (`podman`), image refs, ports,
and secret paths necessary for the component role to converge the
service idempotently.

## Image pinning: `bootwright_component_pins`

Recorded into the rendered `bootwright.lock.yaml`. Roles do **not** read
this fact — the per-component facts above already carry resolved image
URLs. The pin record exists for reproducibility review.

## Adding a new top-level fact

The contract changes only by:

1. Editing `internal/provisioning/render/vars_types.go` to add the type.
2. Wiring it into `Vars()` in `vars.go` (top-level assembler).
3. Filling it in a per-capability projector
   (`vars_install.go`, `vars_cluster.go`, `vars_provider.go`,
   `vars_proxy.go`, `vars_components.go`).
4. Documenting it here.
5. Adding the Ansible role that reads it.

Steps 1-3 must happen together (Go fails to compile otherwise). Step 4
is enforced by code review — if you add a fact the renderer emits that
no role reads, the seam grows silent dead weight.

## Schema validation (forward-looking)

[The architecture review](../specs/adr/README.md) proposed emitting a
JSON Schema alongside `vars.yaml` so an Ansible preflight task can
assert structural shape. That guardrail prevents render/Ansible drift —
"renderer added a field nobody reads" and "role reads a field nobody
renders" both fail loudly. Until that lands, this document is the
contract.
