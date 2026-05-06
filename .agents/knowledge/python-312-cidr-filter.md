# Python 3.12 CIDR checks return false

**Symptom:** Managed load-balancer VIPs are not assigned to the libvirt bridge, and API VIP traffic is unreachable even though the VIP falls inside the configured CIDR.

**Root cause:** `ansible.utils.in_any_network` can return `False` on Python 3.12 because an internal helper reads the removed private `ipaddress.IPv*Network._version` attribute and swallows the resulting exception.

**Fix:** Use the project-local `gitups_in_cidr` Ansible test plugin backed directly by Python's public `ipaddress` APIs.

