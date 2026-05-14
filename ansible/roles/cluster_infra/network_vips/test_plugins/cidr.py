
from __future__ import annotations

import ipaddress


def bootwright_in_cidr(ip, cidr):
    try:
        return ipaddress.ip_address(ip) in ipaddress.ip_network(cidr, strict=False)
    except (TypeError, ValueError):
        return False


class TestModule:
    def tests(self):
        return {"bootwright_in_cidr": bootwright_in_cidr}
