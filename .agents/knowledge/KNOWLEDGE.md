# Knowledge Index

Error and constraint knowledge extracted from code history. Match the reported symptom or error text, then load only the linked file.

| Category | Match symptoms / keywords | File |
| --- | --- | --- |
| Ansible / sudo | `Duplicate become password prompt`; apply hangs mid-play on sudo | [sudo-ansible-duplicate-prompt.md](sudo-ansible-duplicate-prompt.md) |
| Ansible / sudo | `sudo gitups <scope> apply` cannot find ansible; `ModuleNotFoundError` under sudo | [pip-user-sudo-pythonpath.md](pip-user-sudo-pythonpath.md) |
| Ansible / embed | Extracted bundle missing `_respawn.py`, `__init__.py`, dot/underscore files | [ansible-embed-underscore-files.md](ansible-embed-underscore-files.md) |
| Ansible / roles | `gitups_current_cluster is undefined`; dynamic role import fails | [ansible-dynamic-role-dispatch.md](ansible-dynamic-role-dispatch.md) |
| Ansible / runtime | `Module result deserialization failed`; `rc=-15`; cleanup killed Ansible wrapper | [ansible-module-wrapper-pkill.md](ansible-module-wrapper-pkill.md) |
| Provider / BMC | Apply hangs at BMC wait tasks; port already in use after provider rename | [stale-bmc-port-wait-hang.md](stale-bmc-port-wait-hang.md) |
| OpenShift install | Disconnected install fails TLS; agent never reaches SSH; image pull x509 error | [disconnected-trust-bundle-policy.md](disconnected-trust-bundle-policy.md) |
| OpenShift install | Mirror push x509 SAN mismatch after self-signed cert spec change | [self-signed-cert-drift.md](self-signed-cert-drift.md) |
| OpenShift install | Agent ISO cache stale; `cannot generate ISO image due to configuration errors` | [openshift-agent-iso-cache.md](openshift-agent-iso-cache.md) |
| Libvirt / network | API VIP unreachable; `Bootstrap Kube API never initialized` | [libvirt-vip-bootstrap.md](libvirt-vip-bootstrap.md) |
| Libvirt / network | Network UUID mismatch; bridge already in use; stale libvirt XML | [libvirt-network-drift.md](libvirt-network-drift.md) |
| Redfish / boot | `InsertMedia` fails; emulator HTTPS mismatch; virtual media path mismatch | [redfish-virtual-media.md](redfish-virtual-media.md) |
| Python / Ansible | Python 3.12 CIDR check returns false; VIP not matched to bridge CIDR | [python-312-cidr-filter.md](python-312-cidr-filter.md) |
