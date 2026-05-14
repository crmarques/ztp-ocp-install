# Bastion Setup (VM Or Host)

Run `gitups` directly on a Linux VM or physical host. For the
non-root-container alternative, see
[containerized-bastion.md](containerized-bastion.md). The case README
points at this doc when the operator chooses the host-bastion mode.

## Requirements

The bastion is a Linux machine you SSH into and run `gitups` from. It must be
able to reach every provider host the case declares in `provider.yaml` over
SSH (including `localhost` when the bastion is itself the provider host).

- A non-root user with `sudo`.
- `bin/gitups` available in `$PATH`.
- An OpenShift pull secret at `~/.gitups/secrets/openshift-pull-secret`.
- A passwordless SSH key pair authorized on every provider host.
- The case YAML files reachable on the bastion (see
  [Get The Case YAMLs Onto The Bastion](#get-the-case-yamls-onto-the-bastion)).

If you build `bin/gitups` on a workstation, copy it across with the pull
secret:

```bash
scp bin/gitups <bastion-user>@<bastion-host>:/usr/local/bin/gitups
scp ~/.gitups/secrets/openshift-pull-secret \
  <bastion-user>@<bastion-host>:/home/<bastion-user>/.gitups/secrets/openshift-pull-secret
```

Adjust destination paths to whatever the bastion user can read.

## On The Bastion

SSH in and verify the basics:

```bash
ssh <bastion-user>@<bastion-host>

command -v gitups
sudo -v
test -s ~/.gitups/secrets/openshift-pull-secret
```

Generate (or reuse) the SSH key the cluster nodes will authorize and the
bastion will use to reach the provider host(s):

```bash
install -d -m 0700 ~/.ssh
test -f ~/.ssh/gitups-ssh-key || \
  ssh-keygen -t ed25519 -f ~/.ssh/gitups-ssh-key -N '' -C gitups-bastion
```

Push the public key to every provider host listed in `provider.yaml`:

```bash
ssh-copy-id -i ~/.ssh/gitups-ssh-key.pub <provider-user>@<provider-host>
ssh -i ~/.ssh/gitups-ssh-key -o StrictHostKeyChecking=accept-new \
  <provider-user>@<provider-host> true
```

When the bastion is also the provider host (the default for the reference
cases), `<provider-host>` is `localhost`:

```bash
touch ~/.ssh/authorized_keys
chmod 0600 ~/.ssh/authorized_keys
grep -qxF "$(cat ~/.ssh/gitups-ssh-key.pub)" ~/.ssh/authorized_keys || \
  cat ~/.ssh/gitups-ssh-key.pub >> ~/.ssh/authorized_keys
ssh -i ~/.ssh/gitups-ssh-key -o StrictHostKeyChecking=accept-new "$USER"@localhost true
```

## Get The Case YAMLs Onto The Bastion

The case README's "create workspace" step copies four YAML files from
`test/e2e/<case>/`. Put the repo on the bastion so they exist there, then
export `GITUPS_REPO` to that location. Either clone the repo:

```bash
git clone <gitups-repo-url> ~/gitups
export GITUPS_REPO="$HOME/gitups"
```

…or `scp` only the case directory from your workstation:

```bash
scp -r test/e2e/<case> <bastion-user>@<bastion-host>:~/gitups/test/e2e/<case>
# then on the bastion:
export GITUPS_REPO="$HOME/gitups"
```

## Optional Proxy Env For The No-State Bootstrap

Only the **no-state** `gitups apply bastion --yes` below honors ambient proxy
environment variables. The workspace-scoped `gitups apply bastion -f` strips
them and uses `Environment.spec.proxy` from desired state instead. See
[proxy.md](proxy.md) for how to express the proxy in desired state.

Leave unset for direct internet access. Replace the placeholder URL before
exporting:

```bash
# export HTTP_PROXY=http://proxy.example.test:3128
# export HTTPS_PROXY=http://proxy.example.test:3128
# export NO_PROXY=localhost,127.0.0.1,::1,.gitups.test
# export http_proxy="$HTTP_PROXY"
# export https_proxy="$HTTPS_PROXY"
# export no_proxy="$NO_PROXY"
```

## Gitups Env Vars

`$CASE` is the case directory name under `test/e2e/`. Set it once per shell
together with the Gitups state paths and the repo location:

```bash
export CASE=<case-directory>
export GITUPS_USER_DIR="$HOME/.gitups"
export GITUPS_STATE_DIR="$GITUPS_USER_DIR/state"
export GITUPS_SECRETS_DIR="$GITUPS_USER_DIR/secrets"
export GITUPS_REPO=<absolute path to the repo on this bastion>
```

## Bootstrap Bastion Dependencies

The first check is expected to report missing tools on a fresh bastion. The
apply installs the Gitups-managed Ansible runtime. Release-specific OpenShift
CLIs are installed later by the workspace-scoped `gitups apply bastion -f`,
because the release version comes from desired state.

```bash
gitups check bastion || true
gitups apply bastion --yes
gitups check bastion || true
```

In an externally proxied environment, skip this no-state apply and run the
workspace-scoped `gitups apply bastion -f "$WORKSPACE"` after the workspace
and proxy secret exist — see [common-steps.md](common-steps.md).

## Tear Down — Bastion State

After the case-specific cluster destroy steps in
[common-steps.md](common-steps.md), on the bastion:

```bash
rm -rf "$GITUPS_STATE_DIR/git-repos/clusters-bootstrap/$CASE" \
       "$GITUPS_STATE_DIR/runtime/$CASE"
```

The bastion machine itself stays — Gitups does not manage its lifecycle.
