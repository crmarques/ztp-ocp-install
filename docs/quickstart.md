---
title: Quickstart
---

# Quickstart

The fastest end-to-end path is `test/e2e/container-bastion-local-libvirt-sno`: a single SNO
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
bin/gitups check bastion -f test/e2e/container-bastion-local-libvirt-sno
bin/gitups check infra -f test/e2e/container-bastion-local-libvirt-sno --state-dir /tmp/gitups-container-bastion-local-libvirt-sno --dry-run
bin/gitups check clusters -f test/e2e/container-bastion-local-libvirt-sno --state-dir /tmp/gitups-container-bastion-local-libvirt-sno --dry-run
bin/gitups check hub -f test/e2e/container-bastion-local-libvirt-sno --state-dir /tmp/gitups-container-bastion-local-libvirt-sno
bin/gitups apply infra -f test/e2e/container-bastion-local-libvirt-sno --state-dir /tmp/gitups-container-bastion-local-libvirt-sno --dry-run
bin/gitups render installer -f test/e2e/container-bastion-local-libvirt-sno --state-dir /tmp/gitups-container-bastion-local-libvirt-sno
bin/gitups apply clusters -f test/e2e/container-bastion-local-libvirt-sno --state-dir /tmp/gitups-container-bastion-local-libvirt-sno --dry-run
bin/gitups apply hub -f test/e2e/container-bastion-local-libvirt-sno --state-dir /tmp/gitups-container-bastion-local-libvirt-sno --dry-run
```

The dry run renders state and prints the Ansible command for each scope without
changing the host.

## Installer Render Output

`gitups render installer` writes two artifacts per `OCPCluster` under
`<state-dir>/clusters-bootstrap.git/<cluster>/openshift/`:

- `install-config.yaml` and `agent-config.yaml` with placeholder strings
  (`<gitups-ssh-key-ref:...>`, `gitups-secret-ref:...`) in place of secret
  material. These are safe to inspect.
- With `--resolve-secrets`, an additional `work/install-config.yaml` and
  `work/agent-config.yaml` are produced. These have the pull secret, SSH key,
  additional trust bundle, mirror-registry auth, and proxy credentials inlined
  from `--secrets-dir` (default `$GITUPS_SECRETS_DIR` or `~/.gitups/secrets`),
  written mode `0600`, and are the files `openshift-install agent create
  image` consumes. Treat them as credentials and keep them off version
  control.

`gitups apply clusters` rewrites `work/install-config.yaml` and
`work/agent-config.yaml` at apply time, so the `--resolve-secrets` step is
optional and only needed when you want to inspect the effective config
before booting.

## Apply

```text
bin/gitups apply bastion -f test/e2e/container-bastion-local-libvirt-sno --yes
bin/gitups apply infra -f test/e2e/container-bastion-local-libvirt-sno --state-dir /tmp/gitups-container-bastion-local-libvirt-sno --yes
bin/gitups apply clusters -f test/e2e/container-bastion-local-libvirt-sno --state-dir /tmp/gitups-container-bastion-local-libvirt-sno --yes
bin/gitups apply hub -f test/e2e/container-bastion-local-libvirt-sno --state-dir /tmp/gitups-container-bastion-local-libvirt-sno
```

Each mutating `apply` target validates first, renders state, prints the phase
plan, and executes only that target. `apply hub` currently validates the
selected hub cluster and reports that hub component schema is not implemented
yet. `--ask-become-pass` defaults to false when gitups runs as root and true
otherwise; pass `--ask-become-pass=false` on non-root hosts that have
passwordless sudo.

State directory cleanup is manual; remove the `--state-dir` path once you no
longer need the rendered artifacts.
