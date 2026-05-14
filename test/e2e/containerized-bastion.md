# Containerized Bastion Setup

Run `bootwright` inside a non-root UBI9 container with `--network host` on the
operator's Linux host. Good for ephemeral, fully isolated e2e runs on a dev
workstation. For the VM/host alternative, see [bastion.md](bastion.md). The
case README points at this doc when the operator chooses the containerized
mode.

## Requirements

- Linux host with Podman.
- A non-root user that can SSH to `localhost` and escalate with `sudo`.
- `bin/bootwright` built from this repository (`make build`).
- An OpenShift pull secret at `~/.bootwright/secrets/openshift-pull-secret`.

```bash
command -v podman
sudo -v

install -d -m 0700 ~/.ssh
test -f ~/.ssh/bootwright-ssh-key || \
  ssh-keygen -t ed25519 -f ~/.ssh/bootwright-ssh-key -N '' -C bootwright-container-bastion
touch ~/.ssh/authorized_keys
chmod 0600 ~/.ssh/authorized_keys
grep -qxF "$(cat ~/.ssh/bootwright-ssh-key.pub)" ~/.ssh/authorized_keys || \
  cat ~/.ssh/bootwright-ssh-key.pub >> ~/.ssh/authorized_keys
ssh -i ~/.ssh/bootwright-ssh-key -o StrictHostKeyChecking=accept-new "$USER"@localhost true

test -s ~/.bootwright/secrets/openshift-pull-secret
make build
test -x bin/bootwright
```

Cases may add their own host requirements (for example `/dev/kvm` for a
local libvirt provider) — those are in the case README, not here.

## Optional Proxy Env For The Container Build

The container build picks up the standard process proxy environment. The
no-state `bootwright apply bastion --yes` (below, inside the container) also
honors these. The workspace-scoped `bootwright apply bastion -f` strips them and
uses `Environment.spec.proxy` from desired state instead. See
[proxy.md](proxy.md) for how to express the proxy in desired state.

Leave unset for direct internet access. Replace the placeholder URL before
exporting:

```bash
# export HTTP_PROXY=http://proxy.example.test:3128
# export HTTPS_PROXY=http://proxy.example.test:3128
# export NO_PROXY=localhost,127.0.0.1,::1,.bootwright.test
# export http_proxy="$HTTP_PROXY"
# export https_proxy="$HTTPS_PROXY"
# export no_proxy="$NO_PROXY"
```

## Build And Start The Bastion Container

`$CASE` is the case directory name under `test/e2e/`. Run from the repository
root.

```bash
export CASE=<case-directory>

BASTION_IMAGE=bootwright-bastion:$CASE
BASTION_NAME=bootwright-bastion-$CASE
BASTION_HOME="/home/$(id -un)"
E2E_USER_DIR="/tmp/.bootwright-e2e/$CASE"
PROXY_RUN_ARGS=()
if [ -n "${HTTP_PROXY:-}${HTTPS_PROXY:-}${NO_PROXY:-}${http_proxy:-}${https_proxy:-}${no_proxy:-}" ]; then
  PROXY_RUN_ARGS=(
    --env "HTTP_PROXY=${HTTP_PROXY:-}"
    --env "HTTPS_PROXY=${HTTPS_PROXY:-}"
    --env "NO_PROXY=${NO_PROXY:-}"
    --env "http_proxy=${http_proxy:-}"
    --env "https_proxy=${https_proxy:-}"
    --env "no_proxy=${no_proxy:-}"
  )
fi

podman rm -f "$BASTION_NAME" 2>/dev/null || true
rm -rf "$E2E_USER_DIR"
install -d -m 0700 "$E2E_USER_DIR/secrets"

podman build -t "$BASTION_IMAGE" \
  -f test/e2e/$CASE/Containerfile \
  --build-arg UID="$(id -u)" \
  --build-arg GID="$(id -g)" \
  --build-arg USER="$(id -un)" \
  --build-arg HTTP_PROXY="${HTTP_PROXY:-}" \
  --build-arg HTTPS_PROXY="${HTTPS_PROXY:-}" \
  --build-arg NO_PROXY="${NO_PROXY:-}" \
  --build-arg http_proxy="${http_proxy:-}" \
  --build-arg https_proxy="${https_proxy:-}" \
  --build-arg no_proxy="${no_proxy:-}" \
  .

podman run -dit --name "$BASTION_NAME" \
  --network host \
  --userns=keep-id \
  "${PROXY_RUN_ARGS[@]}" \
  -v "$HOME/.ssh:$BASTION_HOME/.ssh:Z" \
  -v "$E2E_USER_DIR:$BASTION_HOME/.bootwright:Z" \
  -v "$HOME/.bootwright/secrets/openshift-pull-secret:$BASTION_HOME/pull-secret.json:ro,Z" \
  -v "$PWD:/work:Z" \
  "$BASTION_IMAGE"

podman exec -it "$BASTION_NAME" bash
```

The repo is mounted read-write at `/work` inside the bastion.

## Inside The Bastion

Re-export `$CASE` after `podman exec`, then set the bootwright env vars the rest
of the case relies on. `BOOTWRIGHT_REPO` points to the repo mount, so subsequent
docs can reference `$BOOTWRIGHT_REPO/test/e2e/$CASE/`:

```bash
export CASE=<same value as on host>
export BOOTWRIGHT_USER_DIR="$HOME/.bootwright"
export BOOTWRIGHT_STATE_DIR="$BOOTWRIGHT_USER_DIR/state"
export BOOTWRIGHT_SECRETS_DIR="$BOOTWRIGHT_USER_DIR/secrets"
export BOOTWRIGHT_REPO=/work
```

## Bootstrap Bastion Dependencies

The first check is expected to report missing tools in a fresh container. The
apply installs the Bootwright-managed Ansible runtime. Release-specific OpenShift
CLIs are installed later by the workspace-scoped `bootwright apply bastion -f`,
because the release version comes from desired state.

```bash
bootwright check bastion || true
bootwright apply bastion --yes
bootwright check bastion || true
```

In an externally proxied environment, skip this no-state apply and run the
workspace-scoped `bootwright apply bastion -f "$WORKSPACE"` after the workspace
and proxy secret exist — see [common-steps.md](common-steps.md).

## Tear Down — Container And Host State

After the case-specific cluster destroy steps in
[common-steps.md](common-steps.md), exit the container and on the host:

```bash
podman rm -f "bootwright-bastion-$CASE"
rm -rf "/tmp/.bootwright-e2e/$CASE"
```
