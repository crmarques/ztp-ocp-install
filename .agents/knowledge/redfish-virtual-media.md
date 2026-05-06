# Redfish virtual media quirks with sushy-tools

**Symptom:** Redfish boot fails with `Base.1.0.GeneralError`, connection refused during `InsertMedia`, HTTPS errors against the local emulator, or the VM boots only from an empty disk after media insertion.

**Root cause:** `sushy-tools` fetches the `InsertMedia` image by HTTP URL, exposes virtual media under the ComputerSystem resource, and rewrites the running libvirt CD-ROM block when media is inserted. The Ansible `community.general.redfish_*` helpers also force an `https://` base URI, which does not work with the local HTTP emulator.

**Fix:** Serve staged ISOs through the loopback vmedia HTTP unit, drive Redfish calls with `ansible.builtin.uri`, discover virtual media from the system entry, keep boot order at the libvirt OS level, and quote `ResetType: "On"` because YAML 1.1 treats bare `On` as a boolean.

