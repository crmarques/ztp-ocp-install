# Code Quality Skill

Use this skill when adding, modifying, deleting, or reviewing Go code in this
repo to keep the source clean and idiomatic, and to guarantee that no unused
parameters, functions, methods, types, variables, imports, or fields are
left behind.

## Load First

- `/specs/architecture.md` (testing and repository layout sections)

## Required Checks

Run from the repo root before declaring an implementation task done, and
report any check that could not be run, including the reason:

- `gofmt -l .` — must print nothing. If anything is listed, run
  `gofmt -w` on the listed paths and re-run.
- `go vet ./...` — must report no findings.
- `go build ./...` — the Go compiler rejects unused imports and unused
  local variables. A clean build is the baseline for "nothing unused".
- `staticcheck ./...` — must report no findings. Pay special attention
  to `U1000` (unused). If `staticcheck` is not installed, install it
  once with:
  `go install honnef.co/go/tools/cmd/staticcheck@latest`
  and ensure `$(go env GOPATH)/bin` is on `PATH`.

`go test -race ./...` (from the `implementation-validation` skill) must
also pass; deletions in this skill can break tests that imported the
removed symbol.

## Standards

- No dead code. Delete unused exported and unexported functions,
  methods, types, constants, variables, and struct fields rather than
  leaving them "for later" or commented out.
- No unused imports or unused local variables. Never silence the
  compiler with blank-identifier (`_`) imports unless the import is
  genuinely needed for its side effects; in that case add a one-line
  `// why` comment.
- No unused parameters. Remove parameters that are never read inside
  the function body. The single exception is when the parameter is
  required to satisfy an interface, an embedded function type, or an
  external callback signature; in that case rename the parameter to
  `_` if the name does not aid readability.
- No unused return values. If a return value is never consumed at any
  call site within the repo, drop it from the signature instead of
  ignoring it at every call.
- No backwards-compatibility shims, renamed `_var` placeholders, or
  `// removed` markers left in place of deleted code. Delete cleanly.
- No speculative abstractions. Three similar lines is better than a
  premature interface, generic helper, or options struct introduced
  "just in case".
- No comments by default. Code is the documentation; names and
  structure must carry the meaning. A comment is allowed only when
  the *why* is non-obvious to a future reader who has the code in
  front of them — a hidden constraint, a subtle invariant, or a
  workaround whose intent cannot be inferred from the code itself.
  Specifically forbidden:
  - Comments that restate what the code already says.
  - "Fix"/"bug"/"after refactor"/"changed because…" notes left over
    from edits. The diff and commit history record that. Delete the
    comment and rely on the surrounding code reading cleanly.
  - `// removed`, `// kept for compatibility`, `// see PR #…`,
    or `// TODO` markers that do not point to a tracked work item.
  - Block comments above a function that re-narrate its body.
  If a comment seems necessary, first try renaming, splitting, or
  restructuring the code so it is self-explanatory; only write the
  comment when that fails.

## Method

- Before finishing, run the checks above and fix every finding within
  the scope of the change.
- When deleting a function, also delete its tests and any helpers it
  was the last caller of; re-run `staticcheck` to surface the next
  layer of newly-unused code, and repeat until clean.
- When a finding is genuinely out of scope, surface it as a note in
  the handoff rather than silently expanding the diff.
- Prefer fixing the root cause (delete the dead symbol, drop the
  unused parameter) over compensating annotations such as `//nolint`
  or `_ = unusedVar`.
