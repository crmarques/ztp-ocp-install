# Knowledge Index

Error and constraint knowledge extracted from code history. Load only the file matching the symptom.

| Symptom | File |
| --- | --- |
| Ansible `Duplicate become password prompt`; apply hangs mid-play on sudo | [sudo-ansible-duplicate-prompt.md](sudo-ansible-duplicate-prompt.md) |
| Ansible collection modules missing from extracted bundle (`_respawn.py`, `__init__`) | [ansible-embed-underscore-files.md](ansible-embed-underscore-files.md) |
| Apply hangs at provider BMC/wait-tasks; port already in use after renaming a provider | [stale-bmc-port-wait-hang.md](stale-bmc-port-wait-hang.md) |
| Disconnected install fails TLS; agent never reaches SSH; x509 error on image pull | [disconnected-trust-bundle-policy.md](disconnected-trust-bundle-policy.md) |
| Mirror push fails with x509 SAN mismatch after cert spec change | [self-signed-cert-drift.md](self-signed-cert-drift.md) |
| `sudo gitups apply` can't find ansible; `ModuleNotFoundError` under sudo | [pip-user-sudo-pythonpath.md](pip-user-sudo-pythonpath.md) |
