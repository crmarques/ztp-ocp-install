# Apply hangs at BMC wait-tasks: stale systemd units holding ports

**Symptom:** `gitups apply infra` or `apply all` hangs indefinitely at provider BMC/wait-tasks. Preflight reports a port already in use.

**Root cause:** `gitups-sushy-<name>`, `gitups-vmedia-<name>`, and `gitups-boot-artifacts-<name>` systemd units from a **prior provider name** (renamed or removed from the state file) are still running and holding the BMC/vmedia/boot-artifacts ports. The new provider phase tries to bind those ports and waits forever.

**Fix:** Stop and disable the stale units. Identify them with:
```
systemctl list-units 'gitups-*' --all
```
Then `systemctl stop` / `systemctl disable` the ones whose name no longer matches any InfrastructureProvider in the current state.

**Preflight gate:** `bmcPortChecks` (internal/cli/preflight.go) probes these three ports and passes when the port is held by the *expected* unit for the current provider (idempotent re-apply). It fails when an unrelated process holds the port.

**Ports probed per provider:**
- Redfish: `bindAddress:port` (default `0.0.0.0:8000`)
- vmedia HTTP: `127.0.0.1:port+1`
- boot-artifacts HTTP: `0.0.0.0:port+2`
