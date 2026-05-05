# Agent Entrypoint

This repository is governed by the project specs in `/specs/`. Before
making changes, load only the specs that match the user request.

## Required Load Order

1. Read `/.agents/README.md`.
2. Read `/specs/README.md`.
3. Read `/specs/index.md`.
4. Load the referenced domain specs needed for the task.
5. If a project-local skill applies, read it from `/.agents/skills/`.

## Operating Rules

- Preserve the long-term goal: automated, GitOps-managed provisioning of
  fleets of OpenShift clusters through zero touch provisioning.
- Treat the initial scope as direct openshift-install-agent runs against
  cluster nodes (single-node and multi-node), while keeping provider
  abstractions open for libvirt, bare metal, vSphere, OpenShift
  Virtualization, and other substrates. Multi-cluster fleet GitOps
  publication is forward-looking; not implemented yet.
- Prefer declarative desired state, idempotent orchestration, typed
  schemas, deterministic rendering, and testable adapters.
- Do not introduce secrets, kubeconfigs, pull secrets, private keys,
  tokens, or environment-specific credentials into versioned content.
- Keep docs and specs concise. Add implementation detail only when it is
  needed by current code or an accepted decision.
- Before completing implementation work, use the
  `/.agents/skills/implementation-validation/` skill.
