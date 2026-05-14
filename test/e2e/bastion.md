# Bastion Setup (VM Or Host)

Run `bootwright` directly on a Linux VM or physical host. For the
non-root-container alternative, see
[containerized-bastion.md](containerized-bastion.md). The case README
points at this doc when the operator chooses the host-bastion mode.

## Requirements

The bastion is a Linux machine you SSH into and run `bootwright` from. It must be
able to reach every provider host the case declares in `provider.yaml` over
SSH (including `localhost` when the bastion is itself the provider host).

- A non-root user with `sudo`.
- `bin/bootwright` available in `$PATH`.
- An OpenShift pull secret at `~/.bootwright/secrets/openshift-pull-secret`.
- A passwordless SSH key pair authorized on every provider host.
- The case YAML files reachable on the bastion (see
  [Get The Case YAMLs Onto The Bastion](#get-the-case-yamls-onto-the-bastion)).

If you build `bin/bootwright` on a workstation, copy it across with the pull
secret:

```bash
scp bin/bootwright <bastion-user>@<bastion-host>:/usr/local/bin/bootwright
scp ~/.bootwright/secrets/openshift-pull-secret \
  <bastion-user>@<bastion-host>:/home/<bastion-user>/.bootwright/secrets/openshift-pull-secret
```

Adjust destination paths to whatever the bastion user can read.

## On The Bastion

SSH in and verify the basics:

```bash
ssh <bastion-user>@<bastion-host>

command -v bootwright
sudo -v
test -s ~/.bootwright/secrets/openshift-pull-secret
```

Generate (or reuse) the SSH key the cluster nodes will authorize and the
bastion will use to reach the provider host(s):

```bash
install -d -m 0700 ~/.ssh
test -f ~/.ssh/bootwright-ssh-key || \
  ssh-keygen -t ed25519 -f ~/.ssh/bootwright-ssh-key -N '' -C bootwright-bastion
```

Push the public key to every provider host listed in `provider.yaml`:

```bash
ssh-copy-id -i ~/.ssh/bootwright-ssh-key.pub <provider-user>@<provider-host>
ssh -i ~/.ssh/bootwright-ssh-key -o StrictHostKeyChecking=accept-new \
  <provider-user>@<provider-host> true
```

When the bastion is also the provider host (the default for the reference
cases), `<provider-host>` is `localhost`:

```bash
touch ~/.ssh/authorized_keys
chmod 0600 ~/.ssh/authorized_keys
grep -qxF "$(cat ~/.ssh/bootwright-ssh-key.pub)" ~/.ssh/authorized_keys || \
  cat ~/.ssh/bootwright-ssh-key.pub >> ~/.ssh/authorized_keys
ssh -i ~/.ssh/bootwright-ssh-key -o StrictHostKeyChecking=accept-new "$USER"@localhost true
```

## Get The Case YAMLs Onto The Bastion

The case README's "create workspace" step copies four YAML files from
`test/e2e/<case>/`. Put the repo on the bastion so they exist there, then
export `BOOTWRIGHT_REPO` to that location. Either clone the repo:

```bash
git clone <bootwright-repo-url> ~/bootwright
export BOOTWRIGHT_REPO="$HOME/bootwright"
```

…or `scp` only the case directory from your workstation:

```bash
scp -r test/e2e/<case> <bastion-user>@<bastion-host>:~/bootwright/test/e2e/<case>
# then on the bastion:
export BOOTWRIGHT_REPO="$HOME/bootwright"
```

## Optional Proxy Env For The No-State Bootstrap

Only the **no-state** `bootwright apply bastion --yes` below honors ambient proxy
environment variables. The workspace-scoped `bootwright apply bastion -f` strips
them and uses `Environment.spec.proxy` from desired state instead. See
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

## Bootwright Env Vars

`$CASE` is the case directory name under `test/e2e/`. Set it once per shell
together with the Bootwright state paths and the repo location:

```bash
export CASE=<case-directory>
export BOOTWRIGHT_USER_DIR="$HOME/.bootwright"
export BOOTWRIGHT_STATE_DIR="$BOOTWRIGHT_USER_DIR/state"
export BOOTWRIGHT_SECRETS_DIR="$BOOTWRIGHT_USER_DIR/secrets"
export BOOTWRIGHT_REPO=<absolute path to the repo on this bastion>
```

## Bootstrap Bastion Dependencies

The first check is expected to report missing tools on a fresh bastion. The
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

## Tear Down — Bastion State

After the case-specific cluster destroy steps in
[common-steps.md](common-steps.md), on the bastion:

```bash
rm -rf "$BOOTWRIGHT_STATE_DIR/git-repos/clusters-bootstrap/$CASE" \
       "$BOOTWRIGHT_STATE_DIR/runtime/$CASE"
```

The bastion machine itself stays — Bootwright does not manage its lifecycle.
