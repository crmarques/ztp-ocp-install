---
title: Quickstart
---

# Quickstart

The fastest end-to-end path is `test/e2e/local-libvirt-sno-hub-gitea`: a single SNO
hub modeled as a libvirt-managed VM on one host, with Redfish BMC emulation and a
local registry mirror (disconnected install).

## Prerequisites

- Linux host with KVM support (`/dev/kvm` present).
- Go toolchain compatible with `go.mod`.
- `python3` on `PATH`; `gitups apply bastion` installs a
  pinned Ansible runtime into the Gitups-managed venv by default.
- Permission to manage root-owned host runtime state under `/var/lib/gitups`
  on provider hosts.
- Install secret material under `~/.gitups/secrets` or a chosen
  `--secrets-dir` (env: `GITUPS_SECRETS_DIR`).

`gitups check bastion -f <state>` reports controller dependency status.

## Build

```text
make build
```

## Dry Run

```text
bin/gitups check bastion -f test/e2e/local-libvirt-sno-hub-gitea
bin/gitups check infra -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --dry-run
bin/gitups check clusters -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --dry-run
bin/gitups check hub -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea
bin/gitups apply infra -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --dry-run
bin/gitups render installer -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea
bin/gitups apply clusters -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --dry-run
bin/gitups apply hub -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --dry-run
```

The dry run renders state and prints the Ansible command for each scope without
changing the host.

## Apply

```text
bin/gitups apply bastion -f test/e2e/local-libvirt-sno-hub-gitea --yes
bin/gitups apply infra -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --yes
bin/gitups apply clusters -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --yes
bin/gitups apply hub -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea
```

Each mutating `apply` target validates first, renders state, prints the phase
plan, and executes only that target. `apply hub` currently validates the
selected hub cluster and reports that hub component schema is not implemented
yet. `--ask-become-pass` defaults to false when gitups runs as root and true
otherwise; pass `--ask-become-pass=false` on non-root hosts that have
passwordless sudo.

State directory cleanup is manual; remove the `--state-dir` path once you no
longer need the rendered artifacts.
