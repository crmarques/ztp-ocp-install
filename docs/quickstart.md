---
title: Quickstart
---

# Quickstart

The fastest end-to-end path is `test/e2e/local-libvirt-1-host-1-sno-hub`: a single SNO
hub modeled as a libvirt-managed VM on one host, with Redfish BMC emulation and a
local registry mirror (disconnected install).

## Prerequisites

- Linux host with KVM support (`/dev/kvm` present).
- Go toolchain compatible with `go.mod`.
- `python3` on `PATH`; `gitups bastion apply` installs a
  pinned Ansible runtime into the Gitups-managed venv by default.
- Permission to manage root-owned host runtime state under `/var/lib/gitups`
  on provider hosts.
- Install secret material under `~/.gitups/secrets` or a chosen
  `--secrets-dir` (env: `GITUPS_SECRETS_DIR`).

`gitups bastion check -f <state>` reports controller dependency status.

## Build

```text
make build
```

## Dry Run

```text
bin/gitups bastion check -f test/e2e/local-libvirt-1-host-1-sno-hub
bin/gitups provider check -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --dry-run
bin/gitups clusters check -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --dry-run
bin/gitups provider apply -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --dry-run
bin/gitups clusters apply -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --dry-run
```

The dry run renders state and prints the Ansible command for each scope without
changing the host.

## Apply

```text
bin/gitups bastion apply -f test/e2e/local-libvirt-1-host-1-sno-hub --yes
bin/gitups provider apply -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
bin/gitups clusters apply -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
```

Each `apply` scope validates first, renders state, prints the phase plan, and
executes only that scope. `--ask-become-pass` defaults to false when gitups
runs as root and true otherwise; pass `--ask-become-pass=false` on non-root
hosts that have passwordless sudo.

## Destroy

```text
bin/gitups clusters destroy -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
bin/gitups provider destroy -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
bin/gitups bastion destroy --yes
```

Run scopes in reverse order. State directory cleanup is manual; remove the
`--state-dir` path once you no longer need the rendered artifacts.
