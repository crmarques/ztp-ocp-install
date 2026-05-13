# Container Bastion + Local Libvirt SNO

This case provisions one OpenShift SNO cluster on the local libvirt host while
the Gitups controller runs inside a UBI9 container. The default input is a
direct connected install; the same case can run through an explicit forward
proxy by declaring it in Gitups desired state.

Gitups owns dependency convergence for the bastion runtime and the provider
host.

## Shape

| Piece | Value |
| --- | --- |
| Bastion | UBI9 container, host UID/GID, non-root user |
| Provider host | Same machine, reached by SSH as `localhost` through `--network host` |
| Cluster | SNO on libvirt bridge `vbr-cb-sno` |
| Network | `192.168.132.0/24`, API/API-int `.10`, Ingress `.11`, node `.20` |
| Install mode | Connected direct by default; routes through a proxy when `Environment.spec.proxy` is set |

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

## Optional Proxy Setup

Leave this section unset for direct internet access.

The container build uses the standard process proxy environment. Replace the
placeholder URL before exporting these variables; do not copy
`proxy.example.test` as-is.

```bash
# export HTTP_PROXY=http://proxy.example.test:3128
# export HTTPS_PROXY=http://proxy.example.test:3128
# export NO_PROXY=localhost,127.0.0.1,::1,.gitups.test,192.168.132.0/24,10.128.0.0/14,172.30.0.0/16
# export http_proxy="$HTTP_PROXY"
# export https_proxy="$HTTPS_PROXY"
# export no_proxy="$NO_PROXY"
```

`gitups apply bastion` strips ambient proxy variables and uses
`Environment.spec.proxy` only when desired state is passed with `-f`. In
proxied environments, create the workspace and proxy secret first, then run
`gitups apply bastion -f "$WORKSPACE" --yes`. Use bare proxy URLs in desired
state. If the proxy requires authentication, set `auth.proxyAuthRef.name` and
write the credentials with `gitups secret set`; do not embed credentials in the
URL.

## Start A Fresh Bastion

Run these commands from the repository root on the host.

```bash
CASE=container-bastion-local-libvirt-sno
BASTION_IMAGE=gitups-bastion:$CASE
BASTION_NAME=gitups-bastion-$CASE
BASTION_HOME="/home/$(id -un)"
E2E_USER_DIR="/tmp/.gitups-e2e/$CASE"
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
  -f test/e2e/container-bastion-local-libvirt-sno/Containerfile \
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
version comes from desired state. In an externally proxied environment, skip the
no-state apply here and run the workspace-scoped bastion apply after
`environment.yaml` and the proxy secret exist.

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
| `environment.yaml` | `spec.baseDomain`, OpenShift release, optional proxy, and secret sources under `spec.secrets` |
| `provider.yaml` | `spec.hosts.lab-host.ssh.address`, optional `ssh.user`, `libvirtURI`, BMC port |
| `infra.yaml` | CIDR, bridge name, VIPs, node IP, and MAC address |

For the standard local-container path, leave `provider.yaml` with
`ssh.address: localhost` and no `ssh.user`; Gitups defaults the SSH user to the
current bastion user, which matches the host user created in the image.

For a proxied OpenShift install, add a top-level `proxy:` block to
`environment.yaml` alongside `ocpInstallType: connected`:

```yaml
proxy:
  http: http://proxy.example.test:3128
  https: http://proxy.example.test:3128
  noProxy:
    - 192.168.132.0/24
```

Gitups auto-extends `noProxy` with cluster-local endpoints (service/cluster
CIDRs, `.svc`, `.cluster.local`, `localhost`, the base domain, mirror registry
host, provider host addresses) — only user-specific entries need to be listed.

If the proxy requires authentication, add an auth ref to that block and a
matching entry under `spec.secrets`. Pick one of these two forms.

**File-backed** — you write the credentials yourself with `gitups secret set`:

```yaml
proxy:
  auth:
    proxyAuthRef:
      name: proxy-credentials
secrets:
  proxy-credentials:
    file: ~/.gitups/secrets/proxy-credentials
```

**Generated** — `gitups secret generate` materializes the file. The password
is auto-generated; the username defaults to `admin` if `username:` is omitted:

```yaml
proxy:
  auth:
    proxyAuthRef:
      name: proxy-credentials
secrets:
  proxy-credentials:
    generated:
      credentials:
        username: proxy
```

Materialize the secret with `gitups secret set` (file-backed form) or `gitups
secret generate` (generated form) in the next section before
`gitups apply bastion -f "$WORKSPACE"` runs.

### Provision The Proxy With Gitups

Skip this subsection if you already have an externally-hosted proxy. The
preceding `proxy:` block on its own assumes the proxy URL is already
reachable.

To have Gitups stand up an authenticated Squid proxy on the provider host
instead, declare it on `provider.yaml`:

```yaml
spec:
  hosts:
    lab-host:
      capabilities:
        - libvirt
        - container-runtime
        - hosts-file
        - proxy
  proxy:
    squid:
      hostRef:
        name: lab-host
      # port: 3128       # default
      # runtime: podman  # default
```

Requirements when `spec.proxy.squid` is set:

- The referenced host must carry the `proxy` capability and an `ssh` block.
- `environment.yaml` `spec.proxy.http` and `spec.proxy.https` must be bare
  `http://` URLs and include the same port as `spec.proxy.squid.port`
  (default `3128`).
- `environment.yaml` `spec.proxy.auth.proxyAuthRef` is mandatory — managed
  Squid is always authenticated. Use either the file-backed or generated
  form shown above; declare the corresponding `spec.secrets` entry.
- For libvirt clusters, the libvirt machine `hostRef` must match
  `spec.proxy.squid.hostRef` (v1 keeps the proxy and the VMs on the same
  host so isolated networks remain reachable). The primary machine network
  must declare a `gateway` so VMs route egress through the managed proxy.

Bastion bootstrap (`gitups apply bastion`) runs *before* the managed proxy
exists, so Gitups deliberately ignores `environment.yaml` `spec.proxy` for
that step. Once `gitups apply infra` provisions Squid, every subsequent
phase (provider-host package/image pulls, install-config rendering, agent
install) routes through it.

Hosts and VMs reach managed Squid at different addresses. Gitups renders
two client URLs:

- **Host URL** — `http://<proxy.squid.hostRef SSH address>:<port>`. Written
  by `host_proxy` into `/etc/dnf/dnf.conf`, `/etc/environment`, the systemd
  drop-in, and `pip.conf`. Routable on every host before libvirt is
  installed, so the very first dnf install (which installs libvirt itself)
  succeeds via the proxy.
- **VM URL** — `http://<machineNetwork.gateway>:<port>`. Embedded in
  `install-config.yaml` so the OpenShift cluster sends runtime egress
  through the libvirt-bridge gateway, where Squid (bound via host
  networking) answers. The bridge becomes a local address only after
  `substrate_libvirt` brings up the libvirt network — which is too late
  for the host-side dnf installs the host URL handles.

For external proxies (no `spec.proxy.squid`) both URLs collapse to the
same user-configured `spec.proxy.http` / `spec.proxy.https`, so the split
is invisible.

## Save And Generate Secrets

Secret material the workspace consumes:

| Secret | Form | Required for |
| --- | --- | --- |
| `gitups-ssh-key` / `.pub` | File under `~/.ssh` (already created in operator inputs) | Cluster SSH key, bastion→host SSH |
| `openshift-pull-secret` | Set from the pull-secret JSON | `render installer`, `apply clusters` |
| `proxy-credentials` (optional) | Set (file-backed form) or generated | `apply bastion -f`, install-config proxy block |
| `bmc-credentials` | Generated from `environment.yaml` | `apply infra`, `apply clusters` |

Run secret commands in this order, before `apply bastion -f "$WORKSPACE"`.

1. Confirm the SSH key pair from the operator-inputs step is readable from
   inside the bastion:

   ```bash
   test -r ~/.ssh/gitups-ssh-key
   test -r ~/.ssh/gitups-ssh-key.pub
   ```

2. Save the OpenShift pull secret:

   ```bash
   gitups secret set openshift-pull-secret \
     --pull-secret $BASTION_HOME/pull-secret.json \
     --secrets-dir "$GITUPS_SECRETS_DIR"
   ```

3. If `environment.yaml` declares `proxy-credentials` in the **file-backed**
   form, write it now (skip this step for the **generated** form — step 4
   creates it):

   ```bash
   gitups secret set proxy-credentials \
     --username <proxy-user> \
     --password-stdin \
     --secrets-dir "$GITUPS_SECRETS_DIR"
   ```

4. Materialize every `generated:` key declared in `environment.yaml`
   (`bmc-credentials` always, `proxy-credentials` if its key is in generated
   form):

   ```bash
   gitups secret generate \
     -f "$WORKSPACE" \
     --secrets-dir "$GITUPS_SECRETS_DIR"
   ```

5. List the secrets directory to confirm what's on disk:

   ```bash
   find "$GITUPS_SECRETS_DIR" -maxdepth 1 -type f -printf '%f\n' | sort
   ```

Expected files (plus `proxy-credentials` when `environment.yaml` references
it):

```bash
bmc-credentials
openshift-pull-secret
proxy-credentials
```

## Apply The Workspace To The Bastion

With desired state and secrets in place, install the release-specific
OpenShift CLIs declared by the workspace:

```bash
gitups apply bastion -f "$WORKSPACE" --yes
gitups check bastion -f "$WORKSPACE"
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

if grep -q '^proxy:' "$GITUPS_STATE_DIR/clusters-bootstrap.git/$CASE/openshift/install-config.yaml"; then
  grep -A6 '^proxy:' "$GITUPS_STATE_DIR/clusters-bootstrap.git/$CASE/openshift/install-config.yaml"
  grep -A6 '^proxy:' "$GITUPS_STATE_DIR/clusters-bootstrap.git/$CASE/openshift/work/install-config.yaml"
fi
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
