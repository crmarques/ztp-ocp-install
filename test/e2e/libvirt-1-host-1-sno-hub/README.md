# Libvirt 1 Host, 1 SNO Hub

Single SNO hub on a remote libvirt provider host. The control host is a
separate machine; gitups reaches the provider over SSH. Connected install
(`Environment.spec.ocpInstall.connected`).

For the variant where control = provider, see
[`local-libvirt-1-host-1-sno-hub`](../local-libvirt-1-host-1-sno-hub/README.md).

Prefer to run `gitups` from a container instead of installing the controller
toolchain on the host? Skip ahead to
[Running the controller in a container](#running-the-controller-in-a-container)
for an end-to-end recipe.

## Topology

| Host | Address | Role |
| --- | --- | --- |
| Control host | (local) | runs `gitups` |
| `remote-libvirt-host` | `10.73.7.236` (`provider.yaml: spec.hosts.remote-libvirt-host.ssh.address`) | libvirt + sushy-tools + HAProxy |
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
bin/gitups doctor fix -f test/e2e/libvirt-1-host-1-sno-hub --yes
bin/gitups doctor check -f test/e2e/libvirt-1-host-1-sno-hub --yes
```

Installs a pinned Ansible venv and the OCP CLIs (`oc`, `kubectl`,
`openshift-install`) at the release version declared in `environment.yaml`.
`doctor check` confirms every controller dependency is satisfied before you proceed.

### 3. Stage secrets

> If migrating from an existing controller, skip this section and follow
> [Migrating from an existing controller](#migrating-from-an-existing-controller) instead.

Generate the SSH keypair and sync all file-sourced secrets into the gitups
secrets store:

```text
install -d -m 0700 ~/.ssh ~/.gitups/secrets

# One keypair is used for both the cluster nodes and provider host SSH access.
ssh-keygen -t ed25519 -f ~/.ssh/gitups-ssh-key -N '' -C gitups-ssh-key

# Authorize the key on the remote provider host (requires an existing way in:
# console, password, or another key).
ssh-copy-id -i ~/.ssh/gitups-ssh-key.pub root@<provider-host-ip>

# Stage the pull secret downloaded from https://console.redhat.com.
install -m 0600 ~/pull-secret.json ~/.gitups/secrets/openshift-pull-secret
```

Then ensure file-sourced keys exist and generate remaining credentials:

```text
bin/gitups secrets generate -f test/e2e/libvirt-1-host-1-sno-hub
```

### 4. Apply

```text
make e2e CASE=libvirt-1-host-1-sno-hub
```

Runs `apply infra` then `apply clusters` against
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
rsync -av ~/.ssh/gitups-ssh-key \
          ~/.ssh/gitups-ssh-key.pub \
          <new-control-host>:~/.ssh/

# On the new control host:
chmod 600 ~/.ssh/gitups-ssh-key
chmod 644 ~/.ssh/gitups-ssh-key.pub
```

Then continue from step 1 (build + doctor fix); skip step 3.

## Running the controller in a container

End-to-end recipe that runs `gitups` from a UBI9 container while libvirt stays
on the host. The container is the control host; the Linux host is the
provider. SSH leaves the container with `--network=host` and re-enters the
host at its real IP. The host-installed OCP CLIs are bind-mounted into the
container, so the image does not bundle them.

**Before starting:** set `provider.yaml: spec.hosts.remote-libvirt-host.ssh.address`
to the host machine's real IP (not `127.0.0.1`; the SSH daemon must accept
connections on that address). Stage the SSH key and pull secret on the host as
described in [Stage secrets](#3-stage-secrets).

Replace `crmarques` / `/home/crmarques` and `4.21.10` below with your
username and the OCP release declared in `environment.yaml`.

### 1. Build the binary

```text
make build
```

### 2. Build the controller image

The image bakes the freshly built `gitups` binary in and creates a non-root
user. NOPASSWD sudo is required for the playbooks.

```text
podman build -t gitups-controller-4.21.10 -f - . <<'EOF'
FROM docker.io/redhat/ubi9:9.7

RUN dnf install -y sudo \
    && dnf clean all \
    && rm -rf /var/cache/dnf

RUN useradd \
    --uid 10001 \
    --create-home \
    --home-dir /home/crmarques \
    --shell /sbin/nologin \
    crmarques

RUN echo 'crmarques ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/crmarques \
    && chmod 0440 /etc/sudoers.d/crmarques

RUN install -d -o 10001 -g 10001 -m 0700 /home/crmarques/.ssh \
    && install -d -o 10001 -g 10001 -m 0755 /home/crmarques/.gitups \
    && install -d -o 10001 -g 10001 -m 0700 /home/crmarques/.gitups/secrets

COPY bin/gitups /usr/local/bin/gitups

USER 10001
EOF
```

### 3. Start the controller shell

`--userns=keep-id` maps uid 10001 inside to your host uid so bind mounts are
readable. The state dir is mounted with the same path inside and outside the
container, so generated files remain accessible from the host afterwards.

```text
STATE_DIR=/tmp/gitups-libvirt-1-host-1-sno-hub
mkdir -p "$STATE_DIR"

podman run --rm -it \
  --network=host \
  --userns=keep-id:uid=10001,gid=10001 \
  -v "$HOME/.ssh/gitups-ssh-key":/home/crmarques/.ssh/gitups-ssh-key:ro,Z \
  -v "$HOME/.ssh/gitups-ssh-key.pub":/home/crmarques/.ssh/gitups-ssh-key.pub:ro,Z \
  -v "$HOME/.gitups/secrets/openshift-pull-secret":/home/crmarques/.gitups/secrets/openshift-pull-secret:ro,Z \
  -v "$HOME/.ssh/known_hosts":/home/crmarques/.ssh/known_hosts:ro,Z \
  -v "$(pwd)"/test/e2e:/gitups/test/e2e:ro,Z \
  -v /usr/local/bin/openshift-install:/usr/local/bin/openshift-install:Z \
  -v /usr/local/bin/kubectl:/usr/local/bin/kubectl:Z \
  -v /usr/local/bin/oc:/usr/local/bin/oc:Z \
  -v "$STATE_DIR:$STATE_DIR:Z" \
  -u crmarques \
    gitups-controller-4.21.10 \
      /bin/bash
```

### 4. Drive the install from inside the container

```text
gitups doctor check -f /gitups/test/e2e/libvirt-1-host-1-sno-hub/ --yes
gitups doctor fix -f /gitups/test/e2e/libvirt-1-host-1-sno-hub/ --yes

gitups secrets generate -f /gitups/test/e2e/libvirt-1-host-1-sno-hub/

gitups apply infra -f /gitups/test/e2e/libvirt-1-host-1-sno-hub/ --state-dir /tmp/gitups-libvirt-1-host-1-sno-hub --yes

gitups apply clusters -f /gitups/test/e2e/libvirt-1-host-1-sno-hub/ --state-dir /tmp/gitups-libvirt-1-host-1-sno-hub --yes
```

### 5. Tear down

```text
gitups destroy all -f /gitups/test/e2e/libvirt-1-host-1-sno-hub/ --state-dir /tmp/gitups-libvirt-1-host-1-sno-hub --yes
```

## Secrets used

| Name | Source |
| --- | --- |
| `openshift-pull-secret` | `gitups secrets pull-secret set` |
| `cluster-admin-key` | public half of the cluster SSH key |
| `remote-libvirt-host-admin-key` | private half of the provider SSH key |
| `libvirt-1-host-bmc-credentials` | generated by `gitups secrets generate` |
