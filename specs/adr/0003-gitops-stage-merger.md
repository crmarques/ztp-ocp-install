# ADR 0003: GitOps stage merger

## Status

Accepted.

## Context

Until this change, three repositories cooperated to deliver an OpenShift
fleet that ends up gitops-managed:

- `ztp-ocp-install-lab` (this repo) — provisioned clusters from declarative
  YAML via infra and cluster workflows.
- `gitops-workspace/gitups` — a CLI binary (also named `gitups`) that
  rendered package compositions, pushed them to a git provider, and
  bootstrapped a KRC/SRC. apiVersion `gitups/v1alpha1`.
- `gitops-workspace/gitups-packages` — a per-package OCI catalog with a
  validate/build/publish pipeline.

The user-facing tool was effectively split: provisioning ran from one
binary, the post-provision gitops workflow ran from another that happened
to share the same name. The two had divergent apiVersions
(`gitups.io/v1alpha1` vs `gitups/v1alpha1`), divergent CLI shapes (scope
pattern vs flat verbs), and divergent module identities
(`github.com/crmarques/ztp-ocp-install-lab` vs `github.com/crmarques/gitups`).

## Decision

Merge `gitops-workspace/gitups` into this repo. Keep
`gitops-workspace/gitups-packages` autonomous and consume it via three
source drivers (filesystem, OCI, git URL).

### Module identity

- Rename module to `github.com/crmarques/gitups`. Disk repo dir stays
  `ztp-ocp-install-lab` for now; rename later if useful.

### Schemas

- New kinds at `apiVersion: gitups.io/v1alpha1`:
  - `GitOpsPackageSet` (the only package-set object).
  - `PackageDefinition` (consumer-side struct; authored in the
    autonomous catalog repo).
- `GitOpsPackageSet` has two profiles: minimal user-authored intent and
  expanded execution state in `spec.resolved`.
- `GitOpsPackageSet.spec.sources[]` discriminates by **structural**
  sub-block (`filesystem`/`oci`/`git`) instead of the upstream
  `type: <string>` discriminator. This matches the authoritative R3 rule
  on structural-only discriminators.
- GitOps kinds are **peer authoring artifacts**, not members of the
  cluster-infra `State` aggregate. Loaders are split:
  `internal/infra/LoadNormalizeValidate` for cluster-infra kinds;
  `internal/gitops/load` for gitops kinds.

### CLI surface

- New top-level group `gitups gitops` peer to the provisioning workflow.
- Verbs preserve the upstream gitops vocabulary
  (`init`, `expand`, `check`, `fill`, `plan`, `push`, `apply`, `wait`,
  `status`) with one rename and one addition:
  - `generate` → `render` (avoids semantic adjacency to `gitups secrets generate`).
  - `destroy` is new (currently advisory; prints manual `kubectl delete`
    commands until KRC packages declare a destroy intent).
- The group does **not** wrap into the ansible-shaped `scopeSpec`. It
  builds a parallel command tree because gitops verbs do not run ansible
  and do not want the `--ansible-playbook`/`--secrets-dir`/
  `--host-state-dir` flag set.
- Default workspace changed from upstream `./gitups-output-dir` to
  `./gitups-workspaces` to make the directory's purpose self-describing.

### Workspace + state-dir model

- Authored YAML (`gitops-package-set.yaml`) lives in the user's workspace.
- Expanded and rendered artifacts live under `<workspace>/<name>/.gitups/`.
- Caches (OCI/git source pulls, apply intent logs) live under
  `<state-dir>/gitops/...` and are wipeable.

### Catalog source drivers

- `filesystem` driver: lifted unchanged from upstream.
- `oci` driver: new. Per-package artifact pull via `oras` shell-out.
  Cache key `(registry, tag|digest)`. Package names extracted from the
  GitOpsPackageSet's templates.
- `git` driver: new. Shallow clone at the requested ref. Branches are
  rejected (mutable); only tags or commit SHAs are accepted.

### Cross-repo apiVersion sweep

`gitops-workspace/gitups-packages` cuts a coordinated breaking change
that rewrites every `apiVersion: gitups/v1alpha1` to
`apiVersion: gitups.io/v1alpha1`. Older versions of the catalog cannot be
loaded by the merged tool, and vice versa. The catalog's per-package OCI
release pipeline (`pkg/<name>/v<version>` tags → GHCR) is unchanged.

## Consequences

- One binary `gitups` covers the full pipeline: `init workspace` → `apply infra`
  → `apply clusters` → `apply hub` → `gitops apply`.
- The autonomous catalog repo continues its independent per-package
  release lifecycle.
- The merge introduces no compatibility shims, consistent with the
  authoritative `v1alpha1` rule that breaking changes ship without
  migrations.
- Sibling `gitops-workspace/gitups` is retired after the merge branch
  lands.
