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
      version: 4.21.10
---
apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: baremetal-redfish-provider
spec:
  bareMetal:
    bmcProtocol: redfish
---
apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: hub
spec:
  providerRef:
    name: baremetal-redfish-provider
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
      bareMetal:
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
  qemuKVM:
    hosts:
      qemu-host:
        address: 192.168.10.11
        sshKeyRef:
          name: qemu-host-ssh
        capabilities:
          - libvirt
          - hosts-file
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
  providerRef:
    name: qemu-redfish-provider
  networks:
    primary:
      cidr: 192.168.130.0/24
      gateway: 192.168.130.1
      dnsServers:
        - 192.168.130.1
      qemuKVM:
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
      qemuKVM:
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

- [`examples/qemu-redfish-fleet`](../examples/qemu-redfish-fleet/) (connected)
- [`examples/baremetal-redfish-fleet`](../examples/baremetal-redfish-fleet/) (connected)
- [`examples/qemu-redfish-hub`](../examples/qemu-redfish-hub/) (disconnected)

## Secret Material

`SecretRef.name` maps to files under `<gitups-home>/secrets` by default, where
Gitups home is `GITUPS_HOME` or `~/.gitups`.

```text
gitups secrets pull-secret set --name openshift-pull-secret --from-file ~/pull-secret.json
gitups secrets bmc set --name baremetal-redfish-bmc --username admin --password-stdin
gitups secrets generate -f examples/qemu-redfish-hub
```

Generated self-signed registry trust is declared by reference, for example
`trustBundle.generatedSelfSigned.secretRef.name`; Gitups writes the generated
certificate material to the local secrets directory, never to the repo.
