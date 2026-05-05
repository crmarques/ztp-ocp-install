# Local QEMU 1 Host, 1 SNO Hub

**Install type:** disconnected (`Environment.spec.ocpInstall.disconnected`).

The control host is itself the QEMU/KVM provider host (SSH back to `localhost`)
and runs a single SNO hub cluster with no managed clusters attached. Smallest
case that still exercises Redfish BMC emulation, managed HAProxy, managed name
resolution, disconnected release content, and generated self-signed registry
trust.

## Topology

| Cluster | Role | Topology | Network |
| --- | --- | --- | --- |
| `local-qemu-1-host-hub` | hub | single-node | `192.168.130.0/24` |

The provider host is `local-qemu-host` at `localhost`.

## Disconnected Inputs

`environment.yaml` uses `ocpInstall.disconnected` with:

- mirror registry `registry.mirror.local:5000` (alias for the libvirt
  bridge gateway `192.168.130.1`)
- mirror credentials `mirror-registry-credentials`
- generated trust bundle `mirror-registry-ca`
- mirrored OpenShift release payload sources
- local HAProxy image `registry.mirror.local:5000/library/haproxy:3.2.15`

## Secrets

Secret files live outside the repo under `~/.gitups/secrets` by default.

| Secret name | Purpose |
| --- | --- |
| `openshift-pull-secret` | OCP pull secret JSON |
| `cluster-admin-key` | Public SSH key installed into OCP nodes |
| `local-qemu-host-admin-key` | SSH key for the local provider host when SSH is required |
| `mirror-registry-credentials` | Mirror registry `username:password` |
| `mirror-registry-ca` | Generated registry CA certificate |
| `local-qemu-1-host-bmc-credentials` | Redfish emulator credentials |

```text
install -d -m 0700 ~/.ssh ~/.gitups/secrets
ssh-keygen -t ed25519 -f ~/.ssh/gitups-local-qemu-1-host -N '' -C gitups-local-qemu-1-host
install -m 0600 ~/.ssh/gitups-local-qemu-1-host.pub ~/.gitups/secrets/cluster-admin-key
ssh-keygen -t ed25519 -f ~/.ssh/gitups-local-qemu-host -N '' -C gitups-local-qemu-host
install -m 0600 ~/.ssh/gitups-local-qemu-host ~/.gitups/secrets/local-qemu-host-admin-key
install -m 0600 /dev/stdin ~/.ssh/authorized_keys < <(cat ~/.ssh/authorized_keys 2>/dev/null; cat ~/.ssh/gitups-local-qemu-host.pub)
gitups secrets pull-secret set --name openshift-pull-secret --from-file ~/pull-secret.json
gitups secrets generate -f test/e2e/local-qemu-1-host-1-sno-hub
```

## Commands

```text
make e2e-dry-run CASE=local-qemu-1-host-1-sno-hub
make e2e CASE=local-qemu-1-host-1-sno-hub
```

Equivalent CLI flow:

```text
gitups validate -f test/e2e/local-qemu-1-host-1-sno-hub --check-host
gitups plan     -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub
gitups apply infra -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --dry-run
gitups apply hub   -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --dry-run
gitups apply infra -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --yes
gitups apply hub   -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --yes
gitups status   -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --diff
gitups destroy all -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --yes
```

## External Prerequisites

The provider's `spec.registry.mirrorRegistry` capability bootstraps the local
registry automatically: the `provider_mirror_registry` role brings up
`docker.io/library/registry:2` on the control host (port `5000`) with htpasswd
auth and the generated self-signed CA, then mirrors the OpenShift release
payload, every `componentImages.<cat>.<type>` `public→local` pair, and the
registry-server image itself. Subsequent applies are fully air-gapped from the
cluster network's perspective.

First apply only requires outbound public reachability for the provider host
to reach `docker.io/library/registry:2`, `quay.io/openshift-release-dev/*`, and
each `componentImages[*].public` ref. After the role completes once, the
local mirror serves them.

The cluster nodes resolve `registry.mirror.local` via the libvirt
network's dnsmasq (auto-plumbed from `Environment.spec.ocpInstall.disconnected`).
On the control host itself, add an `/etc/hosts` entry so the registry's
generated CA matches the URL during pre-flight checks:

```text
192.168.130.1 registry.mirror.local
```
