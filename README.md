# Gitups

Gitups is a desired-state orchestrator for OpenShift cluster fleets. You
author versioned YAML, Gitups validates it, renders deterministic inputs for
OpenShift, Ansible, and GitOps, then converges the environment in ordered
phases.

Today the CLI installs OpenShift clusters end-to-end via the agent installer.
Forward-looking architecture leaves room for a future hub cluster running
ACM and OpenShift GitOps to reconcile additional managed clusters from
declared fleet state, but that GitOps publication path is not implemented yet.

## Start Here

| Audience | Start |
| --- | --- |
| Users running a lab | [Quickstart](docs/quickstart.md) |
| Users authoring desired state | [Desired State](docs/desired-state.md) |
| Contributors and coding agents | [Specs](specs/index.md) |
| Architecture decisions | [ADRs](specs/adr/README.md) |

Human docs explain the workflow. Specs are the source of truth for API shape,
architecture boundaries, validation rules, CLI behavior, and security posture.

## Desired-State Contract

User-authored YAML uses `apiVersion: gitups.io/v1alpha1` and four kinds:

| Kind | Owns |
| --- | --- |
| `Environment` | Shared environment defaults: base domain, OpenShift install mode, secret refs, OpenShift release, component image pins |
| `InfrastructureProvider` | Provider connections and capabilities: libvirt, bare metal, and future vSphere/OpenShift Virtualization scaffolds |
| `ClusterInfrastructure` | One cluster's realised infrastructure: networks, machines, endpoints, load balancers, name resolution |
| `OCPCluster` | Provider-neutral OpenShift intent: topology, install method, networking, node identity |

`OCPCluster` stays provider-neutral. Swapping from libvirt with Redfish
emulation to real bare metal edits only `InfrastructureProvider` and
`ClusterInfrastructure`.

Current `apply` support is explicit: libvirt/Redfish lab providers and
Redfish bare-metal providers are converged by the shipped Ansible workflows.
vSphere, OpenShift Virtualization, and IPMI are schema/validation scaffolds
until their provider roles land.

## CLI

```text
gitups init --template libvirt-redfish-hub --out desired-state
gitups bastion check -f examples/libvirt-redfish-fleet
gitups bastion apply -f examples/libvirt-redfish-fleet --yes
gitups provider check -f examples/libvirt-redfish-fleet --dry-run
gitups provider apply -f examples/libvirt-redfish-fleet --dry-run
gitups clusters check -f examples/libvirt-redfish-fleet --dry-run
gitups clusters apply -f examples/libvirt-redfish-fleet --dry-run
gitups clusters destroy -f examples/libvirt-redfish-fleet --dry-run
```

Public scopes: `bastion`, `provider`, `clusters`, and `hub` (reserved for
clusters declaring `role: hub`). Each scope exposes `check`, `apply`, and
`destroy`. Standalone commands: `init` and `secrets`. The formal CLI
contract lives in
[specs/state-model.md](specs/state-model.md#cli-contract).

## Repository Layout

```text
specs/      Source-of-truth definitions for humans and agents
docs/       Human workflow documentation
.agents/    Project-local coding-agent skills
api/        Versioned desired-state types
cmd/        CLI entrypoints
internal/   Private implementation packages
ansible/    Embedded workflow playbooks and roles
examples/   Canonical user-authored example sets
test/       Test fixtures and end-to-end cases
```

Examples must stay safe to commit: no kubeconfigs, pull secrets, private keys,
tokens, plaintext credentials, personal usernames, or private absolute paths.
