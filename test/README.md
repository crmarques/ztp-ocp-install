# Tests

Repository tests run from the project root.

- `test/e2e/<case>/`: self-contained Gitups input sets for real host
  validation and apply flows.
- Go unit and rendering fixtures live with the package tests that consume
  them.

E2E cases are test assets, not canonical UX examples. Canonical examples live
under [`/examples/`](../examples/).

## E2E Case Names

Case names describe the bastion/substrate shape — where the controller
runs, what provider the cluster lands on, and the cluster topology. OCP
install mode (connected vs. disconnected) is documented in each case's
`README.md`, not the case name.

Current cases:

- `container-bastion-local-libvirt-sno` — Gitups CLI runs inside a UBI9
  container; libvirt host is the same machine, reached as `localhost` via
  `podman run --network host`; one SNO cluster.

Retired fixtures live under `test/e2e/old/` and are kept only as Go test
inputs; they are not maintained as runnable cases or user documentation.

## Running A Case

```text
make build
make list-e2e-cases
make e2e-dry-run CASE=container-bastion-local-libvirt-sno
make e2e         CASE=container-bastion-local-libvirt-sno
make clean-e2e-state CASE=container-bastion-local-libvirt-sno
```

The user-facing equivalent is plain `gitups`:

```text
gitups check bastion -f test/e2e/<case>
gitups check infra -f test/e2e/<case> --state-dir /tmp/gitups-<case> --dry-run
gitups check all -f test/e2e/<case> --state-dir /tmp/gitups-<case> --dry-run
gitups apply all -f test/e2e/<case> --state-dir /tmp/gitups-<case> --dry-run
gitups apply all -f test/e2e/<case> --state-dir /tmp/gitups-<case> --yes
```

## Common Prerequisites

- Linux host with KVM support (`/dev/kvm`) for libvirt cases.
- Go toolchain compatible with `go.mod`.
- `ansible-playbook`, `python3`, and `pip3` on the controller `PATH`.
- Permission to escalate to root on provider hosts and manage host runtime
  state under `/var/lib/gitups`.
- Required install secrets under `~/.gitups/secrets` or `--secrets-dir`.

## Logs And Artifacts

Generated state defaults to `/tmp/gitups-<case>/`. Apply and destroy logs are
written under `ansible/artifacts/<phase>/ansible-output.log` inside that state
directory. Failed phases print the relevant log path.
