# Example: Bare-Metal Redfish Fleet

**Install type:** connected (`Environment.spec.ocpInstallType: connected`).

A hub SNO plus one managed cluster on real bare-metal hosts driven by Redfish
virtual media BMCs.

## Files

| File | Kind |
| --- | --- |
| [environment.yaml](environment.yaml) | `Environment` shared by the connected examples |
| [provider.yaml](provider.yaml) | `InfrastructureProvider` for bare-metal Redfish |
| [cluster-infrastructure-hub.yaml](cluster-infrastructure-hub.yaml) | `ClusterInfrastructure` for the hub |
| [cluster-infrastructure-managed-01.yaml](cluster-infrastructure-managed-01.yaml) | `ClusterInfrastructure` for `managed-01` |
| [ocp-cluster-hub.yaml](ocp-cluster-hub.yaml) | `OCPCluster` hub intent |
| [ocp-cluster-managed-01.yaml](ocp-cluster-managed-01.yaml) | `OCPCluster` managed-cluster intent |

`environment.yaml` and both `ocp-cluster-*.yaml` files are byte-identical to
the matching files under
[`examples/libvirt-redfish-fleet`](../libvirt-redfish-fleet/).
Only provider and cluster-infrastructure files change across the provider swap.

External DNS and load balancing are assumed unless
`ClusterInfrastructure` declares managed name resolution or managed load
balancers.

## Commands

```text
bootwright check bastion -f examples/baremetal-redfish-fleet
bootwright check infra -f examples/baremetal-redfish-fleet --state-dir .state --dry-run
```
