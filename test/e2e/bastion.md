# Bastion Setup (VM Or Host)

Shared setup for e2e cases that run the Gitups controller directly on a Linux
VM or physical host (no container). See
[containerized-bastion.md](containerized-bastion.md) for the equivalent flow
inside a Podman container. Case READMEs reference this file and only cover
what is specific to their cluster shape.

## Bastion Requirements

The bastion is a Linux machine you SSH into and run `gitups` from. It must be
able to reach the provider host(s) over SSH.

- A non-root user with `sudo` on the bastion.
- `bin/gitups` available on the bastion.
- An OpenShift pull secret JSON at `~/.gitups/secrets/openshift-pull-secret`.
- A passwordless SSH key pair authorized on every provider host the case
  declares in `provider.yaml`.

If you build `bin/gitups` on a workstation, copy it to the bastion:

```bash
scp bin/gitups <bastion-user>@<bastion-host>:/usr/local/bin/gitups
scp ~/.gitups/secrets/openshift-pull-secret \
  <bastion-user>@<bastion-host>:/home/<bastion-user>/.gitups/secrets/openshift-pull-secret
```

(Adjust the destination paths to whatever the bastion user can read.)

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

## Optional Proxy For The Bastion

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

These variables only affect the no-state `gitups apply bastion` below.
`gitups apply bastion -f "$WORKSPACE"` strips ambient proxy variables and uses
`Environment.spec.proxy` instead.

## Gitups Env Vars

`$CASE` is the case directory name under `test/e2e/`. Set it once per shell:

```bash
export CASE=<case-directory>
export GITUPS_USER_DIR="$HOME/.gitups"
export GITUPS_STATE_DIR="$GITUPS_USER_DIR/state"
export GITUPS_SECRETS_DIR="$GITUPS_USER_DIR/secrets"
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
and proxy secret exist — see the case README.

## Teardown — Bastion State

After the case-specific `gitups destroy` steps, on the bastion:

```bash
rm -rf "$GITUPS_STATE_DIR/git-repos/clusters-bootstrap/$CASE" \
       "$GITUPS_STATE_DIR/runtime/$CASE"
```

The bastion machine itself stays — Gitups does not manage its lifecycle.
