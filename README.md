# Gitups

Gitups is a desired-state orchestrator for OpenShift cluster fleets. You
author versioned YAML, Gitups validates it, renders deterministic inputs for
OpenShift, Ansible, and GitOps, then converges the environment in ordered
phases.

The project is optimized for zero touch provisioning: one hub cluster runs ACM
and OpenShift GitOps, and managed clusters are reconciled from declared fleet
state.

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
| `InfrastructureProvider` | Provider connections and capabilities: QEMU/KVM, bare metal, vSphere, OpenShift Virtualization |
| `ClusterInfrastructure` | One cluster's realised infrastructure: networks, machines, endpoints, load balancers, name resolution |
| `OCPCluster` | Provider-neutral OpenShift intent: role, topology, install method, networking, node identity |

`OCPCluster` stays provider-neutral. Swapping from QEMU/KVM with Redfish
emulation to real bare metal edits only `InfrastructureProvider` and
`ClusterInfrastructure`.

## CLI

```text
gitups doctor
gitups setup controller -f examples/qemu-redfish-fleet
gitups init --template qemu-redfish-hub --out desired-state
gitups validate -f examples/qemu-redfish-fleet
gitups preflight -f examples/qemu-redfish-fleet
gitups plan -f examples/qemu-redfish-fleet --out text
gitups apply infra -f examples/qemu-redfish-fleet --dry-run
gitups apply hub -f examples/qemu-redfish-fleet --dry-run
gitups status -f examples/qemu-redfish-fleet --diff
```

Public verbs: `doctor`, `setup controller`, `init`, `validate`, `preflight`,
`plan`, `apply <infra|hub|clusters>`, `destroy <infra|hub|clusters|all>`,
`status`, and `secrets`.
The formal CLI contract lives in
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
