# QEMU 3 Hosts, 1 SNO Hub + 2 OCP Fleet

**Install type:** disconnected (`Environment.spec.ocpInstall.disconnected`).

Three QEMU/KVM provider hosts each run a single cluster: one SNO hub plus two
multi-node managed OCP clusters (compact control plane, three nodes each).

## Topology

| Cluster | Provider host | Role | Topology |
| --- | --- | --- | --- |
| `qemu-3-hosts-sno-hub` | `hub-host` (`192.168.140.10`) | hub | single-node |
| `qemu-3-hosts-ocp-01` | `ocp-01-host` (`192.168.140.11`) | managed | multi-node (3 nodes) |
| `qemu-3-hosts-ocp-02` | `ocp-02-host` (`192.168.140.12`) | managed | multi-node (3 nodes) |

Cluster networks are `192.168.150.0/24`, `192.168.151.0/24`, and
`192.168.152.0/24`. Spokes use the `compact-control-plane` provider profile.

## Disconnected Inputs

`environment.yaml` uses `ocpInstall.disconnected` with:

- mirror registry `registry.mirror.local:5000`
- mirror credentials `mirror-registry-credentials`
- generated trust bundle `mirror-registry-ca`
- mirrored OpenShift release payload sources
- local HAProxy image `registry.mirror.local:5000/library/haproxy:3.2.15`

## Secrets

Secret files live outside the repo under `~/.gitups/secrets` by default.

| Secret name | Purpose |
| --- | --- |
| `provider-host-admin-key` | SSH key for each provider host |
| `openshift-pull-secret` | OCP pull secret JSON |
| `cluster-admin-key` | Public SSH key installed into OCP nodes |
| `qemu-3-hosts-sno-hub-fleet-bmc-credentials` | Redfish emulator credentials |
| `mirror-registry-credentials` | Mirror registry `username:password` |
| `mirror-registry-ca` | Generated registry CA certificate |

```text
install -d -m 0700 ~/.ssh ~/.gitups/secrets
ssh-keygen -t ed25519 -f ~/.ssh/gitups-qemu-3-hosts -N '' -C gitups-qemu-3-hosts
install -m 0600 ~/.ssh/gitups-qemu-3-hosts     ~/.gitups/secrets/provider-host-admin-key
install -m 0600 ~/.ssh/gitups-qemu-3-hosts.pub ~/.gitups/secrets/cluster-admin-key
gitups secrets pull-secret set --name openshift-pull-secret --from-file ~/pull-secret.json
gitups secrets generate -f test/e2e/qemu-3-hosts-1-sno-hub-2-ocp-fleet
```

Install the matching public key in `/root/.ssh/authorized_keys` on each
provider host.

## Commands

```text
make e2e-dry-run CASE=qemu-3-hosts-1-sno-hub-2-ocp-fleet
make e2e CASE=qemu-3-hosts-1-sno-hub-2-ocp-fleet
```

Equivalent CLI flow:

```text
gitups validate -f test/e2e/qemu-3-hosts-1-sno-hub-2-ocp-fleet --check-host
gitups render   -f test/e2e/qemu-3-hosts-1-sno-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-sno-hub-2-ocp-fleet
gitups apply    -f test/e2e/qemu-3-hosts-1-sno-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-sno-hub-2-ocp-fleet --dry-run
gitups apply    -f test/e2e/qemu-3-hosts-1-sno-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-sno-hub-2-ocp-fleet --yes
gitups status   -f test/e2e/qemu-3-hosts-1-sno-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-sno-hub-2-ocp-fleet --diff
gitups destroy  -f test/e2e/qemu-3-hosts-1-sno-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-sno-hub-2-ocp-fleet --yes
```

## External Prerequisites

Provide a local registry reachable as `registry.mirror.local:5000` from the
control host and each provider host. It must contain the OpenShift `4.21.10`
release payload and the local HAProxy image. Ensure routing from the control
host to every cluster machine network through the matching provider host.
