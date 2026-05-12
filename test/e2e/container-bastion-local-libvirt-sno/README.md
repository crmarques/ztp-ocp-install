# Container Bastion + Local Libvirt SNO

This case provisions one connected OpenShift SNO cluster on the local libvirt
host while the Gitups controller runs inside a UBI9 container. The bastion
container runs as the same non-root user as the host account and uses Ansible
become when Gitups needs root privileges.

Gitups owns dependency convergence for the bastion runtime and the provider
host.

## Shape

| Piece | Value |
| --- | --- |
| Bastion | UBI9 container, host UID/GID, non-root user |
| Provider host | Same machine, reached by SSH as `localhost` through `--network host` |
| Cluster | SNO on libvirt bridge `vbr-cb-sno` |
| Network | `192.168.132.0/24`, API/API-int `.10`, Ingress `.11`, node `.20` |
| Install mode | Connected OpenShift install |

## Operator Inputs

These are the only things this README expects you to provide before Gitups can
take over:

- A Linux host with hardware virtualization exposed at `/dev/kvm`.
- A container runtime that can start the bastion container. The commands below
  use Podman.
- A non-root host user that can SSH to `localhost` and can escalate with sudo
  on the host.
- An OpenShift pull secret JSON from the Red Hat console.
- A built `bin/gitups` from this repository.

Check those inputs from the host:

```bash
test -c /dev/kvm
command -v podman
sudo -v

install -d -m 0700 ~/.ssh
test -f ~/.ssh/gitups-ssh-key || \
  ssh-keygen -t ed25519 -f ~/.ssh/gitups-ssh-key -N '' -C gitups-container-bastion
touch ~/.ssh/authorized_keys
chmod 0600 ~/.ssh/authorized_keys
grep -qxF "$(cat ~/.ssh/gitups-ssh-key.pub)" ~/.ssh/authorized_keys || \
  cat ~/.ssh/gitups-ssh-key.pub >> ~/.ssh/authorized_keys
ssh -i ~/.ssh/gitups-ssh-key -o StrictHostKeyChecking=accept-new "$USER"@localhost true

test -s ~/.gitups/secrets/openshift-pull-secret
make build
test -x bin/gitups
```

If one of these checks fails, fix that host capability or secret first. Host
package selection for the provider stack is intentionally left to Gitups.

## Start A Fresh Bastion

Run these commands from the repository root on the host.

```bash
CASE=container-bastion-local-libvirt-sno
BASTION_IMAGE=gitups-bastion:$CASE
BASTION_NAME=gitups-bastion-$CASE
BASTION_HOME="/home/$(id -un)"
E2E_USER_DIR="/tmp/.gitups-e2e/$CASE"

podman rm -f "$BASTION_NAME" 2>/dev/null || true
rm -rf "$E2E_USER_DIR"
install -d -m 0700 "$E2E_USER_DIR/secrets"

podman build -t "$BASTION_IMAGE" \
  -f test/e2e/container-bastion-local-libvirt-sno/Containerfile \
  --build-arg UID="$(id -u)" \
  --build-arg GID="$(id -g)" \
  --build-arg USER="$(id -un)" \
  .

podman run -dit --name "$BASTION_NAME" \
  --network host \
  --userns=keep-id \
  -v "$HOME/.ssh:$BASTION_HOME/.ssh:Z" \
  -v "$E2E_USER_DIR:$BASTION_HOME/.gitups:Z" \
  -v "$HOME/.gitups/secrets/openshift-pull-secret:$BASTION_HOME/pull-secret.json:ro,Z" \
  -v "$PWD:/work:Z" \
  "$BASTION_IMAGE"

podman exec -it "$BASTION_NAME" bash
```

The remaining commands run inside the bastion container.

```bash
CASE=container-bastion-local-libvirt-sno
export GITUPS_USER_DIR="$HOME/.gitups"
export GITUPS_STATE_DIR="$GITUPS_USER_DIR/state"
export GITUPS_SECRETS_DIR="$GITUPS_USER_DIR/secrets"
WORKSPACE="$GITUPS_STATE_DIR/clusters-bootstrap.git/$CASE/gitups"
BASTION_HOME="/home/$(id -un)"
```

## Bootstrap Bastion Dependencies

The first check is expected to report missing tools in a fresh container. The
apply command installs the Gitups-managed Ansible runtime. Release-specific
OpenShift CLIs are installed after the workspace exists, because the release
version comes from desired state.

```bash
gitups check bastion || true
gitups apply bastion --yes
gitups check bastion || true
```

## Create And Edit Desired State

Generate the workspace, then copy the reference files for this e2e case. You
can edit the generated files by hand instead; the copied files are just the
known-good target state for the local-container layout.

```bash
gitups init workspace \
  --cluster-name "$CASE" \
  --provider emulated-bare-metal

cp /work/test/e2e/container-bastion-local-libvirt-sno/{environment,provider,infra,cluster}.yaml \
  "$WORKSPACE/"

vi "$WORKSPACE/environment.yaml" "$WORKSPACE/provider.yaml" "$WORKSPACE/infra.yaml"
```

Review these user-specific fields before continuing:

| File | Field |
| --- | --- |
| `environment.yaml` | `spec.baseDomain`, OpenShift release, and key paths under `spec.keys` |
| `provider.yaml` | `spec.hosts.lab-host.ssh.address`, optional `ssh.user`, `libvirtURI`, BMC port |
| `infra.yaml` | CIDR, bridge name, VIPs, node IP, and MAC address |

For the standard local-container path, leave `provider.yaml` with
`ssh.address: localhost` and no `ssh.user`; Gitups defaults the SSH user to the
current bastion user, which matches the host user created in the image.

Now install the release-specific OpenShift CLIs declared by the workspace:

```bash
gitups apply bastion -f "$WORKSPACE" --yes
gitups check bastion -f "$WORKSPACE"
```

## Save And Generate Secrets

The SSH files are file-backed secret references under `~/.ssh`; the pull
secret and generated BMC credentials live under `$GITUPS_SECRETS_DIR`.

```bash
test -r ~/.ssh/gitups-ssh-key
test -r ~/.ssh/gitups-ssh-key.pub

gitups secret set openshift-pull-secret \
  --pull-secret $BASTION_HOME/pull-secret.json \
  --secrets-dir "$GITUPS_SECRETS_DIR"

gitups secret generate \
  -f "$WORKSPACE" \
  --secrets-dir "$GITUPS_SECRETS_DIR"

find "$GITUPS_SECRETS_DIR" -maxdepth 1 -type f -printf '%f\n' | sort
```

Expected generated/saved files:

```bash
bmc-credentials
openshift-pull-secret
```

## Check The Local Libvirt Provider

This is a read-only provider preflight over SSH to the host. It verifies the
provider connection, sudo/become path, KVM device, package manager, and local
port availability before anything is changed.

```bash
gitups check infra -f "$WORKSPACE"
```

## Install Infra Components

`apply infra` converges the local provider host: base packages, libvirt,
the cluster bridge, managed HAProxy, managed host-file entries, and the
Redfish BMC emulator. Mutating provider-host phases run with Ansible become.

Use the default command if sudo prompts for a password:

```bash
gitups apply infra -f "$WORKSPACE" --dry-run
gitups apply infra -f "$WORKSPACE" --yes
gitups check infra -f "$WORKSPACE"
```

If the host user has passwordless sudo, add `--ask-become-pass=false` to the
two `apply` commands.

## Install The SNO Cluster

```bash
gitups check clusters -f "$WORKSPACE"
gitups render installer -f "$WORKSPACE" --scope "$CASE"
```

`render installer` writes `install-config.yaml` and `agent-config.yaml` with
placeholder strings in place of pull secret, SSH key, and trust bundle under
`$GITUPS_STATE_DIR/clusters-bootstrap.git/$CASE/openshift/`. Those files are
safe to inspect and never carry credentials. Add `--resolve-secrets` to also
write `openshift/work/install-config.yaml` and `openshift/work/agent-config.yaml`
with secret material inlined (mode `0600`) — that is the form
`openshift-install` consumes:

```bash
gitups render installer -f "$WORKSPACE" --scope "$CASE" --resolve-secrets
```

```bash
gitups apply clusters -f "$WORKSPACE" --dry-run
gitups apply clusters -f "$WORKSPACE" --yes
```

`apply clusters` regenerates the `work/` copies regardless, then renders the
agent installer assets, boots `master-0` through the emulated Redfish BMC, and
waits for `openshift-install agent wait-for install-complete`.

## Verify

```bash
export KUBECONFIG="$GITUPS_STATE_DIR/clusters/$CASE/auth/kubeconfig"

oc get nodes
oc get clusterversion
oc get clusteroperators

gitups check infra -f "$WORKSPACE"
gitups check clusters -f "$WORKSPACE"
gitups status --state-dir "$GITUPS_STATE_DIR"
```

`status` is a workspace summary and may still print generic next-step hints.
For this case, the authoritative post-install checks are the live `oc` output
and the targeted `check infra` / `check clusters` commands above.

## Tear Down

Run destroy from inside the bastion:

```bash
gitups destroy clusters -f "$WORKSPACE" --yes
gitups destroy infra -f "$WORKSPACE" --yes
exit
```

Then remove only this case's container and state from the host:

```bash
CASE=container-bastion-local-libvirt-sno
podman rm -f "gitups-bastion-$CASE"
rm -rf "$HOME/.gitups-e2e/$CASE"
```
