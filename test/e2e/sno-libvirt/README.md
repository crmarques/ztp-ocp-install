# SNO On Libvirt

Provisions one OpenShift single-node cluster on a libvirt host. The bastion
that runs `bootwright` can be either a VM/host or a Podman container — operator's
choice.

## Shape

| Piece | Value |
| --- | --- |
| Cluster | SNO on libvirt bridge `vbr-cb-sno` |
| Network | `192.168.132.0/24`, API/API-int `.10`, Ingress `.11`, node `.20` |
| Provider | libvirt, emulated bare metal (Redfish BMC) |
| Load balancer | Managed HAProxy on the provider host (default; see [load-balancer.md](../load-balancer.md)) |
| Proxy | Pick one mode from [proxy.md](../proxy.md); reference YAMLs ship the managed-Squid layout |

## Provider Host Requirements

The provider host launches the cluster VMs through QEMU/KVM, so it must
expose hardware-accelerated virtualization to user space (`/dev/kvm`).
On bare metal that means CPU virtualization extensions enabled in BIOS.
If the provider host is itself a VM (VMware / ESXi, OpenStack, libvirt,
etc.), enable **nested virtualization** on that VM so it can act as a
KVM host. Bootwright checks the capability during `bootwright apply infra`;
package installation is its responsibility, not the operator's.

## 1. Bring Up A Bastion

Set `CASE` and follow the bastion doc for the mode you want:

```bash
export CASE=sno-libvirt
```

- VM/host bastion → [bastion.md](../bastion.md)
- Containerized bastion → [containerized-bastion.md](../containerized-bastion.md)

Stop after that doc's "Bootstrap Bastion Dependencies" section. You should
now have a working `bootwright` and the env vars (`$BOOTWRIGHT_STATE_DIR`,
`$BOOTWRIGHT_SECRETS_DIR`, `$BOOTWRIGHT_REPO`, `$CASE`) exported.

## 2. Customize Desired State

Generate the workspace and copy the reference YAMLs for this case. You can
hand-edit a fresh workspace instead; the copied files are the known-good
target state.

```bash
WORKSPACE="$BOOTWRIGHT_STATE_DIR/git-repos/clusters-bootstrap/$CASE/bootwright"

bootwright init workspace \
  --cluster-name "$CASE" \
  --provider emulated-bare-metal

cp "$BOOTWRIGHT_REPO/test/e2e/$CASE"/{environment,provider,infra,cluster}.yaml "$WORKSPACE/"
vi "$WORKSPACE/environment.yaml" "$WORKSPACE/provider.yaml" "$WORKSPACE/infra.yaml"
```

Review these user-specific fields:

| File | Field |
| --- | --- |
| `environment.yaml` | `spec.baseDomain`, OpenShift release, optional proxy |
| `provider.yaml` | `spec.hosts.lab-host.ssh.address` (set to the provider host's reachable address when the bastion is remote), optional `ssh.user`, `libvirtURI`, BMC port |
| `infra.yaml` | CIDR, bridge name, VIPs, node IP, MAC address |

For the same-machine layout (default), leave `ssh.address: localhost` and
no `ssh.user`; Bootwright defaults the SSH user to the current bastion user.

## 3. Pick A Proxy Mode (Optional)

The reference YAMLs ship the managed-Squid layout. To use a different mode,
edit `environment.yaml` and `provider.yaml` per [proxy.md](../proxy.md). For
a direct connected install, prune the `proxy:` blocks per
[proxy.md Mode 1](../proxy.md#mode-1--direct-no-proxy).

## 4. Pick A Load Balancer (Optional)

The reference YAMLs ship the managed-HAProxy layout. To switch modes, edit
`provider.yaml` and `infra.yaml` per [load-balancer.md](../load-balancer.md).
Note: installer-managed (keepalived + haproxy) is **not** supported for
single-node OpenShift — stay on managed HAProxy or use an external LB.

## 5. Run The Common Steps

Save secrets, apply the workspace, provision the provider host, install the
cluster, verify, and tear it down — all in
[common-steps.md](../common-steps.md). When you reach the "Verify" step,
`oc get nodes` should show **one** `Ready` control-plane node.

## 6. Tear Down The Bastion

After [common-steps.md § Tear Down The Cluster](../common-steps.md#6-tear-down-the-cluster),
clean the bastion side:

- VM/host bastion → [bastion.md § Tear Down](../bastion.md#tear-down--bastion-state)
- Containerized bastion → [containerized-bastion.md § Tear Down](../containerized-bastion.md#tear-down--container-and-host-state)
