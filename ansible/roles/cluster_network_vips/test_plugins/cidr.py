
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
