# Gitups E2E Cases

Each subdirectory is one end-to-end scenario. The case README covers what is
specific to that scenario (cluster shape, desired-state files, provider
preflight, install, verify, destroy). The bastion-side setup that does not
change between cases lives in one of these two shared files:

- [containerized-bastion.md](containerized-bastion.md) — Gitups runs inside a
  non-root UBI9 container on the operator's machine with `--network host`.
  Good for ephemeral, fully isolated e2e runs on a dev workstation.
- [bastion.md](bastion.md) — Gitups runs directly on a Linux VM or physical
  host that you SSH into. Closer to a real operator setup; the bastion
  machine's lifecycle is managed outside of Gitups.

Each case README links to whichever bastion mode it uses.
