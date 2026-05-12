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
| base domain, OpenShift install mode, shared secret refs, release defaults | `Environment` |
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
  ocpInstall:
    connected: {}
  secrets:
    pullSecretRef:
      name: openshift-pull-secret
    clusterSSHKeyRef:
      name: cluster-admin-key
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

External DNS and load balancing are assumed unless `ClusterInfrastructure`
declares managed name resolution or managed load balancers.

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
`Environment.spec.keys[name]`. A `file:` source points at operator-supplied
material on disk; a `generated:` source asks gitups to materialize the
secret itself — either a `username:password\n` file (`generated.credentials`)
or a self-signed cert/key pair (`generated.selfSignedCertificate`). The
mirror trust bundle is wired via `registries.mirror.trustBundleRef.name`;
the matching `keys[name].generated.selfSignedCertificate` decides how that
reference is sourced. `gitups secret generate -f` materializes only
`generated:` entries; `file:` entries must already exist at their declared
paths or be written with `gitups secret set <name> --pull-secret <path>` for
pull-secrets or `gitups secret set <name> [--from-file|--username|--generate]`
for credentials.
