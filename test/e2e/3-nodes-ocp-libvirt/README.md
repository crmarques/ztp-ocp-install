# 3-Node OCP On Libvirt (Containerized Bastion)

Provisions one 3-node compact OpenShift cluster (3 control-plane nodes, no
dedicated workers) on the local libvirt host while the Gitups controller runs
inside a UBI9 container. Default is a direct connected install; the same case
can route through an explicit forward proxy or a Gitups-managed Squid by
declaring it in desired state.

## Shape

| Piece | Value |
| --- | --- |
| Bastion | UBI9 container, host UID/GID, non-root user |
| Provider host | Same machine, reached by SSH as `localhost` through `--network host` |
| Cluster | 3 control-plane nodes on libvirt bridge `vbr-cb-3n` |
| Network | `192.168.133.0/24`, API/API-int `.10`, Ingress `.11`, nodes `.20`-`.22` |
| Install mode | Connected direct by default; routes through a proxy when `Environment.spec.proxy` is set |

## Case-Specific Operator Input

In addition to the [shared host requirements](../containerized-bastion.md#host-requirements),
this case needs hardware virtualization exposed on the host:

```bash
test -c /dev/kvm
```

Three VMs are launched on the same host, each sized by the
`compact-control-plane` profile (`provider.yaml`). Confirm the lab host has
enough headroom (default: 4 vCPU + 16 GiB RAM + 120 GiB disk per node).

## Bring Up The Bastion

```bash
CASE=3-nodes-ocp-libvirt
```

Then follow the [shared bastion-container setup](../containerized-bastion.md):

1. Optional proxy env vars for the container build.
2. Build and start the bastion container.
3. `podman exec` into it and set the bastion env vars (remember to re-export
   `CASE=3-nodes-ocp-libvirt` inside the container).
4. Bootstrap bastion dependencies (`gitups apply bastion --yes`).

## Create And Edit Desired State

Generate the workspace, then copy the reference files for this case. You can
edit the generated files by hand instead; the copied files are the known-good
target state for the local-container layout.

```bash
WORKSPACE="$GITUPS_STATE_DIR/git-repos/clusters-bootstrap/$CASE/gitups"

gitups init workspace \
  --cluster-name "$CASE" \
  --provider emulated-bare-metal

cp /work/test/e2e/$CASE/{environment,provider,infra,cluster}.yaml "$WORKSPACE/"
vi "$WORKSPACE/environment.yaml" "$WORKSPACE/provider.yaml" "$WORKSPACE/infra.yaml"
```

Review these user-specific fields:

| File | Field |
| --- | --- |
| `environment.yaml` | `spec.baseDomain`, OpenShift release, optional proxy, secret sources under `spec.secrets` |
| `provider.yaml` | `spec.hosts.lab-host.ssh.address`, optional `ssh.user`, `libvirtURI`, BMC port, `compact-control-plane` profile sizing |
| `infra.yaml` | CIDR, bridge name, VIPs, per-node IPs, MAC addresses |

For the standard local-container path, leave `provider.yaml` with
`ssh.address: localhost` and no `ssh.user`; Gitups defaults the SSH user to the
current bastion user, which matches the host user created in the image.

### Optional Proxy For The OpenShift Install

For a proxied install, add a top-level `proxy:` block to `environment.yaml`
alongside `ocpInstallType: connected`:

```yaml
proxy:
  http: http://proxy.example.test:3128
  https: http://proxy.example.test:3128
  noProxy:
    - 192.168.133.0/24
```

Gitups auto-extends `noProxy` with cluster-local endpoints (service/cluster
CIDRs, `.svc`, `.cluster.local`, `localhost`, the base domain, mirror registry
host, provider host addresses) — only user-specific entries need to be listed.

If the proxy requires authentication, add an auth ref and a matching
`spec.secrets` entry. Pick one form.

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

**Generated** — `gitups secret generate` materializes the file. The password is
auto-generated; the username defaults to `admin` if `username:` is omitted:

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

### Optional Managed Squid Proxy

To have Gitups stand up an authenticated Squid proxy on the provider host
instead of using an external one, declare it on `provider.yaml`:

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
  `http://` URLs and include the same port as `spec.proxy.squid.port`.
- `environment.yaml` `spec.proxy.auth.proxyAuthRef` is mandatory — managed
  Squid is always authenticated.
- The libvirt machine `hostRef` must match `spec.proxy.squid.hostRef` (v1 keeps
  proxy and VMs on the same host so isolated networks remain reachable). The
  primary machine network must declare a `gateway` so VMs route egress through
  the managed proxy.

Hosts and VMs reach managed Squid at different addresses. Gitups renders two
client URLs:

- **Host URL** — `http://<proxy.squid.hostRef SSH address>:<port>`. Written by
  `host_proxy` into `/etc/dnf/dnf.conf`, `/etc/environment`, the systemd
  drop-in, and `pip.conf`. Routable on every host before libvirt is installed.
- **VM URL** — `http://<machineNetwork.gateway>:<port>`. Embedded in
  `install-config.yaml` so the OpenShift cluster sends runtime egress through
  the libvirt-bridge gateway, where Squid (bound via host networking) answers.

For external proxies (no `spec.proxy.squid`) both URLs collapse to the same
user-configured `spec.proxy.http` / `spec.proxy.https`.

## Save And Generate Secrets

| Secret | Form | Required for |
| --- | --- | --- |
| `gitups-ssh-key` / `.pub` | File under `~/.ssh` (from operator inputs) | Cluster SSH key, bastion→host SSH |
| `openshift-pull-secret` | Set from the pull-secret JSON | `render installer`, `apply clusters` |
| `proxy-credentials` (optional) | Set (file-backed) or generated | `apply bastion -f`, install-config proxy block |
| `bmc-credentials` | Generated from `environment.yaml` | `apply infra`, `apply clusters` |

```bash
test -r ~/.ssh/gitups-ssh-key
test -r ~/.ssh/gitups-ssh-key.pub

gitups secret set openshift-pull-secret \
  --pull-secret "$HOME/pull-secret.json" \
  --secrets-dir "$GITUPS_SECRETS_DIR"
```

If `environment.yaml` declares `proxy-credentials` in the **file-backed** form,
write it now (skip for the **generated** form):

```bash
gitups secret set proxy-credentials \
  --username <proxy-user> \
  --password-stdin \
  --secrets-dir "$GITUPS_SECRETS_DIR"
```

Materialize every `generated:` key (`bmc-credentials` always; `proxy-credentials`
if it is in generated form):

```bash
gitups secret generate -f "$WORKSPACE" --secrets-dir "$GITUPS_SECRETS_DIR"
find "$GITUPS_SECRETS_DIR" -maxdepth 1 -type f -printf '%f\n' | sort
```

## Apply The Workspace To The Bastion

Installs release-specific OpenShift CLIs declared by the workspace:

```bash
gitups apply bastion -f "$WORKSPACE" --yes
gitups check bastion -f "$WORKSPACE"
```

Bastion bootstrap runs *before* the managed Squid proxy exists, so Gitups
deliberately ignores `environment.yaml` `spec.proxy` here. Once
`gitups apply infra` provisions Squid, every subsequent phase (provider-host
package/image pulls, install-config rendering, agent install) routes through
it.

## Provision The Provider Host

Read-only preflight, then converge: base packages, libvirt, the cluster bridge,
managed HAProxy, managed host-file entries, and the Redfish BMC emulator.

```bash
gitups check infra -f "$WORKSPACE"
gitups apply infra -f "$WORKSPACE" --dry-run
gitups apply infra -f "$WORKSPACE" --yes
gitups check infra -f "$WORKSPACE"
```

If the host user has passwordless sudo, add `--ask-become-pass=false` to the
two `apply` commands.

## Install The 3-Node Cluster

`render installer` writes `install-config.yaml` and `agent-config.yaml` under
`git-repos/clusters-bootstrap/<cluster>/openshift/` with placeholder strings in place
of pull secret, SSH key, and trust bundle (this tree is the GitOps-publishable
declarative source). Add `--resolve-secrets` to also write
`runtime/<cluster>/installer/{install,agent}-config.yaml` with secret material
inlined (mode `0600`) — the form `openshift-install` consumes. The runtime
tree never leaves local state.

```bash
gitups check clusters -f "$WORKSPACE"
gitups render installer -f "$WORKSPACE" --scope "$CASE" --resolve-secrets

gitups apply clusters -f "$WORKSPACE" --dry-run
gitups apply clusters -f "$WORKSPACE" --yes
```

`apply clusters` regenerates the runtime installer copies under
`runtime/<cluster>/installer/`, renders the agent installer assets, boots all
three masters through the emulated Redfish BMC, and waits for
`openshift-install agent wait-for install-complete`.

## Verify

```bash
export KUBECONFIG="$GITUPS_STATE_DIR/clusters/$CASE/auth/kubeconfig"

oc get nodes
oc get clusterversion
oc get clusteroperators

gitups check infra -f "$WORKSPACE"
gitups check clusters -f "$WORKSPACE"
```

`oc get nodes` should list three `Ready` control-plane nodes.

## Tear Down

Inside the bastion:

```bash
gitups destroy clusters -f "$WORKSPACE" --yes
gitups destroy infra -f "$WORKSPACE" --yes
exit
```

Then remove the container and per-case host state — see
[shared teardown](../containerized-bastion.md#teardown--container-and-host-state).
