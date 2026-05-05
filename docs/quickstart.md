---
title: Quickstart
---

# Quickstart

The fastest end-to-end path is `test/e2e/local-qemu-1-host-1-sno-hub`: a single SNO
hub modeled as a QEMU/KVM node on one host, with Redfish BMC emulation and a
local registry mirror (disconnected install).

## Prerequisites

- Linux host with KVM support (`/dev/kvm` present).
- Go toolchain compatible with `go.mod`.
- `python3` and `sudo` on `PATH`; `gitups setup controller` installs a
  pinned Ansible runtime into the Gitups-managed venv by default.
- Permission to manage root-owned host runtime state under `/var/lib/gitups`.
- Install secret material under `~/.gitups/secrets` or a chosen
  `--secrets-dir`.

`gitups doctor` reports controller dependency status.

## Build

```text
go build -o bin/gitups ./cmd/gitups
```

## Dry Run

```text
bin/gitups validate -f test/e2e/local-qemu-1-host-1-sno-hub --check-host
bin/gitups preflight -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --dry-run
bin/gitups plan -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub
bin/gitups apply infra -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --dry-run
bin/gitups apply hub -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --dry-run
```

The dry run renders state and prints the Ansible command for each phase without
changing the host.

## Apply

```text
bin/gitups apply infra -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --yes
bin/gitups apply hub -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --yes
```

Each `apply` scope validates first, renders state, prints the phase plan, and
executes only that scope. Use `--ask-become-pass=false` on hosts with
passwordless sudo.

## Check Status

```text
bin/gitups status -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --diff
```

`status --diff` reports drift between the rendered desired state and the
current `--state-dir`.

## Destroy

```text
bin/gitups destroy all -f test/e2e/local-qemu-1-host-1-sno-hub --state-dir /tmp/gitups-local-qemu-1-host-1-sno-hub --yes
```

`destroy` reverses the phase order and removes generated state after a full
successful teardown unless `--keep-state-dir` is set.
