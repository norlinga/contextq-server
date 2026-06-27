# Security policy

## Supported versions

Until the first stable release, security fixes are made on the latest `v0.x`
release line and `main`. Older `v0.x` releases are not guaranteed to receive
backports.

## Reporting a vulnerability

Do not open a public issue for vulnerabilities involving authentication bypass,
API-key disclosure, command execution, filesystem isolation, the root bootstrap
workflow, or Caddy/systemd configuration.

Use GitHub's private vulnerability reporting for this repository:

<https://github.com/norlinga/contextq-server/security/advisories/new>

Include:

- the affected version or commit
- the deployment topology and operating system
- reproduction steps or a proof of concept
- the expected and observed security boundary
- any suggested mitigation

If private vulnerability reporting is unavailable, contact the maintainer through
the GitHub profile at <https://github.com/norlinga> without publishing exploit
details.

Reports will be acknowledged as availability permits. Confirmed issues will be
fixed on a private branch, assigned an advisory when appropriate, and released as a
new immutable version. Published tags will not be rewritten.

## Security boundary

The intended boundary is documented in [docs/security.md](docs/security.md). In
particular:

- Caddy terminates TLS; contextq-server binds to loopback.
- API keys grant broad access to exactly one namespace.
- Host, namespace, and key administration requires SSH access and is not public HTTP.
- The service account is unprivileged but is not a container or virtual-machine
  isolation boundary.
