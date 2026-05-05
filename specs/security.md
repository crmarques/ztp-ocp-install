# Security Spec

## Secret Handling

- Never commit plaintext credentials, kubeconfigs, pull secrets, private
  keys, or tokens. Generated examples must use placeholders only.
- Desired-state YAML references secret material by `SecretRef.name`. The
  local resolver maps the name to one file under `<gitups-home>/secrets`,
  where Gitups home is `GITUPS_HOME` or `~/.gitups` by default.
- The secrets directory must be host-local, unversioned, mode `0700`, and
  individual files mode `0600`.
- BMC credential files referenced by
  `InfrastructureProvider.spec.machine.libvirt.bmcEmulation.auth.credentialRef`
  and `ClusterInfrastructure.spec.machines.<name>.baremetal.bmc.credentialRef`
  are stored as a single `username:password` line. `gitups secrets credentials
  set` is the only supported writer; sushy emulator htpasswd files are
  derived from the credential file at apply time and are never committed.
- Generated self-signed certificate material is still secret material when
  it includes a private key. Gitups creates it through
  `gitups secrets generate`, not through root-escalated Ansible tasks, and
  must not render it into committed examples, logs, or GitOps output.
- Logs must not print sensitive values.

## OCP Install Trust Material

`Environment.spec.ocpInstall.disconnected` requires registry mirrors and
trust material defined inside the `disconnected` sub-block. The validator
rejects `disconnected` without both. Trust bundle material is referenced
by `registries.mirror.trustBundleRef.name`; bundle bytes are never
inlined into YAML. Lab environments may request generated self-signed
registry trust by declaring the same name in
`Environment.spec.keys[name].generated.selfSignedCertificate`, and Gitups
writes the certificate and private key only to the local secrets directory.
Disconnected mode requires OpenShift `imageDigestSources` for the mirrored
release payload sources. Gitups renders those sources with
`NeverContactSource`, rejects mirror entries outside the configured
registry, and uses a local release-image override. For QEMU/KVM lab
installs with emulated BMC, disconnected agent ISO rendering also uses
provider-hosted boot artifacts rather than public RHCOS boot-artifact URLs.

`Environment.spec.ocpInstall.connected` must remain empty. The validator
rejects proxy, mirror, or trust material declared under `connected` so no
mirror credentials, trust bundles, or `imageDigestSources` ever reach the
rendered install-config or host runtime.

When the operator declares `InfrastructureProvider.spec.registry.mirrorRegistry`,
the `provider_mirror_registry` role installs the registry CA into the host
trust store at `/etc/pki/ca-trust/source/anchors/gitups-mirror-<host>.crt`
and runs `update-ca-trust`. This trust write is host-local: cluster-node
trust still flows through the install-config `additionalTrustBundle` path.
The mirror htpasswd file is derived at apply time from a single-line
`username:password` secret named by `registries.mirror.credentialsRef` and
never inlined into committed YAML or rendered manifests.

## Host Runtime State

Keep root-managed runtime state separate. The default host runtime
directory is `/var/lib/gitups`. Do not place libvirt disks, BMC emulator
state, systemd unit assets, or rootful Podman bind-mounted config under
`~/.gitups`; secure home traversal, SELinux labels, and QEMU/libvirt
service-user access become harder and less predictable.

## Access Control

- Prefer least-privilege service accounts and scoped kubeconfigs.
- Separate bootstrap privileges from steady-state GitOps privileges.
- Document required permissions for each workflow.

## Supply Chain

- Pin external tools and container images.
- Search official upstream or vendor sources before adding or updating
  any external component.
- Select the latest stable release; do not use preview, nightly,
  development, or floating releases. Never use generic floating image
  tags such as `latest`.
- Verify downloaded artifacts where practical.
- Keep generated assets reproducible.
- Go module dependencies must come from trusted, widely-used upstreams
  and be pinned via `go.mod` and `go.sum`. Reject one-off forks,
  abandoned modules, and `replace` directives that point at a fork. The
  project-local `go-dependencies` skill encodes the working procedure.
