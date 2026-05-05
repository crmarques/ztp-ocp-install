# QEMU 3 Hosts, 1 Hub + 2 OCP Fleet

**Install type:** connected through an explicit forward proxy
(`Environment.spec.ocpInstall.restricted.proxy`).

Three QEMU/KVM provider hosts each run a single multi-node cluster: a 3-node
hub plus two multi-node managed OCP clusters (compact control plane).

## Topology

| Cluster | Provider | Provider host | Role | Topology |
| --- | --- | --- | --- | --- |
| `qemu-3-hosts-hub` | `qemu-3-hosts-hub-provider` | `hub-host` | hub | multi-node (3 nodes) |
| `qemu-3-hosts-ocp-01` | `qemu-3-hosts-ocp-01-provider` | `ocp-01-host` | managed | multi-node (3 nodes) |
| `qemu-3-hosts-ocp-02` | `qemu-3-hosts-ocp-02-provider` | `ocp-02-host` | managed | multi-node (3 nodes) |

Each cluster has its own `InfrastructureProvider` so the rendered Ansible
inventory binds one provider host to one cluster — without that split, every
provider host ends up in every cluster's group and every host tries to bring
up every cluster.

Provider host addresses are defined in
`provider.yaml: spec.hosts.<host>.ssh.address`. Cluster networks are configured
per cluster in `cluster-infrastructure-<cluster>.yaml: spec.networks.primary.cidr`.
All three clusters use the `compact-control-plane` provider profile.

## Connected + Proxy Inputs

`environment.yaml` uses `ocpInstall.restricted.proxy` to declare a single
forward proxy. Gitups projects this in two places:

1. **`install-config.yaml.proxy`** on every cluster — bootstrap, MCO, and
   image pulls inside the cluster honour the proxy and skip `noProxy` ranges.
2. **Provider host configuration** — the new `host_proxy` Ansible role runs
   first on every host and writes:
   - `/etc/environment` and `/etc/profile.d/gitups-proxy.sh`
   - `/etc/dnf/dnf.conf` (and `/etc/yum.conf` if present) `proxy=` lines
   - `/root/.pip/pip.conf` `[global] proxy = ...`
   - `/etc/containers/containers.conf` engine env block
   - `/etc/systemd/system.conf.d/10-gitups-proxy.conf` (`DefaultEnvironment`)

   Subsequent tasks (`dnf install`, `pip install sushy-tools`,
   `podman pull haproxy`, `openshift-install agent create image`,
   `openshift-install agent wait-for install-complete`) inherit the proxy via
   the play-level `environment:` block, so external pulls go through the
   proxy while everything in `noProxy` (provider net, cluster machine
   networks, baseDomain, BMC loopback, cluster/service CIDRs) is reached
   directly.

The defaults shipped here use a placeholder URL
(`environment.yaml: spec.ocpInstall.restricted.proxy.{httpProxy,httpsProxy}`) —
replace with the URL of your environment's proxy before applying.

When the upstream proxy needs authentication, set
`ocpInstall.restricted.proxy.credentialsRef.name` to a secret containing one
`username:password` line (same format as BMC and mirror credentials). Both
`host_proxy` and `hub_install_agent` URL-encode the credentials and splice
them into every URL they emit; the rendered `vars.yaml` and
`install-config.yaml` keep the bare URL so credentials never reach committed
or rendered artifacts. Drop the `credentialsRef` block to use an open proxy.

## Secrets

Secret files live outside the repo under `~/.gitups/secrets` by default.

| Secret name | Purpose |
| --- | --- |
| `provider-host-admin-key` | SSH key for each provider host |
| `openshift-pull-secret` | OCP pull secret JSON |
| `cluster-admin-key` | Public SSH key installed into OCP nodes |
| `qemu-3-hosts-hub-fleet-bmc-credentials` | Redfish emulator credentials (shared across the three providers) |
| `proxy-credentials` | Upstream proxy `username:password` (omit when the proxy is unauthenticated) |

```text
install -d -m 0700 ~/.ssh ~/.gitups/secrets
ssh-keygen -t ed25519 -f ~/.ssh/gitups-qemu-3-hosts -N '' -C gitups-qemu-3-hosts
install -m 0600 ~/.ssh/gitups-qemu-3-hosts     ~/.gitups/secrets/provider-host-admin-key
install -m 0600 ~/.ssh/gitups-qemu-3-hosts.pub ~/.gitups/secrets/cluster-admin-key
gitups secrets pull-secret set --name openshift-pull-secret --from-file ~/pull-secret.json
# Authenticated proxy only — drop credentialsRef from environment.yaml when
# the proxy is open. `gitups secrets credentials set` is still the writer for any
# operator-provided `username:password` secret (e.g. when the proxy
# credentials must come from a vault rather than be generated).
gitups secrets credentials set --name proxy-credentials
gitups secrets generate -f test/e2e/qemu-3-hosts-1-hub-2-ocp-fleet
```

Install the matching public key in `/root/.ssh/authorized_keys` on each of
the three provider hosts (the addresses listed under `provider.yaml: spec.hosts.*.ssh.address`).

## Bastion Prerequisites

`gitups apply` runs on a bastion that SSHes to the three provider hosts. The
bastion needs:

- `ansible-playbook` on `PATH` and the embedded bundle (handled by `make build`).
- Reachability to each `provider.yaml: spec.hosts.*.ssh.address:22` and to the
  proxy itself.
- Proxy env exported in the shell that runs `gitups apply` (the runner
  forwards `os.Environ()` to ansible-playbook). Use the values from
  `environment.yaml: spec.ocpInstall.restricted.proxy`:

  ```sh
  export HTTP_PROXY=<httpProxy from environment.yaml>
  export HTTPS_PROXY=<httpsProxy from environment.yaml>
  export NO_PROXY=<comma-joined noProxy entries from environment.yaml>
  ```

The provider hosts only need a working base OS and `python3`; `host_proxy`
configures dnf/pip/podman/systemd before `host_prepare` runs the first
`dnf install`.

## Commands

```text
make e2e-dry-run CASE=qemu-3-hosts-1-hub-2-ocp-fleet
make e2e CASE=qemu-3-hosts-1-hub-2-ocp-fleet
```

Equivalent CLI flow:

```text
gitups validate -f test/e2e/qemu-3-hosts-1-hub-2-ocp-fleet --check-host
gitups plan     -f test/e2e/qemu-3-hosts-1-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-hub-2-ocp-fleet
gitups apply infra -f test/e2e/qemu-3-hosts-1-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-hub-2-ocp-fleet --dry-run
gitups apply hub   -f test/e2e/qemu-3-hosts-1-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-hub-2-ocp-fleet --dry-run
gitups apply infra -f test/e2e/qemu-3-hosts-1-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-hub-2-ocp-fleet --yes
gitups apply hub   -f test/e2e/qemu-3-hosts-1-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-hub-2-ocp-fleet --yes
gitups status   -f test/e2e/qemu-3-hosts-1-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-hub-2-ocp-fleet --diff
gitups destroy all -f test/e2e/qemu-3-hosts-1-hub-2-ocp-fleet --state-dir /tmp/gitups-qemu-3-hosts-1-hub-2-ocp-fleet --yes
```
