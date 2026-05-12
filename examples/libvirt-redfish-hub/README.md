# Example: Libvirt/Redfish Hub

**Install type:** disconnected (`Environment.spec.ocpInstall.disconnected`).

A hub SNO running with disconnected OpenShift release content from a local
registry mirror. The example demonstrates
`Environment.spec.ocpInstall.disconnected`, including mirror credentials and
generated self-signed registry trust referenced through `SecretRef`.

## Files

| File | Kind |
| --- | --- |
| [environment.yaml](environment.yaml) | `Environment` with disconnected OCP install |
| [provider.yaml](provider.yaml) | `InfrastructureProvider` for one libvirt host with Redfish emulation |
| [cluster-infrastructure-hub.yaml](cluster-infrastructure-hub.yaml) | `ClusterInfrastructure` for the hub |
| [ocp-cluster-hub.yaml](ocp-cluster-hub.yaml) | `OCPCluster` hub intent |

## Commands

```text
gitups check bastion -f examples/libvirt-redfish-hub
gitups check infra -f examples/libvirt-redfish-hub --state-dir .state --dry-run
gitups apply infra -f examples/libvirt-redfish-hub --state-dir .state --dry-run
gitups apply clusters -f examples/libvirt-redfish-hub --state-dir .state --dry-run
gitups apply hub -f examples/libvirt-redfish-hub --state-dir .state --dry-run
gitups secrets generate -f examples/libvirt-redfish-hub
```
