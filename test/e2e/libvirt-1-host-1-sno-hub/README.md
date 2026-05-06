# Libvirt 1 Host, 1 SNO Hub

Single SNO hub on a remote libvirt provider host. The control host is a
separate machine; gitups reaches the provider over SSH. Connected install
(`Environment.spec.ocpInstall.connected`).

For the variant where control = provider, see
[`local-libvirt-1-host-1-sno-hub`](../local-libvirt-1-host-1-sno-hub/README.md).

## Topology

| Host | Address | Role |
| --- | --- | --- |
| Control host | (local) | runs `gitups` |
| `remote-libvirt-host` | `192.168.140.10` (`provider.yaml: spec.hosts.remote-libvirt-host.ssh.address`) | libvirt + sushy-tools + HAProxy |
| `libvirt-1-host-hub` (SNO) | API `192.168.130.10` / Ingress `192.168.130.11` (`cluster-infrastructure-hub.yaml: spec.endpoints`) | hub cluster |

The cluster primary network (`192.168.130.0/24`) is reached through the provider host.

## Prerequisites

- Linux control host with Go toolchain compatible with `go.mod`, `make`, and `python3` on `PATH`.
- SSH access to `root@<provider-host-ip>` from the control host (console or
  password auth is sufficient for the initial `ssh-copy-id` in step 3).
- Outbound public reachability from the provider host to `quay.io` and
  `docker.io` (connected install).
- An OCP pull secret JSON downloaded from <https://console.redhat.com>.

## Run

### 0. Edit the fixture

Set `spec.hosts.remote-libvirt-host.ssh.address` in `provider.yaml` to your
actual provider host IP before running anything else.

### 1. Build

```text
make build
```

Compiles the CLI and syncs the embedded Ansible bundle into the binary.

### 2. Bootstrap the controller

```text
bin/gitups setup controller -f test/e2e/libvirt-1-host-1-sno-hub
bin/gitups doctor
```

Installs a pinned Ansible venv and the OCP CLIs (`oc`, `kubectl`,
`openshift-install`) at the release version declared in `environment.yaml`.
`doctor` confirms every controller dependency is satisfied before you proceed.

### 3. Stage secrets

> If migrating from an existing controller, skip this section and follow
> [Migrating from an existing controller](#migrating-from-an-existing-controller) instead.

Create the SSH keypairs and stage them where gitups expects them:

```text
install -d -m 0700 ~/.ssh ~/.gitups/secrets

# Cluster node SSH key — public half goes to the cluster, private half stays local.
ssh-keygen -t ed25519 -f ~/.ssh/gitups-libvirt-1-host      -N '' -C gitups-libvirt-1-host
install -m 0600 ~/.ssh/gitups-libvirt-1-host.pub ~/.gitups/secrets/cluster-admin-key

# Provider host SSH key — used by Ansible to connect to the libvirt host.
ssh-keygen -t ed25519 -f ~/.ssh/gitups-remote-libvirt-host -N '' -C gitups-remote-libvirt-host
install -m 0600 ~/.ssh/gitups-remote-libvirt-host ~/.gitups/secrets/remote-libvirt-host-admin-key

# Authorize the provider key on the remote host (requires an existing way in:
# console, password, or another key).
ssh-copy-id -i ~/.ssh/gitups-remote-libvirt-host.pub root@<provider-host-ip>
```

Then stage the pull secret and let gitups generate the remaining credentials:

```text
bin/gitups secrets pull-secret set --name openshift-pull-secret --from-file ~/pull-secret.json
bin/gitups secrets generate -f test/e2e/libvirt-1-host-1-sno-hub
```

### 4. Apply

```text
make e2e CASE=libvirt-1-host-1-sno-hub
```

Runs `apply infra` then `apply ocp` against
`/tmp/gitups-libvirt-1-host-1-sno-hub` with `--yes`. Use
`make e2e-dry-run CASE=libvirt-1-host-1-sno-hub` to preview without changing
the host.

### 5. Tear down

```text
make e2e-destroy CASE=libvirt-1-host-1-sno-hub
```

## Migrating from an existing controller

If you already ran this case on another machine, copy secrets and SSH keys
instead of re-generating them. The provider host keeps the key already
authorized, so generating new keys would break SSH access.

```text
# On the old control host:
rsync -av ~/.gitups/secrets/  <new-control-host>:~/.gitups/secrets/
rsync -av ~/.ssh/gitups-libvirt-1-host \
          ~/.ssh/gitups-libvirt-1-host.pub \
          ~/.ssh/gitups-remote-libvirt-host \
          ~/.ssh/gitups-remote-libvirt-host.pub \
          <new-control-host>:~/.ssh/

# On the new control host:
chmod 600 ~/.ssh/gitups-remote-libvirt-host ~/.ssh/gitups-libvirt-1-host
chmod 644 ~/.ssh/gitups-remote-libvirt-host.pub ~/.ssh/gitups-libvirt-1-host.pub
```

Then continue from step 1 (build + setup controller); skip step 3.

## Running the controller in a container (same machine)

Use this to simulate the control-host / provider-host separation on a single
machine. The container acts as the control host; the Linux host is the provider
(libvirt runs on it). SSH leaves the container and re-enters the host at its
real IP.

**Before starting:** set `provider.yaml: spec.hosts.remote-libvirt-host.ssh.address`
to the host machine's real IP (not `127.0.0.1`; the SSH daemon must accept
connections on that address).

Build a minimal controller image once:

```text
podman build -t gitups-controller - <<'EOF'
FROM fedora:41
RUN dnf install -y python3 openssh-clients && dnf clean all
EOF
```

Run any `gitups` command inside the container. `--network=host` lets the
container reach the provider at its real IP. `GITUPS_HOME` redirects gitups to
the mounted directory so the container reuses the ansible venv, OCP tools, and
secrets already installed on the host — no re-bootstrap needed.

```text
STATE_DIR=/tmp/gitups-libvirt-1-host-1-sno-hub
mkdir -p "$STATE_DIR"

podman run --rm -it \
  --network=host \
  -e GITUPS_HOME=/gitups-home \
  -v "$HOME/.gitups":/gitups-home:Z \
  -v "$HOME/.ssh/gitups-libvirt-1-host":/root/.ssh/gitups-libvirt-1-host:ro,Z \
  -v "$HOME/.ssh/gitups-libvirt-1-host.pub":/root/.ssh/gitups-libvirt-1-host.pub:ro,Z \
  -v "$HOME/.ssh/gitups-remote-libvirt-host":/root/.ssh/gitups-remote-libvirt-host:ro,Z \
  -v "$HOME/.ssh/gitups-remote-libvirt-host.pub":/root/.ssh/gitups-remote-libvirt-host.pub:ro,Z \
  -v "$(pwd)":/workspace:ro,Z \
  -v "$STATE_DIR:$STATE_DIR:Z" \
  -w /workspace \
  gitups-controller \
  bin/gitups apply infra -f test/e2e/libvirt-1-host-1-sno-hub \
    --state-dir "$STATE_DIR" --yes
```

Swap the last two lines to run any other verb (`doctor`, `plan`, `apply ocp`,
`destroy all`, etc.). The state dir mount uses the same path inside and outside
the container so generated files are accessible from the host after the run.

To simulate a fully clean controller (no shared venv), omit the
`GITUPS_HOME` mount and run `setup controller` as the first command:

```text
podman run --rm -it \
  --network=host \
  -v "$HOME/.gitups/secrets":/root/.gitups/secrets:ro,Z \
  -v "$HOME/.ssh/gitups-remote-libvirt-host":/root/.ssh/gitups-remote-libvirt-host:ro,Z \
  -v "$HOME/.ssh/gitups-remote-libvirt-host.pub":/root/.ssh/gitups-remote-libvirt-host.pub:ro,Z \
  -v "$(pwd)":/workspace:ro,Z \
  -v "$STATE_DIR:$STATE_DIR:Z" \
  -w /workspace \
  gitups-controller \
  sh -c 'bin/gitups setup controller -f test/e2e/libvirt-1-host-1-sno-hub && \
         bin/gitups apply infra -f test/e2e/libvirt-1-host-1-sno-hub \
           --state-dir '"$STATE_DIR"' --yes'
```

In this form the ansible venv is created inside the container and discarded
when it exits; each run re-bootstraps from scratch.

## Secrets used

| Name | Source |
| --- | --- |
| `openshift-pull-secret` | `gitups secrets pull-secret set` |
| `cluster-admin-key` | public half of the cluster SSH key |
| `remote-libvirt-host-admin-key` | private half of the provider SSH key |
| `libvirt-1-host-bmc-credentials` | generated by `gitups secrets generate` |
