# gitops-controllers

Repo rendered by gitups from FullProvision "test1".

## Packages

- **argocd** (local/argocd, install, renderer=helm, role=kubernetes-resource-controller) -> `packages/argocd/install/helm`

## Unfilled placeholders at render time

- `spec.packages[argocd-managed-repo-basic-infra-dev].resolvedValues.repoURL` — git URL for the managed output repo (set per env)
- `spec.packages[argocd-managed-repo-basic-infra].resolvedValues.repoURL` — git URL for the managed output repo (set per env)
- `spec.packages[argocd-managed-repo-gitops-controllers-dev].resolvedValues.repoURL` — git URL for the managed output repo (set per env)
