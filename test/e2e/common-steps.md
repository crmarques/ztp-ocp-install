# Common Steps

Identical for every case once you have a bastion, a workspace, and the
desired-state files edited. The case README drops you here after
"Customize Desired State"; it returns at the very end for the bastion-side
teardown.

These steps assume the env vars from the bastion doc are exported:
`$CASE`, `$GITUPS_USER_DIR`, `$GITUPS_STATE_DIR`, `$GITUPS_SECRETS_DIR`,
`$GITUPS_REPO`, plus `$WORKSPACE` from the case README.

## 1. Save And Generate Secrets

The case workspace references four or five secrets through
`environment.yaml` `spec.secrets:`:

| Secret | Form | Required for |
| --- | --- | --- |
| `gitups-ssh-key` / `.pub` | File under `~/.ssh` (created during bastion setup) | Cluster SSH key, bastion→host SSH |
| `openshift-pull-secret` | Set from the pull-secret JSON | `render installer`, `apply clusters` |
| `proxy-credentials` (optional) | Set (file-backed) or generated — see [proxy.md](proxy.md) | `apply bastion -f`, install-config proxy block |
| `bmc-credentials` | Generated | `apply infra`, `apply clusters` |

Confirm the SSH key pair, then set the pull secret:

```bash
test -r ~/.ssh/gitups-ssh-key
test -r ~/.ssh/gitups-ssh-key.pub

gitups secret set openshift-pull-secret \
  --pull-secret "$HOME/pull-secret.json" \
  --secrets-dir "$GITUPS_SECRETS_DIR"
```

For the containerized bastion the pull secret is mounted at
`$HOME/pull-secret.json`. For a host bastion, point `--pull-secret` at
wherever you placed the JSON (typically
`~/.gitups/secrets/openshift-pull-secret`).

If `environment.yaml` declares `proxy-credentials` in the **file-backed**
form (see [proxy.md](proxy.md)), write it now (skip for the **generated**
form):

```bash
gitups secret set proxy-credentials \
  --username <proxy-user> \
  --password-stdin \
  --secrets-dir "$GITUPS_SECRETS_DIR"
```

Materialize every `generated:` entry (`bmc-credentials` always;
`proxy-credentials` if generated):

```bash
gitups secret generate -f "$WORKSPACE" --secrets-dir "$GITUPS_SECRETS_DIR"
find "$GITUPS_SECRETS_DIR" -maxdepth 1 -type f -printf '%f\n' | sort
```

## 2. Apply The Workspace To The Bastion

Installs release-specific OpenShift CLIs declared by the workspace.
`apply bastion -f` strips ambient proxy variables and uses
`Environment.spec.proxy` from desired state.

```bash
gitups apply bastion -f "$WORKSPACE" --yes
gitups check bastion -f "$WORKSPACE"
```

Managed Squid is **not** running yet at this point — the bastion phase
runs before `apply infra` provisions it. Gitups deliberately ignores
`spec.proxy` for this phase; every later phase routes through Squid once
infra is up. See [proxy.md](proxy.md) for the full bootstrap order.

## 3. Provision The Provider Host

Read-only preflight, then converge: base packages, libvirt, the cluster
bridge, the managed load balancer (HAProxy by default — see
[load-balancer.md](load-balancer.md)), managed host-file entries, the
Redfish BMC emulator, and managed Squid (if declared).

```bash
gitups check infra -f "$WORKSPACE"
gitups apply infra -f "$WORKSPACE" --dry-run
gitups apply infra -f "$WORKSPACE" --yes
gitups check infra -f "$WORKSPACE"
```

If the provider-host user has passwordless sudo, add
`--ask-become-pass=false` to the two `apply` commands.

## 4. Install The Cluster

`render installer` writes `install-config.yaml` and `agent-config.yaml`
under `git-repos/clusters-bootstrap/<cluster>/openshift/` with placeholder
strings in place of pull secret, SSH key, and trust bundle (this tree is
the GitOps-publishable declarative source). `--resolve-secrets` also writes
`runtime/<cluster>/installer/{install,agent}-config.yaml` with secret
material inlined (mode `0600`) — the form `openshift-install` consumes.
The runtime tree never leaves local state.

```bash
gitups check clusters -f "$WORKSPACE"
gitups render installer -f "$WORKSPACE" --scope "$CASE" --resolve-secrets

gitups apply clusters -f "$WORKSPACE" --dry-run
gitups apply clusters -f "$WORKSPACE" --yes
```

`apply clusters` regenerates the runtime installer copies, renders the
agent installer assets, boots every cluster node through the emulated
Redfish BMC, and waits for
`openshift-install agent wait-for install-complete`.

## 5. Verify

```bash
export KUBECONFIG="$GITUPS_STATE_DIR/clusters/$CASE/auth/kubeconfig"

oc get nodes
oc get clusterversion
oc get clusteroperators

gitups check infra -f "$WORKSPACE"
gitups check clusters -f "$WORKSPACE"
```

The case README lists the per-case expectation for `oc get nodes`
(typically one `Ready` node for SNO, three for the 3-node case).

## 6. Tear Down The Cluster

```bash
gitups destroy clusters -f "$WORKSPACE" --yes
gitups destroy infra -f "$WORKSPACE" --yes
```

After this, return to the bastion doc you used:

- [bastion.md](bastion.md#tear-down--bastion-state) — clean per-case state
  dirs on the bastion.
- [containerized-bastion.md](containerized-bastion.md#tear-down--container-and-host-state)
  — remove the container and per-case host state.
