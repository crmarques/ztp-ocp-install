# Bootwright

Bootwright is a desired-state orchestrator that takes a fleet from bare hardware
to an autonomous, GitOps-managed system. You author versioned YAML, Bootwright
validates it, renders deterministic inputs for OpenShift, Ansible, and
GitOps, then converges the environment in ordered phases.

The CLI covers the full pipeline:

```text
bootwright init workspace --cluster-name ocp-bm-01 --provider bare-metal
                                          author bootstrap repo desired state
bootwright apply infra                         install/configure infra (libvirt, bare metal, …)
bootwright render installer        render OpenShift install files
bootwright apply clusters --scope ocp-bm-01    install clusters via openshift-install agent
bootwright apply hub                           validate/apply hub components for the hub-role cluster
bootwright apply gitops <package-set>          render package compositions, push to git, bootstrap KRC/SRC
```

The `gitops` target consumes a user-authored
`apiVersion: bootwright.io/v1alpha1, kind: GitOpsPackageSet` that names catalog
sources (filesystem path / OCI artifact / git URL), output repositories,
package selections, and the KRC/SRC controllers that own ongoing
reconciliation. Packages are sourced from the autonomous `gitops-packages`
catalog repo via the filesystem, OCI, or git driver.

## Start Here

| Audience | Start |
| --- | --- |
| Users running a lab | [Quickstart](docs/quickstart.md) |
| Users authoring desired state | [Desired State](docs/desired-state.md) |
| Contributors mapping implementation ownership | [Architecture](docs/architecture.md) |
| Contributors and coding agents | [Specs](specs/index.md) |
| Architecture decisions | [ADRs](specs/adr/README.md) |

Human docs explain the workflow. Specs are the source of truth for API shape,
architecture boundaries, validation rules, CLI behavior, and security posture.

## Desired-State Contract

User-authored YAML uses `apiVersion: bootwright.io/v1alpha1` and four kinds:

| Kind | Owns |
| --- | --- |
| `Environment` | Shared environment defaults: base domain, OpenShift install mode, secret sources, OpenShift release, component image pins |
| `InfrastructureProvider` | Provider connections and capabilities: libvirt, bare metal, managed HAProxy, mirror registry, managed Squid proxy, and future vSphere/OpenShift Virtualization scaffolds |
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
bootwright init workspace --cluster-name managed-01 --provider emulated-bare-metal
bootwright check bastion -f examples/libvirt-redfish-fleet
bootwright apply bastion -f examples/libvirt-redfish-fleet --yes
bootwright check infra -f examples/libvirt-redfish-fleet --dry-run
bootwright apply infra -f examples/libvirt-redfish-fleet --dry-run
bootwright render installer -f examples/libvirt-redfish-fleet --scope managed-01
bootwright apply clusters -f examples/libvirt-redfish-fleet --scope managed-01 --dry-run
bootwright apply all -f examples/libvirt-redfish-fleet --dry-run

bootwright init gitops dev
bootwright expand gitops dev
bootwright check gitops dev
bootwright render gitops dev
bootwright push gitops dev --base-url https://github.com/myorg
bootwright apply gitops dev --to <kubectl-context>
```

The CLI is verb-first; every subcommand picks a target. Provisioning
targets are `bastion`, `infra`, `clusters`, `hub`, and `all`; the gitops
target is `gitops <name>`. Verbs are `init`, `secret`, `check`, `status`,
`expand`, `fill`, `plan`, `render`, `push`, `apply`, `wait`, `diff`, and
`destroy`. The formal CLI contract lives in
[specs/state-model.md](specs/state-model.md#cli-contract); the gitops
contract lives in [specs/gitops.md](specs/gitops.md).

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
