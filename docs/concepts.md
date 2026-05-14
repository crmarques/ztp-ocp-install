---
title: Concepts
---

# Concepts

Short definitions for reading examples and command output. The full contract is
in [`/specs/`](../specs/index.md).

## Desired State

The YAML you author. It uses `apiVersion: bootwright.io/v1alpha1` and is the only
source of declared platform intent. Generated files are outputs, not edit
points.

## The Four Kinds

- `Environment`: shared environment defaults such as base domain,
  OpenShift install mode, secret sources, OpenShift release, and component
  image pins.
- `InfrastructureProvider`: provider connections and reusable capabilities,
  such as libvirt hosts, Redfish emulation, bare-metal BMC defaults, managed
  HAProxy, mirror registry, managed Squid proxy, or provider credentials.
- `ClusterInfrastructure`: one cluster's realised infrastructure on a
  provider: networks, machines, endpoints, managed load balancers, and managed
  name resolution. Load balancers are optional — omitting them defers the VIPs
  to an external LB or, for multi-node `platform: baremetal`, to the agent
  installer's built-in keepalived + haproxy on the control planes.
- `OCPCluster`: provider-neutral OpenShift intent: topology, install method,
  cluster networking, and node identity.

## Provider Swap

Provider-specific facts stay out of `OCPCluster`. Moving a cluster from the
libvirt/Redfish lab example to real bare metal changes only
`InfrastructureProvider` and `ClusterInfrastructure`.

## OCP Install Type, Proxy, And Registries

`Environment.spec.ocpInstallType` selects how the OpenShift install reaches
its release content:

- `connected` (default when unset) for public registry access.
- `disconnected` for local mirror usage; requires `spec.registries.mirror` and
  trust material.

Two optional top-level blocks shape every component, independent of install
type:

- `spec.proxy` (`http`, `https`, `noProxy`, `auth.proxyAuthRef`) — applies to
  bastion CLI downloads, provider-host package and image pulls, generated
  `install-config.yaml`, and `openshift-install`. Bootwright auto-extends
  `noProxy` with cluster-local endpoints (service/cluster CIDRs, `.svc`,
  `.cluster.local`, `localhost`, base domain, mirror registry host, provider
  host addresses); user entries take precedence and are listed first.
- `spec.registries` (`mirror`, `imageDigestSources`) — describes the OpenShift
  mirror endpoint, its credentials, and trust bundle. Required for
  `disconnected`; optional alongside `connected` when you mirror release
  content but still have public-network access for everything else.

If `spec.proxy` is paired with a referenced
`InfrastructureProvider.spec.proxy.squid`, Bootwright provisions an authenticated
Squid proxy and materializes its htpasswd from
`spec.proxy.auth.proxyAuthRef` (file-backed or `generated.credentials`).
Otherwise the proxy URL is external. On Bootwright-managed libvirt networks only,
the managed proxy path also disables direct VM NAT egress so cluster VMs leave
through Squid.

### Managed-Squid Two-URL Model

For the managed-Squid case Bootwright derives **two** proxy URLs from the same
Squid deployment, because hosts and VMs can't reach Squid at the same address:

- **Host URL** — `http://<squid-host-ssh-address>:<port>`. Written by
  `host_proxy` into `/etc/dnf/dnf.conf`, `/etc/environment`, the systemd
  drop-in, and `pip.conf` on every host (bastion, providers, infra, OCP).
  Uses the SSH address bootwright already knows works on this host (since
  ansible reached it that way), so it's routable before libvirt is
  installed — solving the bootstrap chicken-and-egg where host_proxy must
  configure a proxy that host_libvirt needs to install libvirt itself.
- **VM URL** — `http://<libvirt-network-gateway>:<port>`. Embedded in
  `install-config.yaml` so the OpenShift cluster reaches Squid at runtime.
  VMs sit on the libvirt bridge and naturally reach Squid (bound via host
  networking) by sending traffic at the gateway IP.

For an **external proxy** both URLs collapse to the same user-configured
URL, so the split is invisible. Internally the values surface as
`bootwright_ocp_install.proxy.http` / `.https` (host) and `.vmHttp` / `.vmHttps`
(VM) in the ansible vars file.

## Future: Multi-Cluster Topology

Forward-looking architecture leaves room for one cluster to host ACM and
OpenShift GitOps and reconcile additional clusters whose intent is published
as fleet GitOps content. That publication path is not implemented today;
every `OCPCluster` in the desired state is installed locally via the agent
installer.

## Workflow Targets

Provisioning commands are verb-first. `bootwright apply <target>` runs
idempotent phases through explicit targets:

1. `bastion`: controller-local dependencies (managed Ansible venv, OCP CLIs).
2. `infra`: provider infrastructure plus per-cluster substrate
   (`InfrastructureProvider` services and `ClusterInfrastructure` instances).
3. `clusters`: openshift-install agent against the cluster nodes plus per-cluster
   install state.
4. `hub` *(reserved)*: hub-cluster components for clusters declaring
   `role: hub`. Not implemented yet.
5. `all`: infra, clusters, and the reserved hub component step.

`bootwright check <target>` exposes the matching read-only checks. Cluster
targets accept `--scope` to select named `OCPCluster` definitions.

## Rendered Output

Rendering is internal to mutating targets, and can be requested explicitly with
`bootwright render installer`. Bootwright writes deterministic output under
`--state-dir`, including effective state, installer assets, Ansible inventory
and variables, and the embedded Ansible bundle.

## Secrets

Desired state references secret names. Secret bytes live outside the repo under
`<bootwright-user-dir>/secrets` by default. See
[`/specs/security.md`](../specs/security.md).
