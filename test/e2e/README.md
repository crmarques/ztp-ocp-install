# Bootwright E2E Cases

End-to-end scenarios for `bootwright`. Pick a case, follow its README, and let it
point you at the shared docs in this directory for the steps that do not
change between cases.

## How To Use These Docs

Each run follows the same five stages. The case README is the spine; it tells
you, in order, which shared doc to open at each stage.

1. **Bastion** — where `bootwright` runs. Pick one mode and follow that doc:
   - [bastion.md](bastion.md) — a Linux VM or physical host you SSH into.
   - [containerized-bastion.md](containerized-bastion.md) — a non-root UBI9
     Podman container on the operator's machine (`--network host`).
2. **Desired state** — case-specific. The case README lists the YAML files
   to copy from the case directory and the user-specific fields to edit.
3. **Proxy** — optional. [proxy.md](proxy.md) covers direct, external
   proxy (with or without auth), and Bootwright-managed Squid.
4. **Load balancer** — [load-balancer.md](load-balancer.md). The reference
   cases use managed HAProxy on the provider host.
5. **Apply, install, verify, tear down** — [common-steps.md](common-steps.md).
   Identical commands across cases; the case README only flags the few
   case-specific outputs to expect.

## Cases

| Case | Topology | Provider | Network |
| --- | --- | --- | --- |
| [sno-libvirt](sno-libvirt/README.md) | Single-node OpenShift | libvirt, emulated bare metal | `192.168.132.0/24` |
| [3-nodes-ocp-libvirt](3-nodes-ocp-libvirt/README.md) | 3-node compact (3 control-plane, no workers) | libvirt, emulated bare metal | `192.168.133.0/24` |

Both cases work with either bastion mode. The reference YAMLs assume bastion
and provider are the same Linux host (`ssh.address: localhost`); change
`provider.yaml` `spec.hosts.lab-host.ssh.address` for a separate provider.

## Shared Docs

| File | Covers |
| --- | --- |
| [bastion.md](bastion.md) | VM/host bastion setup, SSH key push to provider hosts, optional bastion proxy env |
| [containerized-bastion.md](containerized-bastion.md) | Build + run the UBI9 bastion container, host-to-bastion volume layout |
| [proxy.md](proxy.md) | Direct / external proxy / managed Squid; auth secret forms; two-URL render |
| [load-balancer.md](load-balancer.md) | Managed HAProxy used by the cases, pointer to the other LB modes |
| [common-steps.md](common-steps.md) | Save secrets → apply bastion/infra/clusters → verify → destroy |
