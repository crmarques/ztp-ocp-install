# Spec Index

Load only the specs needed for the task.

| Task | Specs |
| --- | --- |
| Mission, operating model, UX principles | `domain.md` |
| Pipeline, layers, adapters, orchestration, Ansible, GitOps, testing | `architecture.md` |
| Desired-state schema, layer ownership, validation, CLI contract | `state-model.md` |
| GitOpsPackageSet schema, catalog source drivers, KRC/SRC, renderer priority | `gitops.md` |
| Secrets, credentials, OCP install trust, supply chain | `security.md` |

When in doubt, start with `domain.md` and `state-model.md`. The four-layer
desired-state API is recorded in
[ADR 0001](adr/0001-foundational-desired-state-api.md). The gitops stage,
its kinds (`GitOpsPackageSet`, `PackageDefinition`), and the merge of the
formerly-separate `gitops` CLI are recorded in
[ADR 0003](adr/0003-gitops-stage-merger.md).
