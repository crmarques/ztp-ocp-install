# GitOps Spec

The gitops command leaves (`init gitops`, `check gitops`, `render gitops`, `apply gitops`, …) are the post-provisioning concern of the
gitups CLI. It turns a declarative package set into rendered git repos and
bootstraps a target cluster with the KRC/SRC controllers that own ongoing
reconciliation.

## Kinds

All public objects use `apiVersion: gitups.io/v1alpha1`.

| Kind | Owns |
| --- | --- |
| `GitOpsPackageSet` | User-authored intent plus optional machine-expanded execution state in `spec.resolved`. |
| `PackageDefinition` | Per-package descriptor authored in the catalog (`packages/<name>/package.yaml`). Loaded by the catalog driver; never written by gitups. |

`GitOpsPackageSet` is a peer GitOps authoring artifact, not a member of the
cluster-infra `State` aggregate ([state-model.md](state-model.md)). Cluster
infra kinds are consumed by `internal/infra/LoadNormalizeValidate`; GitOps
kinds are consumed by `internal/gitops/load.PackageSet` and
`ExpandedPackageSet`.

## Non-Negotiable Invariants

1. **One object, two profiles.** A minimal `GitOpsPackageSet` carries user
   intent. `gitups expand gitops` writes the same kind with
   `spec.resolved` populated under `.gitups/expanded/`.
2. **Lean input.** User-authored fields carry only sources, repositories,
   selected templates, controllers, and bindings. Defaults and inferable
   fields belong in `package.yaml` and `spec.resolved`.
3. **Generated output is disposable.** Expanded and rendered artifacts live
   under `.gitups/`; users edit the source `gitops-package-set.yaml`.
4. **Install vs resources.** A package is one service/application. It declares
   install methods under `install/<renderer>/descriptor.yaml` and optional
   custom resources under `resources/<resourceTemplate>/descriptor.yaml`.
5. **Renderer priority OLM → Kustomize → Helm → raw.** Apply when choosing an
   install method and when picking the renderer for a resource descriptor.
6. **Repositories are user intent.** `spec.repositories[]` is the only source
   of output repo placement. Package descriptors do not declare `targetRepo`.
7. **Controllers are first-class.** Every package declares
   `role: kubernetes-resource-controller | service-resource-controller |
   workload`. `spec.controllers.{kubernetesResources,serviceResources}`
   assigns KRC/SRC roles to selected package instances.
8. **Prefer KRC over SRC.** When desired state can be expressed as Kubernetes
   CRs, route it through the KRC. Use SRC only when the target has no stable CR.
9. **Capabilities are open extension points.** Packages declare `provides[]`
   / `requires[]` with free-form capability names.
10. **Hook ABI is stable.** Hooks are invoked as
    `<script> --phase <phase> --values <json-path> --out <render-dir>` and may
    write only inside `--out`.
11. **Generation is deterministic and hermetic.** Same inputs produce identical
    bytes. No timestamps, random IDs, or network calls during render except
    pinned `helm template --repo` chart pulls.
12. **Pin everything.** Chart versions, `startingCSV`, catalog sources, image
    tags, and upstream URLs must be exact. Never `latest` or floating ranges.
13. **Cluster access is explicit and KRC-declared.** `apply` is the only
    mutating cluster command. The selected KRC's `spec.cli.binary` and intent
    arg templates drive cluster operations.
14. **Push is the only outbound mutating command.** It publishes rendered repos
    through a git provider and reads credentials from flags/env only, never from
    desired state or rendered trees.

## GitOpsPackageSet Shape

Minimal input:

```yaml
apiVersion: gitups.io/v1alpha1
kind: GitOpsPackageSet
metadata:
  name: dev
spec:
  sources:
    - name: local
      filesystem:
        path: ./packages
  repositories:
    - name: platform
      type: kubernetes-resources
      packages:
        - template: local/metallb
```

Expanded form uses the same object and adds `spec.resolved`:

```yaml
spec:
  resolved:
    repository:
      layout: split
      outputPath: .gitups/render/dev
    repositories: []
    packages: []
    placeholders: []
```

`spec.resolved` is machine-owned. It may carry resolved package descriptors,
render destinations, package instances, placeholder requirements, hook/render
plans, and apply ordering metadata. It must not carry generated secret values.

## Catalog Source Drivers

`spec.sources[]` discriminates by structural sub-block. Exactly one of
`filesystem`, `oci`, or `git` must be set per source.

- **filesystem** — Resolved relative to the `GitOpsPackageSet` file. This is
  the local-development source driver.
- **oci** — Per-package artifact pull via `oras`. Sources must be pinned by
  digest.
- **git** — Shallow clone at an immutable tag or commit SHA. Branches and
  `HEAD` are rejected. `git.path` must stay inside the clone.

## Workspace Model

| Artifact | Location | Why |
| --- | --- | --- |
| `gitops-package-set.yaml` | `<workspace>/<name>/` | User-authored input |
| Expanded `GitOpsPackageSet` | `<workspace>/<name>/.gitups/expanded/gitops-package-set.yaml` | Generated execution plan |
| Rendered repo trees | `<workspace>/<name>/.gitups/render/` | Generated repo content |
| OCI/git source caches | `<state-dir>/gitops/sources/...` | Pure derivative; safe to delete |
| Apply logs / lock | `<state-dir>/gitops/<name>/apply/` | Pure derivative |

## CLI

```text
gitups init gitops <name>            # scaffold a GitOpsPackageSet workspace
gitups expand gitops <name>          # populate spec.resolved
gitups check gitops <name>           # validate and dry-expand
gitups render gitops <name>          # render repo trees under .gitups/render
gitups fill gitops <name>            # fill non-secret placeholders
gitups plan gitops <name>            # print apply plan
gitups push gitops <name>            # publish rendered repos
gitups apply gitops <name>           # render/push/bootstrap
gitups wait gitops <name>            # poll cluster state
gitups status gitops <name>        # drift + freshness report
gitups destroy gitops <name>         # tear down bootstrap
```

## Anti-Goals

- Not a Kubernetes controller; gitops does not watch or reconcile in-cluster.
- Not a Helm/Kustomize/OLM replacement.
- Not a secrets manager; secrets surface as placeholders and generated secret
  values are never written into desired-state or expanded artifacts.
- Not a general-purpose templating engine.
