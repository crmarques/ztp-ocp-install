# QEMU 1 Host, 1 SNO Hub

**Install type:** disconnected (`Environment.spec.ocpInstall.disconnected`).

One remote QEMU/KVM provider host hosts a single SNO hub cluster with no
managed clusters attached. Smallest case that still exercises Redfish BMC
emulation, managed HAProxy, managed name resolution, disconnected release
content, and generated self-signed registry trust. For the variant where the
control host is itself the QEMU/KVM provider host, see
[`local-qemu-1-host-1-sno-hub`](../local-qemu-1-host-1-sno-hub/README.md).

## Topology

| Cluster | Role | Topology | Network |
| --- | --- | --- | --- |
| `qemu-1-host-hub` | hub | single-node | `192.168.130.0/24` |

The provider host is `remote-qemu-host` at `192.168.140.10` (SSH user `root`).

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
| `openshift-pull-secret` | OCP pull secret JSON |
| `cluster-admin-key` | Public SSH key installed into OCP nodes |
| `remote-qemu-host-admin-key` | SSH key for the remote provider host |
| `mirror-registry-credentials` | Mirror registry `username:password` |
| `mirror-registry-ca` | Generated registry CA certificate |
| `qemu-1-host-bmc-credentials` | Redfish emulator credentials |

```text
install -d -m 0700 ~/.ssh ~/.gitups/secrets
ssh-keygen -t ed25519 -f ~/.ssh/gitups-qemu-1-host -N '' -C gitups-qemu-1-host
install -m 0600 ~/.ssh/gitups-qemu-1-host.pub ~/.gitups/secrets/cluster-admin-key
ssh-keygen -t ed25519 -f ~/.ssh/gitups-remote-qemu-host -N '' -C gitups-remote-qemu-host
install -m 0600 ~/.ssh/gitups-remote-qemu-host ~/.gitups/secrets/remote-qemu-host-admin-key
ssh-copy-id -i ~/.ssh/gitups-remote-qemu-host.pub root@192.168.140.10
gitups secrets pull-secret set --name openshift-pull-secret --from-file ~/pull-secret.json
gitups secrets generate -f test/e2e/qemu-1-host-1-sno-hub
```

Install the matching public key in `/root/.ssh/authorized_keys` on the remote
provider host (the `ssh-copy-id` line above is one way).

## Commands

```text
make e2e-dry-run CASE=qemu-1-host-1-sno-hub
make e2e CASE=qemu-1-host-1-sno-hub
```

Equivalent CLI flow:

```text
gitups validate -f test/e2e/qemu-1-host-1-sno-hub --check-host
gitups render   -f test/e2e/qemu-1-host-1-sno-hub --state-dir /tmp/gitups-qemu-1-host-1-sno-hub
gitups apply    -f test/e2e/qemu-1-host-1-sno-hub --state-dir /tmp/gitups-qemu-1-host-1-sno-hub --dry-run
gitups apply    -f test/e2e/qemu-1-host-1-sno-hub --state-dir /tmp/gitups-qemu-1-host-1-sno-hub --yes
gitups status   -f test/e2e/qemu-1-host-1-sno-hub --state-dir /tmp/gitups-qemu-1-host-1-sno-hub --diff
gitups destroy  -f test/e2e/qemu-1-host-1-sno-hub --state-dir /tmp/gitups-qemu-1-host-1-sno-hub --yes
```

## External Prerequisites

Before a full apply, provide a registry reachable as `registry.mirror.local:5000`
from the control host and the remote provider host's cluster network. It must
contain the OpenShift `4.21.10` release payload and the local HAProxy image.
Ensure routing from the control host to the cluster machine network through
the provider host.
