# gitops-controllers-dev

Repo rendered by gitups from FullProvision "test1".

## Packages

- **argocd-instance-default** (local/argocd, resource, renderer=raw, role=kubernetes-resource-controller) -> `packages/argocd/resources/instance/default`
- **argocd-managed-repo-basic-infra** (local/argocd, resource, renderer=raw, role=kubernetes-resource-controller) -> `packages/argocd/kubernetes-resource-controller/managed-repo/basic-infra`
- **argocd-managed-repo-basic-infra-dev** (local/argocd, resource, renderer=raw, role=kubernetes-resource-controller) -> `packages/argocd/kubernetes-resource-controller/managed-repo/basic-infra-dev`
- **argocd-managed-repo-gitops-controllers-dev** (local/argocd, resource, renderer=raw, role=kubernetes-resource-controller) -> `packages/argocd/kubernetes-resource-controller/managed-repo/gitops-controllers-dev`

## Unfilled placeholders at render time

- `spec.packages[argocd-managed-repo-basic-infra-dev].resolvedValues.repoURL` — git URL for the managed output repo (set per env)
- `spec.packages[argocd-managed-repo-basic-infra].resolvedValues.repoURL` — git URL for the managed output repo (set per env)
- `spec.packages[argocd-managed-repo-gitops-controllers-dev].resolvedValues.repoURL` — git URL for the managed output repo (set per env)
