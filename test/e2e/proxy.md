# Proxy Options

Shared reference for how the e2e cases handle outbound egress. Cases link
here when they reach the "optional proxy" step. Pick one of the four modes
below and edit `environment.yaml` / `provider.yaml` accordingly before
running `gitups secret generate`.

The cases' reference yamls ship the **managed Squid** layout (so the
files exercise the richest path). To use any other mode, prune the
`proxy:` blocks shown below from those files.

## Mode 1 — Direct (No Proxy)

Cluster and bastion reach the internet directly.

- `environment.yaml`: remove the entire `spec.proxy:` block. Remove the
  `proxy-credentials` entry from `spec.secrets:`.
- `provider.yaml`: remove the `spec.proxy:` block. Drop `proxy` from the
  host's `capabilities:` list.

No further secrets to materialize for proxy.

## Mode 2 — External Proxy, Unauthenticated

Bastion and cluster route through a forward proxy that does not require
credentials.

- `environment.yaml`:

  ```yaml
  spec:
    proxy:
      http: http://proxy.example.test:3128
      https: http://proxy.example.test:3128
      noProxy:
        - <case primary network CIDR>
  ```

  Drop the `auth:` sub-block and the `proxy-credentials` secret entry.

- `provider.yaml`: remove the `spec.proxy:` block. Drop `proxy` from the
  host's `capabilities:` list.

Gitups auto-extends `noProxy` with cluster-local endpoints
(service/cluster CIDRs, `.svc`, `.cluster.local`, `localhost`, the base
domain, mirror registry host, provider host addresses); only
user-specific entries need to be listed.

## Mode 3 — External Proxy, Authenticated

Same as Mode 2 plus credentials. Pick one of the two secret forms.

**File-backed** — you write the credentials yourself:

```yaml
# environment.yaml
spec:
  proxy:
    http: http://proxy.example.test:3128
    https: http://proxy.example.test:3128
    noProxy:
      - <case primary network CIDR>
    auth:
      proxyAuthRef:
        name: proxy-credentials
  secrets:
    proxy-credentials:
      file: ~/.gitups/secrets/proxy-credentials
```

Then on the bastion:

```bash
gitups secret set proxy-credentials \
  --username <proxy-user> \
  --password-stdin \
  --secrets-dir "$GITUPS_SECRETS_DIR"
```

**Generated** — `gitups secret generate` materializes the file. Password
is auto-generated; username defaults to `admin` when omitted:

```yaml
# environment.yaml
spec:
  secrets:
    proxy-credentials:
      generated:
        credentials:
          username: proxy
```

No `gitups secret set` step — `gitups secret generate -f "$WORKSPACE"`
covers it.

In both forms `provider.yaml` keeps no `spec.proxy:` block and the host
keeps no `proxy` capability.

## Mode 4 — Gitups-Managed Squid On The Provider Host

Gitups stands up an authenticated Squid container on the provider host
itself.

`environment.yaml` carries the same `proxy:` block as Mode 3 (auth is
mandatory; managed Squid is always authenticated):

```yaml
spec:
  proxy:
    http: http://proxy.example.test:3128
    https: http://proxy.example.test:3128
    noProxy:
      - <case primary network CIDR>
    auth:
      proxyAuthRef:
        name: proxy-credentials
```

`http`/`https` must be bare `http://` URLs whose port matches
`spec.proxy.squid.port`.

`provider.yaml` declares Squid on the provider host:

```yaml
spec:
  hosts:
    lab-host:
      capabilities:
        - libvirt
        - container-runtime
        - hosts-file
        - proxy
  proxy:
    squid:
      hostRef:
        name: lab-host
      # port: 3128       # default
      # runtime: podman  # default
```

Constraints:

- The referenced host must carry the `proxy` capability and an `ssh`
  block.
- The libvirt machine `hostRef` must match `spec.proxy.squid.hostRef` — v1
  keeps proxy and VMs on the same host so isolated networks stay
  reachable.
- The primary machine network in `infra.yaml` must declare a `gateway` so
  VMs route egress through the managed proxy.

### Two Client URLs

Managed Squid is reached at different addresses by the host and by the
VMs. Gitups renders both:

- **Host URL** — `http://<provider-host SSH address>:<port>`. Written by
  `host_proxy` into `/etc/dnf/dnf.conf`, `/etc/environment`, the systemd
  drop-in, and `pip.conf` on every host. Routable before libvirt is
  installed (solves the bootstrap chicken-and-egg).
- **VM URL** — `http://<machineNetwork.gateway>:<port>`. Embedded in
  `install-config.yaml` so the OpenShift cluster sends runtime egress
  through the libvirt-bridge gateway, where Squid (bound via host
  networking) answers.

For external proxies the two URLs collapse to the user-configured
`spec.proxy.http` / `spec.proxy.https`.

## Bootstrap Order Note

`gitups apply bastion -f "$WORKSPACE"` runs *before* `gitups apply infra`
provisions the managed Squid container, so the bastion phase deliberately
ignores `environment.yaml`'s `spec.proxy` for Mode 4. Once `apply infra`
finishes, every later phase (provider-host package/image pulls,
install-config rendering, agent install) routes through Squid.

Modes 2 and 3 do not have this exception — the bastion picks up the
external proxy URL from the workspace immediately.

## Bastion-Build Proxy

The no-state `gitups apply bastion` and (for the containerized bastion)
the `podman build` reach the internet *before* a workspace exists. Both
read ambient `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` env vars. Set
these on the host (or in the bastion shell) when an external proxy is
required to reach base packages and registry content. They have no
effect once `gitups apply bastion -f "$WORKSPACE"` runs — that phase
strips them and uses `Environment.spec.proxy` instead.
