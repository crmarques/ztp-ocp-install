# sudo / ansible Duplicate become password prompt

**Symptom:** ansible prints `Duplicate become password prompt` or hangs mid-play waiting for a password that is never answered (or is answered wrong and silently rejected).

**Root cause:** The bootwright process has neither root privileges nor passwordless sudo. Ansible's per-task `become: true` fires a password prompt that stdin cannot satisfy when the process was launched non-interactively.

**Fix / invariant:** `ensureSudoReady` (internal/cli/apply.go) enforces the rule before any ansible execution. Two invocation patterns are supported:
1. Run as root: `sudo bootwright <scope> apply`
2. NOPASSWD sudo configured for the invoking user

Remote provider hosts are not probed from the controller — their sudo policy lives on the target box. The play surfaces the failure there if NOPASSWD is missing.

**Test pattern:** `ensureSudoReady` is a package-level `var` so tests can swap in a no-op without a real sudo check blocking CI.
