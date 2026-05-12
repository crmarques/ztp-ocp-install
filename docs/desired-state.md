---
title: Desired State
---

# Desired State

Gitups input is four user-authored YAML kinds. Every fact has one owner, and
everything else is rendered from those files. The definitive schema and
validation rules live in [`/specs/state-model.md`](../specs/state-model.md).

## Where Facts Belong

| Fact | Kind |
| --- | --- |
| base domain, OpenShift install type, optional proxy and registries, shared secret sources, release defaults | `Environment` |
| provider hosts, provider credentials, Redfish/BMC emulation, machine profiles | `InfrastructureProvider` |
| cluster networks, machines, VIPs, load balancer placement, name resolution | `ClusterInfrastructure` |
| hub/managed role, topology, install method, cluster networking, node identity | `OCPCluster` |

If changing the substrate forces an `OCPCluster` edit, the model is wrong.

## Minimal Connected Hub

```yaml
apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: connected-hub
spec:
  baseDomain: example.test
  ocpInstallType: connected
  secrets:
    openshift-pull-secret:
      file: ~/.gitups/secrets/openshift-pull-secret
    cluster-admin-key:
      file: ~/.ssh/gitups-ssh-key.pub
  openshift:
    release:
      channel: stable-4.21
      version: 4.21.12
---
apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: baremetal-redfish-provider
spec:
  machine:
    baremetal:
      bmcProtocol: redfish
---
apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: hub
spec:
  providerRefs:
    - name: baremetal-redfish-provider
  networks:
    primary:
      cidr: 192.168.130.0/24
      gateway: 192.168.130.1
      dnsServers:
        - 192.168.130.1
  machines:
    master-0:
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.130.20
          macAddress: 52:54:00:21:11:10
      rootDeviceHints:
        deviceName: /dev/sda
      baremetal:
        bmc:
          address: redfish-virtualmedia+https://bmc-hub-0.example.test/redfish/v1/Systems/1
          credentialRef:
            name: baremetal-redfish-bmc
  endpoints:
    api:
      address: 192.168.130.10
    apiInt:
      address: 192.168.130.10
    ingress:
      address: 192.168.130.11
---
apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: hub
spec:
  role: hub
  topology: single-node
  infrastructureRef:
    name: hub
  install:
    method: agent
  networking:
    clusterNetwork:
      - cidr: 10.128.0.0/14
        hostPrefix: 23
    serviceNetwork:
      - 172.30.0.0/16
  nodes:
    master-0:
      role: control-plane
```

External DNS is assumed unless `ClusterInfrastructure` declares managed name
resolution. Load balancing has three dispositions, selected purely by what
`ClusterInfrastructure` does or does not declare:

- **External LB** — `endpoints` defined, `loadBalancers` omitted, and the
  referenced provider has no `loadBalancer` capability. Gitups passes the VIPs
  to the installer and provisions nothing; an external LB answers the VIPs.
- **Installer-managed (keepalived + haproxy on the control planes)** — same
  desired state as External LB. On multi-node `platform: baremetal` (libvirt
  renders as baremetal) the agent installer itself deploys keepalived and
  haproxy on the control-plane nodes, so no external LB is required. See
  [`examples/baremetal-redfish-fleet`](../examples/baremetal-redfish-fleet/).
- **Gitups-managed HAProxy** — `loadBalancers` is declared on
  `ClusterInfrastructure` and the referenced provider declares a
  `loadBalancer.haProxy` capability. Gitups pins the HAProxy component and
  Ansible places it on the chosen provider host. See
  [`examples/baremetal-edge-lb-fleet`](../examples/baremetal-edge-lb-fleet/).

Proxy has the same split between client contract and provider placement.
`Environment.spec.proxy` (`http`, `https`, `noProxy`, `auth.proxyAuthRef`)
declares the outbound proxy that every component should use — bastion CLI,
provider-host package and image pulls, generated `install-config.yaml`, and
`openshift-install`. Gitups auto-extends `noProxy` with cluster-local
endpoints (service/cluster CIDRs, `.svc`, `.cluster.local`, base domain,
mirror registry host, provider host addresses); user-supplied entries take
precedence. If a referenced provider also declares `spec.proxy.squid`, Gitups
provisions authenticated Squid on that host and materializes its htpasswd
from the same `auth.proxyAuthRef`. If no provider declares `proxy.squid`, the
proxy URLs are treated as external.

`Environment.spec.registries.mirror` declares the OpenShift mirror endpoint
and trust material. It is required when `ocpInstallType: disconnected` and
optional alongside `connected` when only release content is mirrored.

For Gitups-managed libvirt networks, managed Squid also isolates VM egress:
the rendered libvirt network omits NAT and VMs reach the internet through the
proxy only. This is not applied for external proxies, no proxy, bare metal,
vSphere, OpenShift Virtualization, or provider-host networking.

## Provider Swap

The same `Environment` and `OCPCluster` work for the QEMU/Redfish lab. Only
the provider and cluster infrastructure change:

```yaml
apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: qemu-redfish-provider
spec:
  hosts:
    qemu-host:
      ssh:
        address: 192.168.10.11
        user: gitups
        keyRef:
          name: qemu-host-ssh
      capabilities:
        - libvirt
        - hosts-file
  machine:
    libvirt:
      hostRefs:
        - name: qemu-host
      bmcEmulation:
        auth:
          credentialRef:
            name: qemu-redfish-bmc
---
apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: hub
spec:
  providerRefs:
    - name: qemu-redfish-provider
  networks:
    primary:
      cidr: 192.168.130.0/24
      gateway: 192.168.130.1
      dnsServers:
        - 192.168.130.1
      libvirt:
        bridge: vbr-hub
  machines:
    master-0:
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.130.20
          macAddress: 52:54:00:21:11:10
      rootDeviceHints:
        deviceName: /dev/vda
      libvirt:
        hostRef:
          name: qemu-host
  endpoints:
    api:
      address: 192.168.130.10
    apiInt:
      address: 192.168.130.10
    ingress:
      address: 192.168.130.11
```

Canonical examples:

- [`examples/libvirt-redfish-fleet`](../examples/libvirt-redfish-fleet/) (connected)
- [`examples/baremetal-redfish-fleet`](../examples/baremetal-redfish-fleet/) (connected)
- [`examples/libvirt-redfish-hub`](../examples/libvirt-redfish-hub/) (disconnected)

## Secret Material

`SecretRef.name` maps to files under `<gitups-user-dir>/secrets` by default,
where the Gitups user directory is `GITUPS_USER_DIR` or `~/.gitups`. Override
the secrets directory directly with `GITUPS_SECRETS_DIR` or `--secrets-dir`.

```text
gitups secret set openshift-pull-secret --pull-secret ~/pull-secret.json
gitups secret set baremetal-redfish-bmc --username admin --password-stdin
gitups secret generate -f examples/libvirt-redfish-hub
```

Both file-sourced and gitups-generated secrets are declared in
`Environment.spec.secrets[name]`. A `file:` source points at operator-supplied
material on disk; a `generated:` source asks gitups to materialize the
secret itself — either a `username:password\n` file (`generated.credentials`)
or a self-signed cert/key pair (`generated.selfSignedCertificate`). The
mirror trust bundle is wired via `registries.mirror.trustBundleRef.name`;
the matching `secrets[name].generated.selfSignedCertificate` decides how that
reference is sourced. `gitups secret generate -f` materializes only
`generated:` entries; `file:` entries must already exist at their declared
paths or be written with `gitups secret set <name> --pull-secret <path>` for
pull-secrets or `gitups secret set <name> [--from-file|--username|--generate]`
for credentials.
