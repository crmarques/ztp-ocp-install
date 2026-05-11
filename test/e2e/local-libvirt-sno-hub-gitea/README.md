# Local Libvirt SNO Hub + Gitea (gitops-bootstrapped)

**Install type:** connected (`Environment.spec.ocpInstall.connected`).

The control host is itself the libvirt provider host (SSH back to `localhost`)
and runs a single SNO cluster that acts as the hub. Once the SNO is up, the
hub is bootstrapped by the gitops stage: an Argo CD KRC plus a Gitea workload
rendered from the [`gitea`](../../../../gitops-workspace/gitups-packages/packages/gitea/)
package definition.

## Topology

| Cluster | Role | Topology | Network |
| --- | --- | --- | --- |
| `local-libvirt-sno-hub` | hub | single-node | `cluster-infrastructure-hub.yaml: spec.networks.primary.cidr` |

The provider host is `local-libvirt-host` reached via
`provider.yaml: spec.hosts.local-libvirt-host.ssh.address` (defaults to `localhost`).

## Layout on disk

```text
test/e2e/local-libvirt-sno-hub-gitea/      # cluster-infra fixture (this dir)
├── environment.yaml
├── provider.yaml
├── cluster-infrastructure-hub.yaml
└── ocp-cluster-hub.yaml
test/e2e/gitops/local-libvirt-sno-hub-gitea/   # gitops package set
└── gitops-package-set.yaml
```

The two trees are split because the infra loader rejects any kind it does not
own (`GitOpsPackageSet`), and the gitops CLI looks up its workspace under
`-d <dir>/<name>/`. Splitting matches the existing `make e2e` /
`make e2e-gitops` Makefile targets without further plumbing.

## Secrets

Secret files live outside the repo under `~/.gitups/secrets` by default.

| Secret name | Purpose |
| --- | --- |
| `openshift-pull-secret` | OCP pull secret JSON |
| `cluster-admin-key` | Public SSH key installed into OCP nodes |
| `local-libvirt-host-admin-key` | SSH key for the local provider host when SSH is required |
| `local-libvirt-sno-hub-bmc-credentials` | Redfish emulator credentials |

```text
install -d -m 0700 ~/.ssh ~/.gitups/secrets
ssh-keygen -t ed25519 -f ~/.ssh/gitups-ssh-key -N '' -C gitups-local-libvirt-sno-hub
install -m 0600 /dev/stdin ~/.ssh/authorized_keys < <(cat ~/.ssh/authorized_keys 2>/dev/null; cat ~/.ssh/gitups-ssh-key.pub)
gitups secrets pull-secret set --name openshift-pull-secret --from-file ~/pull-secret.json
gitups secrets generate -f test/e2e/local-libvirt-sno-hub-gitea
```

## 1. Bring up the SNO hub

```text
make e2e-dry-run CASE=local-libvirt-sno-hub-gitea
make e2e         CASE=local-libvirt-sno-hub-gitea
```

Equivalent CLI flow:

```text
gitups bastion  check  -f test/e2e/local-libvirt-sno-hub-gitea
gitups provider apply  -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --yes
gitups clusters apply  -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --yes
```

When `clusters apply` finishes, the kubeadmin kubeconfig lands in the state
directory:

```text
export KUBECONFIG=/tmp/gitups-local-libvirt-sno-hub-gitea/clusters/local-libvirt-sno-hub/auth/kubeconfig
oc get nodes
```

## 2. Bootstrap the gitops flow on the hub

The package set under `test/e2e/gitops/local-libvirt-sno-hub-gitea/` declares
two repositories:

| Repo | Package | Install | Role |
| --- | --- | --- | --- |
| `gitops-controllers` | `argocd` | helm | KRC for the hub |
| `workloads` | `gitea` | helm | Source-control workload |

Both have `-{{.Env}}` overlays so per-env tweaks land in their own repo without
forking the generic one.

```text
make e2e-gitops CASE=local-libvirt-sno-hub-gitea
```

That target runs `gitops check`, `gitops expand`, and `gitops render` against
the fixture (no `apply`/`push` — read-only against the cluster). To actually
apply onto the SNO hub:

```text
KUBE_CONTEXT=$(oc config current-context)
WORKSPACE=test/e2e/gitops

./bin/gitups gitops fill   local-libvirt-sno-hub-gitea -d $WORKSPACE \
  --set argocd-managed-repo-gitops-controllers.repoURL=https://example.invalid/gitops/gitops-controllers.git \
  --set argocd-managed-repo-gitops-controllers-dev.repoURL=https://example.invalid/gitops/gitops-controllers-dev.git \
  --set argocd-managed-repo-workloads.repoURL=https://example.invalid/gitops/workloads.git \
  --set argocd-managed-repo-workloads-dev.repoURL=https://example.invalid/gitops/workloads-dev.git
./bin/gitups gitops render local-libvirt-sno-hub-gitea -d $WORKSPACE --context "$KUBE_CONTEXT" --prune
./bin/gitups gitops apply  local-libvirt-sno-hub-gitea -d $WORKSPACE --to "$KUBE_CONTEXT" --wait-crds
./bin/gitups gitops wait   local-libvirt-sno-hub-gitea -d $WORKSPACE --to "$KUBE_CONTEXT" --timeout 15m
```

The `argocd-managed-repo-*.repoURL` placeholders only need real URLs once
Gitea is up and `gitops push` is wired to it. For the first apply they can
point at any URL since the smoke does not push.

## 3. Verify Gitea

The default chart values turn off persistence and the bundled Postgres/Redis
so a first apply is fast. `rootURL` defaults to
`http://gitea.gitea.svc.cluster.local:3000/` — reach it from the laptop with
a port-forward:

```text
oc -n gitea port-forward svc/gitea-http 3000:3000
# then open http://localhost:3000
```

Initial admin credentials follow the chart defaults (see
[`packages/gitea/README.md`](../../../../gitops-workspace/gitups-packages/packages/gitea/README.md)
for current values and overrides).

## 4. Tear down

```text
make e2e-destroy CASE=local-libvirt-sno-hub-gitea
```

Or step-by-step:

```text
gitups gitops   destroy local-libvirt-sno-hub-gitea -d test/e2e/gitops --to "$KUBE_CONTEXT" --yes
gitups clusters destroy -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --yes
gitups provider destroy -f test/e2e/local-libvirt-sno-hub-gitea --state-dir /tmp/gitups-local-libvirt-sno-hub-gitea --yes
```

## Prerequisites on the laptop

- Libvirt + QEMU + a working `qemu:///system` connection for the invoking user.
- `ansible-playbook` on `$PATH` (or pass `ANSIBLE_PLAYBOOK=`).
- `make build` produces `./bin/gitups`.
- For step 2 only: a sibling `gitops-workspace/gitups-packages` checkout
  alongside this repo (the package set's `sources[0].filesystem.path`
  resolves five levels up, then into `gitops-workspace/gitups-packages/packages`).
- `helm` and `kustomize` on `$PATH` for `gitops render`.
