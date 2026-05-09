# Local Libvirt 1 Host, 1 SNO Hub

**Install type:** disconnected (`Environment.spec.ocpInstall.disconnected`).

The control host is itself the libvirt provider host (SSH back to `localhost`)
and runs a single SNO hub cluster with no managed clusters attached. Smallest
case that still exercises Redfish BMC emulation, managed HAProxy, managed name
resolution, disconnected release content, and generated self-signed registry
trust.

## Topology

| Cluster | Role | Topology | Network |
| --- | --- | --- | --- |
| `local-libvirt-1-host-hub` | hub | single-node | `cluster-infrastructure-hub.yaml: spec.networks.primary.cidr` |

The provider host is `local-libvirt-host` at the address from
`provider.yaml: spec.hosts.local-libvirt-host.ssh.address` (defaults to `localhost`).

## Disconnected Inputs

`environment.yaml` uses `ocpInstall.disconnected` with:

- mirror registry hostname / port from
  `environment.yaml: spec.ocpInstall.disconnected.mirrorRegistry` (resolved on
  the cluster network to the libvirt bridge gateway,
  `cluster-infrastructure-hub.yaml: spec.networks.primary.gateway`)
- mirror credentials secret `mirror-registry-credentials`
- generated trust bundle `mirror-registry-ca`
- mirrored OpenShift release payload sources
- local HAProxy image from `environment.yaml: spec.componentImages.load-balancer.haproxy.local`

## Secrets

Secret files live outside the repo under `~/.gitups/secrets` by default.

| Secret name | Purpose |
| --- | --- |
| `openshift-pull-secret` | OCP pull secret JSON |
| `cluster-admin-key` | Public SSH key installed into OCP nodes |
| `local-libvirt-host-admin-key` | SSH key for the local provider host when SSH is required |
| `mirror-registry-credentials` | Mirror registry `username:password` |
| `mirror-registry-ca` | Generated registry CA certificate |
| `local-libvirt-1-host-bmc-credentials` | Redfish emulator credentials |

```text
install -d -m 0700 ~/.ssh ~/.gitups/secrets
ssh-keygen -t ed25519 -f ~/.ssh/gitups-local-libvirt-1-host -N '' -C gitups-local-libvirt-1-host
install -m 0600 ~/.ssh/gitups-local-libvirt-1-host.pub ~/.gitups/secrets/cluster-admin-key
ssh-keygen -t ed25519 -f ~/.ssh/gitups-local-libvirt-host -N '' -C gitups-local-libvirt-host
install -m 0600 ~/.ssh/gitups-local-libvirt-host ~/.gitups/secrets/local-libvirt-host-admin-key
install -m 0600 /dev/stdin ~/.ssh/authorized_keys < <(cat ~/.ssh/authorized_keys 2>/dev/null; cat ~/.ssh/gitups-local-libvirt-host.pub)
gitups secrets pull-secret set --name openshift-pull-secret --from-file ~/pull-secret.json
gitups secrets generate -f test/e2e/local-libvirt-1-host-1-sno-hub
```

## Commands

```text
make e2e-dry-run CASE=local-libvirt-1-host-1-sno-hub
make e2e CASE=local-libvirt-1-host-1-sno-hub
```

Equivalent CLI flow:

```text
gitups bastion check -f test/e2e/local-libvirt-1-host-1-sno-hub
gitups provider check -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --dry-run
gitups provider apply -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --dry-run
gitups clusters apply -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --dry-run
gitups provider apply -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
gitups clusters apply -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
gitups clusters destroy -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
gitups provider destroy -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
```

## External Prerequisites

The provider's `spec.registry.mirrorRegistry` capability bootstraps the local
registry automatically: the `provider_mirror_registry` role brings up
`docker.io/library/registry:3.1.1@sha256:85347ed2ecde64161c7a4788a4d7d3dcc9d6f86f7be95834022e3c6a423a945a` on the control host (port `5000`) with htpasswd
auth and the generated self-signed CA, then mirrors the OpenShift release
payload, every `componentImages.<cat>.<type>` `public→local` pair, and the
registry-server image itself. Subsequent applies are fully air-gapped from the
cluster network's perspective.

First apply only requires outbound public reachability for the provider host
to reach `docker.io/library/registry:3.1.1@sha256:85347ed2ecde64161c7a4788a4d7d3dcc9d6f86f7be95834022e3c6a423a945a`, `quay.io/openshift-release-dev/*`, and
each `componentImages[*].public` ref. After the role completes once, the
local mirror serves them.

The cluster nodes resolve the mirror registry hostname via the libvirt
network's dnsmasq (auto-plumbed from `Environment.spec.ocpInstall.disconnected`).
On the control host itself, add an `/etc/hosts` entry so the registry's
generated CA matches the URL during pre-flight checks. Use the network gateway
and registry hostname from your config:

```text
<cluster-infrastructure-hub.yaml: spec.networks.primary.gateway> <mirror registry host from environment.yaml>
```
