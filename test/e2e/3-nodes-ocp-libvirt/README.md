# 3-Node OCP On Libvirt

Provisions one 3-node compact OpenShift cluster (3 control-plane nodes, no
dedicated workers) on a libvirt host. The bastion that runs `gitups` can be
either a VM/host or a Podman container — operator's choice.

## Shape

| Piece | Value |
| --- | --- |
| Cluster | 3 control-plane nodes on libvirt bridge `vbr-cb-3n` |
| Network | `192.168.133.0/24`, API/API-int `.10`, Ingress `.11`, nodes `.20`–`.22` |
| Provider | libvirt, emulated bare metal (Redfish BMC) |
| Load balancer | Managed HAProxy on the provider host (default; see [load-balancer.md](../load-balancer.md)) |
| Proxy | Pick one mode from [proxy.md](../proxy.md); reference YAMLs ship the managed-Squid layout |

## Provider Host Requirements

The provider host launches the cluster VMs through QEMU/KVM, so it must
expose hardware-accelerated virtualization to user space (`/dev/kvm`).
On bare metal that means CPU virtualization extensions enabled in BIOS.
If the provider host is itself a VM (VMware / ESXi, OpenStack, libvirt,
etc.), enable **nested virtualization** on that VM so it can act as a
KVM host. Gitups checks the capability during `gitups apply infra`;
package installation is its responsibility, not the operator's.

Three VMs are launched on the same provider host, each sized by the
`compact-control-plane` profile (`provider.yaml`). Plan for roughly
**12 vCPU + 48 GiB RAM + 360 GiB disk** total (default: 4 vCPU,
16 GiB RAM, 120 GiB disk per node).

## 1. Bring Up A Bastion

Set `CASE` and follow the bastion doc for the mode you want:

```bash
export CASE=3-nodes-ocp-libvirt
```

- VM/host bastion → [bastion.md](../bastion.md)
- Containerized bastion → [containerized-bastion.md](../containerized-bastion.md)

Stop after that doc's "Bootstrap Bastion Dependencies" section. You should
now have a working `gitups` and the env vars (`$GITUPS_STATE_DIR`,
`$GITUPS_SECRETS_DIR`, `$GITUPS_REPO`, `$CASE`) exported.

## 2. Customize Desired State

Generate the workspace and copy the reference YAMLs for this case. You can
hand-edit a fresh workspace instead; the copied files are the known-good
target state.

```bash
WORKSPACE="$GITUPS_STATE_DIR/git-repos/clusters-bootstrap/$CASE/gitups"

gitups init workspace \
  --cluster-name "$CASE" \
  --provider emulated-bare-metal

cp "$GITUPS_REPO/test/e2e/$CASE"/{environment,provider,infra,cluster}.yaml "$WORKSPACE/"
vi "$WORKSPACE/environment.yaml" "$WORKSPACE/provider.yaml" "$WORKSPACE/infra.yaml"
```

Review these user-specific fields:

| File | Field |
| --- | --- |
| `environment.yaml` | `spec.baseDomain`, OpenShift release, optional proxy |
| `provider.yaml` | `spec.hosts.lab-host.ssh.address` (set to the provider host's reachable address when the bastion is remote), optional `ssh.user`, `libvirtURI`, BMC port, `compact-control-plane` profile sizing |
| `infra.yaml` | CIDR, bridge name, VIPs, per-node IPs, MAC addresses |

For the same-machine layout (default), leave `ssh.address: localhost` and
no `ssh.user`; Gitups defaults the SSH user to the current bastion user.

## 3. Pick A Proxy Mode (Optional)

The reference YAMLs ship the managed-Squid layout. To use a different mode,
edit `environment.yaml` and `provider.yaml` per [proxy.md](../proxy.md). For
a direct connected install, prune the `proxy:` blocks per
[proxy.md Mode 1](../proxy.md#mode-1--direct-no-proxy).

## 4. Pick A Load Balancer (Optional)

The reference YAMLs ship the managed-HAProxy layout. Multi-node lets you
also switch to installer-managed `keepalived` + `haproxy` on the control
planes or to an external LB — see [load-balancer.md](../load-balancer.md).

## 5. Run The Common Steps

Save secrets, apply the workspace, provision the provider host, install the
cluster, verify, and tear it down — all in
[common-steps.md](../common-steps.md). When you reach the "Verify" step,
`oc get nodes` should show **three** `Ready` control-plane nodes.

## 6. Tear Down The Bastion

After [common-steps.md § Tear Down The Cluster](../common-steps.md#6-tear-down-the-cluster),
clean the bastion side:

- VM/host bastion → [bastion.md § Tear Down](../bastion.md#tear-down--bastion-state)
- Containerized bastion → [containerized-bastion.md § Tear Down](../containerized-bastion.md#tear-down--container-and-host-state)
