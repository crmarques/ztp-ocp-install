# Common Steps

Identical for every case once you have a bastion, a workspace, and the
desired-state files edited. The case README drops you here after
"Customize Desired State"; it returns at the very end for the bastion-side
teardown.

These steps assume the env vars from the bastion doc are exported:
`$CASE`, `$BOOTWRIGHT_USER_DIR`, `$BOOTWRIGHT_STATE_DIR`, `$BOOTWRIGHT_SECRETS_DIR`,
`$BOOTWRIGHT_REPO`, plus `$WORKSPACE` from the case README.

## 1. Save And Generate Secrets

The case workspace references four or five secrets through
`environment.yaml` `spec.secrets:`:

| Secret | Form | Required for |
| --- | --- | --- |
| `bootwright-ssh-key` / `.pub` | File under `~/.ssh` (created during bastion setup) | Cluster SSH key, bastion→host SSH |
| `openshift-pull-secret` | Set from the pull-secret JSON | `render installer`, `apply clusters` |
| `proxy-credentials` (optional) | Set (file-backed) or generated — see [proxy.md](proxy.md) | `apply bastion -f`, install-config proxy block |
| `bmc-credentials` | Generated | `apply infra`, `apply clusters` |

Confirm the SSH key pair, then set the pull secret:

```bash
test -r ~/.ssh/bootwright-ssh-key
test -r ~/.ssh/bootwright-ssh-key.pub

bootwright secret set openshift-pull-secret \
  --pull-secret "$HOME/pull-secret.json" \
  --secrets-dir "$BOOTWRIGHT_SECRETS_DIR"
```

For the containerized bastion the pull secret is mounted at
`$HOME/pull-secret.json`. For a host bastion, point `--pull-secret` at
wherever you placed the JSON (typically
`~/.bootwright/secrets/openshift-pull-secret`).

If `environment.yaml` declares `proxy-credentials` in the **file-backed**
form (see [proxy.md](proxy.md)), write it now (skip for the **generated**
form):

```bash
bootwright secret set proxy-credentials \
  --username <proxy-user> \
  --password-stdin \
  --secrets-dir "$BOOTWRIGHT_SECRETS_DIR"
```

Materialize every `generated:` entry (`bmc-credentials` always;
`proxy-credentials` if generated):

```bash
bootwright secret generate -f "$WORKSPACE" --secrets-dir "$BOOTWRIGHT_SECRETS_DIR"
find "$BOOTWRIGHT_SECRETS_DIR" -maxdepth 1 -type f -printf '%f\n' | sort
```

## 2. Apply The Workspace To The Bastion

Installs release-specific OpenShift CLIs declared by the workspace.
`apply bastion -f` strips ambient proxy variables and uses
`Environment.spec.proxy` from desired state.

```bash
bootwright apply bastion -f "$WORKSPACE" --yes
bootwright check bastion -f "$WORKSPACE"
```

Managed Squid is **not** running yet at this point — the bastion phase
runs before `apply infra` provisions it. Bootwright deliberately ignores
`spec.proxy` for this phase; every later phase routes through Squid once
infra is up. See [proxy.md](proxy.md) for the full bootstrap order.

## 3. Provision The Infrastructure

Read-only preflight, then converge the `InfrastructureProvider` and the
`ClusterInfrastructure` it owns: cluster networks, machine infrastructure,
the managed load balancer (see [load-balancer.md](load-balancer.md)),
name resolution, the machine-control integration (Redfish BMC for
baremetal, libvirt for libvirt, hypervisor API for OpenShift
Virtualization / ESXi), and managed Squid (see [proxy.md](proxy.md))
when declared. The case's `provider.yaml` and `infra.yaml` decide which
of these apply.

```bash
bootwright check infra -f "$WORKSPACE"
bootwright apply infra -f "$WORKSPACE" --dry-run
bootwright apply infra -f "$WORKSPACE" --yes
bootwright check infra -f "$WORKSPACE"
```

If the provider integration drives Ansible over SSH (libvirt, bare
metal) and the target SSH user has passwordless sudo, add
`--ask-become-pass=false` to the two `apply` commands. Providers that
talk to an API (OpenShift Virtualization, ESXi) skip this prompt.

## 4. Install The Cluster

`render installer` writes `install-config.yaml` and `agent-config.yaml`
under `git-repos/clusters-bootstrap/<cluster>/openshift/` with
placeholder strings in place of pull secret, SSH key, and trust bundle.
This tree is the GitOps-publishable declarative source — safe to
commit.

```bash
bootwright check clusters -f "$WORKSPACE"
bootwright render installer -f "$WORKSPACE"

bootwright apply clusters -f "$WORKSPACE" --dry-run
bootwright apply clusters -f "$WORKSPACE" --yes
```

`apply clusters` materializes
`runtime/<cluster>/installer/{install,agent}-config.yaml` with secret
material inlined (mode `0600`) — the form `openshift-install` consumes.
The runtime tree never leaves local state. It then renders the agent
installer assets, boots every cluster node through the provider's
machine-control path (Redfish/IPMI BMC for baremetal, libvirt for
libvirt, hypervisor API for OpenShift Virtualization / ESXi), and waits
for `openshift-install agent wait-for install-complete`.

To see the final form `openshift-install` will consume, re-run `render
installer` with `--resolve-secrets`:

```bash
bootwright render installer -f "$WORKSPACE" --resolve-secrets
```

That writes the runtime copies eagerly so you can review them. It is
**not** required for the install — `apply clusters` regenerates the
same runtime copies on its own. Skip it when you want the rendered
files to stay free of secret material (for example, before checking
them into a GitOps repo).

### Following The Install Logs

`bootwright apply clusters` is one long-running command. Its Ansible output
streams to the foreground terminal; the `openshift-install agent
wait-for install-complete` phase that gates the run writes a richer log
to disk. Open a second shell on the bastion to follow it:

```bash
tail -f "$BOOTWRIGHT_STATE_DIR/runtime/$CASE/installer/.openshift_install.log"
```

For node-side visibility, SSH to a booted control plane (IPs are in
`infra.yaml` under `spec.machines.<name>.interfaces.primary.ipAddress`)
and watch the agent / bootkube journals:

```bash
ssh -i ~/.ssh/bootwright-ssh-key core@<node-ip> \
  sudo journalctl -fu assisted-service.service
# or, after bootstrap kicks off:
ssh -i ~/.ssh/bootwright-ssh-key core@<node-ip> \
  sudo journalctl -fu bootkube.service
```

## 5. Verify

```bash
export KUBECONFIG="$BOOTWRIGHT_STATE_DIR/clusters/$CASE/auth/kubeconfig"

oc get nodes
oc get clusterversion
oc get clusteroperators

bootwright check infra -f "$WORKSPACE"
bootwright check clusters -f "$WORKSPACE"
```

The case README lists the per-case expectation for `oc get nodes`
(typically one `Ready` node for SNO, three for the 3-node case).

## 6. Tear Down The Cluster

```bash
bootwright destroy clusters -f "$WORKSPACE" --yes
bootwright destroy infra -f "$WORKSPACE" --yes
```

After this, return to the bastion doc you used:

- [bastion.md](bastion.md#tear-down--bastion-state) — clean per-case state
  dirs on the bastion.
- [containerized-bastion.md](containerized-bastion.md#tear-down--container-and-host-state)
  — remove the container and per-case host state.
