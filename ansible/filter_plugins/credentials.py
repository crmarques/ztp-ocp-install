
from __future__ import annotations

import base64

from ansible.errors import AnsibleFilterError


def gitups_parse_credential(slurp_result, label="credential"):
    if not isinstance(slurp_result, dict):
        raise AnsibleFilterError(
            f"{label}: expected an ansible.builtin.slurp result mapping, "
            f"got {type(slurp_result).__name__}"
        )
    content = slurp_result.get("content")
    if content is None:
        raise AnsibleFilterError(f"{label}: slurp result missing 'content' field")
    try:
        raw = base64.b64decode(content).decode("utf-8").strip()
    except (ValueError, UnicodeDecodeError) as err:
        raise AnsibleFilterError(f"{label}: base64/utf-8 decode failed: {err}")
    if ":" not in raw:
        raise AnsibleFilterError(
            f"{label}: must be a single username:password line"
        )
    username, password = raw.split(":", 1)
    if not username or not password:
        raise AnsibleFilterError(
            f"{label}: username and password must both be non-empty"
        )
    return {"username": username, "password": password}


class FilterModule:
    def filters(self):
        return {"gitups_parse_credential": gitups_parse_credential}
