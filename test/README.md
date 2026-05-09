# Tests

Repository tests run from the project root.

- `test/e2e/<case>/`: self-contained Gitups input sets for real host
  validation and apply flows.
- Go unit and rendering fixtures live with the package tests that consume
  them.

E2E cases are test assets, not canonical UX examples. Canonical examples live
under [`/examples/`](../examples/).

## E2E Case Names

Case names describe the architectural shape — substrate, host layout, and
fleet shape:

```text
<substrate>-<host-layout>-<fleet-shape>
```

OCP install mode (connected vs. disconnected) is not encoded in the case
name; it is documented in each case's `README.md`.

Current cases:

- `local-libvirt-1-host-1-sno-hub` — control host is itself the libvirt provider
  host (SSH back to `localhost`), 1 SNO hub, no managed clusters.
- `libvirt-1-host-1-sno-hub` — 1 remote QEMU host, 1 SNO hub, no managed clusters.
- `qemu-3-hosts-1-sno-hub-2-ocp-fleet` — 3 QEMU hosts, 1 SNO hub plus 2
  multi-node managed OCP clusters (one cluster per host).
- `qemu-3-hosts-1-hub-2-ocp-fleet` — 3 QEMU hosts, 1 multi-node hub plus 2
  multi-node managed OCP clusters (one cluster per host).

## Running A Case

```text
make build
make list-e2e-cases
make e2e-dry-run CASE=libvirt-1-host-1-sno-hub
make e2e         CASE=libvirt-1-host-1-sno-hub
make e2e-destroy CASE=libvirt-1-host-1-sno-hub
```

The user-facing equivalent is plain `gitups`:

```text
gitups bastion check -f test/e2e/<case>
gitups provider check -f test/e2e/<case> --state-dir /tmp/gitups-<case> --dry-run
gitups clusters check -f test/e2e/<case> --state-dir /tmp/gitups-<case> --dry-run
gitups provider apply -f test/e2e/<case> --state-dir /tmp/gitups-<case> --dry-run
gitups clusters apply -f test/e2e/<case> --state-dir /tmp/gitups-<case> --dry-run
gitups provider apply -f test/e2e/<case> --state-dir /tmp/gitups-<case> --yes
gitups clusters apply -f test/e2e/<case> --state-dir /tmp/gitups-<case> --yes
gitups clusters destroy -f test/e2e/<case> --state-dir /tmp/gitups-<case> --yes
gitups provider destroy -f test/e2e/<case> --state-dir /tmp/gitups-<case> --yes
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
