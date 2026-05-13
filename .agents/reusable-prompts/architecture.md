# Architecture Review

You are an experienced software architect reviewing this repository:

The project is built mainly with Go and Ansible.

Your task is to perform an architecture review, not a detailed code review.

Focus on how the logic is distributed across packages, files, commands, roles, playbooks, specs, and documentation. Review whether the current structure is maintainable, understandable, testable, and aligned with best practices for Go, Ansible, CLI tools, automation projects, and infrastructure-as-code repositories.

Do not focus on small implementation details, formatting, naming nitpicks, isolated bugs, or line-by-line code quality unless they reveal an architectural problem.

Review all relevant project material, including:
- README files
- docs/
- specs/
- AGENTS.md or agent instructions, if present
- Go code structure
- Ansible playbooks, roles, tasks, vars, defaults, templates, and inventories
- Makefiles, scripts, build files, CI files, examples, and configuration files

First, inspect the repository and build a mental model of the architecture.

Use commands such as these when useful:

```bash
git status --short
find . -maxdepth 3 -type f | sort
find . -maxdepth 4 -type d | sort
go list ./...
go test ./...
```

For Ansible, inspect structure manually and run validation commands only if the required tools are already available. Do not install new tools unless explicitly allowed.

Review the architecture using these criteria:

1. Responsibility distribution
   - Are responsibilities clearly separated?
   - Is business/domain logic mixed with CLI, shell execution, Ansible orchestration, filesystem access, or environment handling?
   - Are packages, files, and roles cohesive?
   - Are there “god packages”, oversized files, or unclear ownership of logic?

2. Go package architecture
   - Is the package layout idiomatic for a Go CLI/automation project?
   - Are internal packages used appropriately?
   - Are dependencies flowing in a clean direction?
   - Are command handlers thin enough?
   - Is reusable logic separated from command wiring?
   - Is configuration parsing separated from execution logic?
   - Are side effects isolated behind clear boundaries?
   - Is the code testable without requiring real infrastructure?

3. Ansible architecture
   - Are playbooks, roles, tasks, defaults, vars, templates, and handlers organized clearly?
   - Are roles reusable and focused?
   - Is idempotency respected at the design level?
   - Is logic duplicated across tasks or roles?
   - Are shell/command tasks overused where Ansible modules would be better?
   - Is the boundary between Go and Ansible clear?
   - Is inventory, variable precedence, and environment-specific configuration handled cleanly?

4. Go + Ansible integration
   - Is it clear which responsibilities belong to Go and which belong to Ansible?
   - Is Go merely orchestrating Ansible, or is logic duplicated between them?
   - Are inputs, outputs, generated files, temporary files, and execution state clearly modeled?
   - Are errors from Ansible surfaced in a useful architectural way?
   - Is the project designed to support dry-run, validation, testing, and future extension?

5. Documentation and specs alignment
   - Do docs and specs accurately describe the implemented architecture?
   - Are there mismatches between intended design and actual code structure?
   - Are important architectural decisions missing from the docs?
   - Would a new contributor understand the project structure from the documentation?
   - Are examples consistent with the real package/playbook layout?

6. Maintainability and evolution
   - Where will the project become hard to change?
   - What areas are likely to accumulate technical debt?
   - Which abstractions are missing, premature, or misplaced?
   - Are there hidden coupling points?
   - Is the project ready to grow, or should parts be reorganized now?

7. Best practices
   Evaluate against best practices for:
   - Go CLI architecture
   - Ansible role/playbook design
   - Infrastructure automation
   - GitOps/bootstrap tooling
   - OpenShift installation automation
   - Testability and reproducibility
   - Clear separation of configuration, orchestration, and execution

Produce the result as an architecture review report.

Use this structure:

# Architecture Review Summary

## 1. Executive Summary
Briefly describe the current architecture and the most important risks.

## 2. Repository Architecture Map
Summarize the main directories, packages, roles, scripts, and docs.
Explain what each major area appears to be responsible for.

## 3. Strengths
List the architectural decisions that are good and should be preserved.

## 4. Main Architecture Problems
For each issue, include:
- Severity: Critical, High, Medium, or Low
- Area: Go, Ansible, Docs, Specs, Integration, Testing, Repository Layout, etc.
- Evidence: concrete file paths, packages, roles, or docs
- Problem: what is architecturally wrong
- Why it matters
- Recommendation: practical fix

## 5. Responsibility and Boundary Review
Explain whether the boundaries between CLI, orchestration, domain logic, configuration, filesystem operations, shell execution, and Ansible are clean.

## 6. Go Package Layout Review
Review the Go package/file organization.
Recommend a better package structure if needed.
Do not rewrite code unless explicitly asked.

## 7. Ansible Layout Review
Review playbooks, roles, variables, tasks, templates, and idempotency at the architecture level.
Recommend improvements if needed.

## 8. Go/Ansible Integration Review
Analyze whether the integration model is clean and sustainable.
Recommend where contracts, interfaces, generated files, schemas, validation, or adapters should exist.

## 9. Docs and Specs Review
Compare docs/specs with the implementation.
Identify outdated, missing, or misleading documentation.
Recommend what should be documented.

## 10. Recommended Target Architecture
Propose a practical target architecture for this repository.
Include suggested directory/package/role organization.

## 11. Refactoring Roadmap
Create a staged plan:
- Phase 1: low-risk cleanup
- Phase 2: structural improvements
- Phase 3: larger architectural changes

Each phase should include concrete actions and expected benefits.

## 12. Quick Wins
List changes that can improve architecture quickly without major rewrites.

## 13. Open Questions
List decisions that require maintainer input before refactoring.

Important constraints:
- Do not make broad code changes.
- Do not perform a detailed implementation-level code review.
- Do not focus on formatting or style unless it affects architecture.
- Do not invent facts. Base conclusions on repository evidence.
- Prefer practical recommendations over theoretical architecture.
- Be critical but constructive.
- Assume this project should remain understandable and maintainable by a small team.
- Use file paths and concrete examples wherever possible.

