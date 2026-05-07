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
- `python3` on `PATH`; `gitups doctor fix` installs a
  pinned Ansible runtime into the Gitups-managed venv by default.
- Permission to manage root-owned host runtime state under `/var/lib/gitups`
  on provider hosts.
- Install secret material under `~/.gitups/secrets` or a chosen
  `--secrets-dir`.

`gitups doctor check -f <state>` reports controller dependency status.

## Build

```text
make build
```

## Dry Run

```text
bin/gitups validate -f test/e2e/local-libvirt-1-host-1-sno-hub --check-host
bin/gitups preflight -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --dry-run
bin/gitups plan -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub
bin/gitups apply infra -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --dry-run
bin/gitups apply clusters -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --dry-run
```

The dry run renders state and prints the Ansible command for each phase without
changing the host.

## Apply

```text
bin/gitups apply infra -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
bin/gitups apply clusters -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
```

Each `apply` scope validates first, renders state, prints the phase plan, and
executes only that scope. `--ask-become-pass` defaults to false when gitups
runs as root and true otherwise; pass `--ask-become-pass=false` on non-root
hosts that have passwordless sudo.

## Check Status

```text
bin/gitups status -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --diff
```

`status --diff` reports drift between the rendered desired state and the
current `--state-dir`.

## Destroy

```text
bin/gitups destroy all -f test/e2e/local-libvirt-1-host-1-sno-hub --state-dir /tmp/gitups-local-libvirt-1-host-1-sno-hub --yes
```

`destroy` reverses the phase order and removes generated state after a full
successful teardown unless `--keep-state-dir` is set.
