# QEMU 1 Host, 1 SNO Hub

**Install type:** disconnected (`Environment.spec.ocpInstall.disconnected`).

One local QEMU/KVM provider host hosts a single SNO hub cluster with no managed
clusters attached. Smallest case that still exercises Redfish BMC emulation,
managed HAProxy, managed name resolution, disconnected release content, and
generated self-signed registry trust.

## Topology

| Cluster | Role | Topology | Network |
| --- | --- | --- | --- |
| `qemu-1-host-hub` | hub | single-node | `192.168.130.0/24` |

The provider host is `local-qemu-host` at `localhost`.

## Disconnected Inputs

`environment.yaml` uses `ocpInstall.disconnected` with:

- mirror registry `registry.mirror.test:5000`
- mirror credentials `mirror-registry-credentials`
- generated trust bundle `mirror-registry-ca`
- mirrored OpenShift release payload sources
- local HAProxy image `registry.mirror.test:5000/library/haproxy:3.2.15`

## Secrets

Secret files live outside the repo under `~/.gitups/secrets` by default.

| Secret name | Purpose |
| --- | --- |
| `openshift-pull-secret` | OCP pull secret JSON |
| `cluster-admin-key` | Public SSH key installed into OCP nodes |
| `local-qemu-host-admin-key` | SSH key for the local provider host when SSH is required |
| `mirror-registry-credentials` | Mirror registry `username:password` |
| `mirror-registry-ca` | Generated registry CA certificate |
| `qemu-1-host-bmc-credentials` | Redfish emulator credentials |

```text
install -d -m 0700 ~/.ssh ~/.gitups/secrets
ssh-keygen -t ed25519 -f ~/.ssh/gitups-qemu-1-host -N '' -C gitups-qemu-1-host
install -m 0600 ~/.ssh/gitups-qemu-1-host.pub ~/.gitups/secrets/cluster-admin-key
ssh-keygen -t ed25519 -f ~/.ssh/gitups-local-qemu-host -N '' -C gitups-local-qemu-host
install -m 0600 ~/.ssh/gitups-local-qemu-host ~/.gitups/secrets/local-qemu-host-admin-key
install -m 0600 /dev/stdin ~/.ssh/authorized_keys < <(cat ~/.ssh/authorized_keys 2>/dev/null; cat ~/.ssh/gitups-local-qemu-host.pub)
gitups secrets pull-secret set --name openshift-pull-secret --from-file ~/pull-secret.json
gitups secrets bmc set --name qemu-1-host-bmc-credentials --generate
gitups secrets bmc set --name mirror-registry-credentials --generate
gitups secrets generate -f test/e2e/qemu-1-host-1-sno-hub
```

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

Before a full apply, provide a local registry reachable as
`registry.mirror.test:5000` from the host and cluster network. It must contain
the OpenShift `4.21.10` release payload and the local HAProxy image.
