# Example: Libvirt/Redfish Fleet

**Install type:** connected (`Environment.spec.ocpInstall.connected`).

A hub SNO plus one managed cluster running as libvirt-managed virtual machines.
Redfish BMC emulation keeps the lab path close to real bare-metal
provisioning.

## Files

| File | Kind |
| --- | --- |
| [environment.yaml](environment.yaml) | `Environment` shared by the connected examples |
| [provider.yaml](provider.yaml) | `InfrastructureProvider` for one libvirt host with Redfish emulation |
| [cluster-infrastructure-hub.yaml](cluster-infrastructure-hub.yaml) | `ClusterInfrastructure` for the hub |
| [cluster-infrastructure-managed-01.yaml](cluster-infrastructure-managed-01.yaml) | `ClusterInfrastructure` for `managed-01` |
| [ocp-cluster-hub.yaml](ocp-cluster-hub.yaml) | `OCPCluster` hub intent |
| [ocp-cluster-managed-01.yaml](ocp-cluster-managed-01.yaml) | `OCPCluster` managed-cluster intent |

`environment.yaml` and both `ocp-cluster-*.yaml` files are byte-identical to
the matching files under
[`examples/baremetal-redfish-fleet`](../baremetal-redfish-fleet/).

## Commands

```text
gitups validate -f examples/libvirt-redfish-fleet
gitups plan -f examples/libvirt-redfish-fleet --state-dir .state
gitups apply infra -f examples/libvirt-redfish-fleet --state-dir .state --dry-run
gitups apply ocp -f examples/libvirt-redfish-fleet --state-dir .state --dry-run
```
