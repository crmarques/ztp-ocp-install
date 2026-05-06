# Project-Local Agent Guidance

Project definitions live in `/specs/` and are the source of truth for humans
and coding agents. Load only the specs and skills needed for the current task.

## Required Load Order

1. `/specs/README.md`
2. `/specs/index.md`
3. Task-specific specs listed in the index
4. Relevant skills under `/.agents/skills/`

## Skills

| Skill | Use When |
| --- | --- |
| `definition-stewardship` | Changing specs, ADRs, docs, examples, E2E fixture names, or agent guidance |
| `ztp-architecture` | Designing OpenShift ZTP behavior, provider boundaries, hub bootstrap, ACM, GitOps, or lab Redfish emulation |
| `repo-stewardship` | Changing repository layout, generated-output boundaries, tests, or security hygiene |
| `security-analysis` | Reviewing secrets, credentials, permissions, command execution, supply chain, or TLS/trust handling |
| `go-dependencies` | Adding, upgrading, replacing, or removing Go module dependencies |
| `code-quality` | Adding, modifying, deleting, or reviewing Go code |
| `implementation-validation` | Final validation before completing implementation work |

## Knowledge Base

When a user reports an error or an unexpected failure, check `.agents/knowledge/KNOWLEDGE.md`
for a matching symptom or error fragment before investigating. Load only the matching file; do
not scan or bulk-load the full knowledge directory.

| Index | Location |
| --- | --- |
| Category + symptom map | `.agents/knowledge/KNOWLEDGE.md` |

## Operating Rules

- Desired state is the user-facing API.
- Specs own normative rules; docs teach workflows and link back to specs.
- Examples and E2E fixtures must not contain secrets, kubeconfigs, private
  keys, tokens, personal usernames, or private absolute paths.
- `v1alpha1` can break cleanly: do not add migrations, aliases, compatibility
  shims, or legacy examples.
- Before completing implementation work, run the validation required by the
  applicable skills and report anything that could not be run.
