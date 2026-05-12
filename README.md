# Gitups

Gitups is a desired-state orchestrator that takes a fleet from bare hardware
to an autonomous, GitOps-managed system. You author versioned YAML, Gitups
validates it, renders deterministic inputs for OpenShift, Ansible, and
GitOps, then converges the environment in ordered phases.

The CLI covers the full pipeline:

```text
gitups init workspace --cluster-name ocp-bm-01 --provider bare-metal
                                          author bootstrap repo desired state
gitups apply infra                         install/configure infra (libvirt, bare metal, …)
gitups render installer        render OpenShift install files
gitups apply clusters --scope ocp-bm-01    install clusters via openshift-install agent
gitups apply hub                           validate/apply hub components for the hub-role cluster
gitups gitops apply <package-set>          render package compositions, push to git, bootstrap KRC/SRC
```

The `gitops` group consumes a user-authored
`apiVersion: gitups.io/v1alpha1, kind: GitOpsPackageSet` that names catalog
sources (filesystem path / OCI artifact / git URL), output repositories,
package selections, and the KRC/SRC controllers that own ongoing
reconciliation. Packages are sourced from the autonomous `gitops-packages`
catalog repo via the filesystem, OCI, or git driver.

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
gitups init workspace --cluster-name managed-01 --provider emulated-bare-metal
gitups check bastion -f examples/libvirt-redfish-fleet
gitups apply bastion -f examples/libvirt-redfish-fleet --yes
gitups check infra -f examples/libvirt-redfish-fleet --dry-run
gitups apply infra -f examples/libvirt-redfish-fleet --dry-run
gitups render installer -f examples/libvirt-redfish-fleet --scope managed-01
gitups apply clusters -f examples/libvirt-redfish-fleet --scope managed-01 --dry-run
gitups apply all -f examples/libvirt-redfish-fleet --dry-run

gitups gitops init dev
gitups gitops expand dev
gitups gitops check dev
gitups gitops render dev
gitups gitops push dev --base-url https://github.com/myorg
gitups gitops apply dev --to <kubectl-context>
```

Provisioning commands are verb-first: `check`, `render`, and `apply`
operate on targets such as `bastion`, `infra`, `clusters`, `hub`, and `all`.
The `gitops` group still exposes `init`, `expand`, `check`, `render`, `fill`,
`plan`, `push`, `apply`, `wait`, `status`, and `destroy`. Standalone commands:
`init workspace` and `secrets`. The formal CLI contract lives in
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
