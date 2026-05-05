# Jinja test plugin for CIDR membership.
#
# ansible.utils.in_any_network silently returns False on Python >= 3.12
# because its _is_subnet_of helper reads ipaddress.IPv*Network._version,
# which the stdlib has dropped — the AttributeError is swallowed by a
# broad try/except and every check looks like a non-match. The
# cluster_network_vips role hits this when pairing managed load balancer
# VIPs to libvirt bridges, so it never plumbs the VIP and the API stays
# unreachable.
#
# This plugin uses ipaddress directly with stable, public attributes.

from __future__ import annotations

import ipaddress


def gitups_in_cidr(ip, cidr):
    try:
        return ipaddress.ip_address(ip) in ipaddress.ip_network(cidr, strict=False)
    except (TypeError, ValueError):
        return False


class TestModule:
    def tests(self):
        return {"gitups_in_cidr": gitups_in_cidr}
