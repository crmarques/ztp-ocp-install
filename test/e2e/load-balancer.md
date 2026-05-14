# Load Balancer Options

The e2e cases front the API, API-internal, and Ingress VIPs with a load
balancer the operator picks via desired state. The reference YAMLs use
**Gitups-managed HAProxy** on the provider host — switch modes by editing
`provider.yaml` and (for some modes) `infra.yaml`.

| Mode | What Gitups does | When to use |
| --- | --- | --- |
| [Managed HAProxy](#managed-haproxy) | Pins a HAProxy component on the provider host and writes its config | Lab / e2e default; provider has the `loadBalancer.haProxy` capability |
| [Installer-managed](#installer-managed-or-external) | Hands the VIPs to the agent installer, which runs `keepalived` + `haproxy` on the control planes (multi-node baremetal/libvirt only) | Production-style platform-managed VIPs without an external LB |
| [External](#installer-managed-or-external) | Hands the VIPs to the installer; expects an LB you operate already answering them | Pre-existing F5 / LB appliance / cloud LB |

Installer-managed and External share the same desired state. The installer
picks installer-managed automatically on multi-node `platform: baremetal`
(libvirt renders as baremetal); otherwise it expects the VIPs to be served
externally. See [`docs/desired-state.md`](../../docs/desired-state.md) for
the authoritative schema and constraints.

## Managed HAProxy

This is what both reference cases ship in `provider.yaml`:

```yaml
spec:
  hosts:
    lab-host:
      capabilities:
        - libvirt
        - container-runtime
        - hosts-file
        - proxy
        # implicit: loadBalancer.haProxy capability when provider declares it
  loadBalancer:
    haProxy:
      hostRef:
        name: lab-host
```

`infra.yaml` lists the LB endpoints (already in the reference YAMLs):

```yaml
spec:
  endpoints:
    api:
      address: <api VIP>
    apiInt:
      address: <api-int VIP>
    ingress:
      address: <ingress VIP>
  loadBalancers:
    default:
      endpoints:
        - api
        - apiInt
        - ingress
```

`gitups apply infra` provisions and binds the HAProxy container on the
referenced host. Nothing else for the operator to set up.

## Installer-Managed Or External

Drop the `loadBalancer:` block from `provider.yaml` and the
`loadBalancers:` block from `infra.yaml`, but keep `endpoints:`. Gitups
forwards the VIPs to the installer and provisions nothing.

What happens next depends on the cluster shape:

- **Multi-node baremetal/libvirt** — the agent installer brings up
  `keepalived` + `haproxy` on the control planes themselves. No external
  LB needed. Works for the 3-node case; not supported for SNO.
- **Any other topology, or when you already operate an LB** — you are
  responsible for having an LB routing the three VIPs to the cluster nodes
  by the time `apply clusters` runs.
