# Architecture Decision Records

ADRs capture cross-cutting architectural decisions and their context. Each
file is current; superseded ADRs are removed rather than carried forward.

| ADR | Title | Status |
| --- | --- | --- |
| [0001](0001-foundational-desired-state-api.md) | Foundational Desired State API | Accepted |
| [0002](0002-ansible-provider-dispatch.md) | Ansible Provider Dispatch | Accepted |
| [0003](0003-gitops-stage-merger.md) | GitOps stage merger | Accepted |

## Authoring Rules

- One file per decision, numbered sequentially: `NNNN-short-slug.md`.
- Required sections: `Status`, `Context`, `Decision`, `Consequences`.
- Keep ADRs focused on the decision and its tradeoffs; enforced rules
  belong in the relevant spec, not the ADR.
- Write a new ADR for a new cross-cutting decision. When a later decision
  replaces an old one, update the specs with the surviving rule and remove
  the superseded ADR file.
