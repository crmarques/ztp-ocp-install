# Spec Index

Load only the specs needed for the task.

| Task | Specs |
| --- | --- |
| Mission, operating model, UX principles | `domain.md` |
| Pipeline, layers, adapters, orchestration, Ansible, GitOps, testing | `architecture.md` |
| Desired-state schema, layer ownership, validation, CLI contract | `state-model.md` |
| Secrets, credentials, OCP install trust, supply chain | `security.md` |

When in doubt, start with `domain.md` and `state-model.md`. The four-layer
desired-state API is recorded in
[ADR 0001](adr/0001-foundational-desired-state-api.md).
