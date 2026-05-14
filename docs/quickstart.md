---
title: Quickstart
---

# Quickstart

The fastest end-to-end path is `test/e2e/sno-libvirt`: a single SNO
hub modeled as a libvirt-managed VM on one host, with Redfish BMC emulation and a
local registry mirror (disconnected install).

## Prerequisites

- Linux host with KVM support (`/dev/kvm` present).
- Go toolchain compatible with `go.mod`.
- `python3` on `PATH`; `bootwright apply bastion` installs a
  pinned Ansible runtime into the Bootwright-managed venv by default.
- Permission to manage root-owned host runtime state under `/var/lib/bootwright`
  on provider hosts.
- Install secret material under `~/.bootwright/secrets` or a chosen
  `--secrets-dir` (env: `BOOTWRIGHT_SECRETS_DIR`).

`bootwright check bastion -f <state>` reports controller dependency status.

## Build

```text
make build
```

## Dry Run

```text
bin/bootwright check bastion -f test/e2e/sno-libvirt
bin/bootwright check infra -f test/e2e/sno-libvirt --state-dir /tmp/bootwright-sno-libvirt --dry-run
bin/bootwright check clusters -f test/e2e/sno-libvirt --state-dir /tmp/bootwright-sno-libvirt --dry-run
bin/bootwright check hub -f test/e2e/sno-libvirt --state-dir /tmp/bootwright-sno-libvirt
bin/bootwright apply infra -f test/e2e/sno-libvirt --state-dir /tmp/bootwright-sno-libvirt --dry-run
bin/bootwright render installer -f test/e2e/sno-libvirt --state-dir /tmp/bootwright-sno-libvirt
bin/bootwright apply clusters -f test/e2e/sno-libvirt --state-dir /tmp/bootwright-sno-libvirt --dry-run
bin/bootwright apply hub -f test/e2e/sno-libvirt --state-dir /tmp/bootwright-sno-libvirt --dry-run
```

The dry run renders state and prints the Ansible command for each scope without
changing the host.

## Installer Render Output

`bootwright render installer` writes two artifacts per `OCPCluster` under
`<state-dir>/git-repos/clusters-bootstrap/<cluster>/openshift/`:

- `install-config.yaml` and `agent-config.yaml` with placeholder strings
  (`<bootwright-ssh-key-ref:...>`, `bootwright-secret-ref:...`) in place of secret
  material. These are safe to inspect and are the GitOps-publishable source.
- With `--resolve-secrets`, effective copies are written under
  `<state-dir>/runtime/<cluster>/installer/install-config.yaml` and
  `agent-config.yaml`. These have the pull secret, SSH key, additional trust
  bundle, mirror-registry auth, and proxy credentials inlined from
  `--secrets-dir` (default `$BOOTWRIGHT_SECRETS_DIR` or `~/.bootwright/secrets`),
  written mode `0600`, and are the files `openshift-install agent create
  image` consumes. Treat them as credentials and keep them off version
  control — they live outside the bootstrap repo for exactly that reason.

`bootwright apply clusters` rewrites the runtime
`installer/install-config.yaml` and `installer/agent-config.yaml` at apply
time, so the `--resolve-secrets` step is optional and only needed when you
want to inspect the effective config
before booting.

## Apply

```text
bin/bootwright apply bastion -f test/e2e/sno-libvirt --yes
bin/bootwright apply infra -f test/e2e/sno-libvirt --state-dir /tmp/bootwright-sno-libvirt --yes
bin/bootwright apply clusters -f test/e2e/sno-libvirt --state-dir /tmp/bootwright-sno-libvirt --yes
bin/bootwright apply hub -f test/e2e/sno-libvirt --state-dir /tmp/bootwright-sno-libvirt
```

Each mutating `apply` target validates first, renders state, prints the phase
plan, and executes only that target. `apply hub` currently validates the
selected hub cluster and reports that hub component schema is not implemented
yet. `--ask-become-pass` defaults to false when bootwright runs as root and true
otherwise; pass `--ask-become-pass=false` on non-root hosts that have
passwordless sudo.

State directory cleanup is manual; remove the `--state-dir` path once you no
longer need the rendered artifacts.
