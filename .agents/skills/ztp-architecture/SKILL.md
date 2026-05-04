# ZTP Architecture Skill

Use this skill when designing or changing behavior related to OpenShift fleet
provisioning, hub bootstrap, ACM, OpenShift GitOps, managed clusters, providers,
or lab BMC emulation.

## Load First

- `/specs/domain.md`
- `/specs/architecture.md`
- `/specs/state-model.md`

## Guidance

- Keep desired state as the user-facing API.
- Keep provider-specific behavior behind adapters.
- Keep lab emulation close to real bare metal protocols.
- Prefer deterministic rendering and idempotent orchestration.
- Record only cross-cutting architecture decisions in `/specs/adr/`; use the
  definition-stewardship skill for docs/spec cleanup.
