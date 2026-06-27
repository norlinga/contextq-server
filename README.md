# contextq-server

[![CI](https://github.com/norlinga/contextq-server/actions/workflows/ci.yml/badge.svg)](https://github.com/norlinga/contextq-server/actions/workflows/ci.yml)
[![CodeQL](https://github.com/norlinga/contextq-server/actions/workflows/codeql.yml/badge.svg)](https://github.com/norlinga/contextq-server/actions/workflows/codeql.yml)

`contextq-server` makes local [contextq](https://github.com/norlinga/contextq)
queues available to agents and automation running on other machines.

The underlying queue remains ordinary contextq: append-only JSONL journals,
filesystem locks, FIFO claims, explicit lifecycle states, and files an operator can
inspect directly. This project adds only the network and operational boundary:

```text
agent or script
      │ HTTPS + namespace key
      ▼
    Caddy
      │ loopback reverse proxy
      ▼
contextq-server
      │ validated exec.CommandContext
      ▼
 contextq CLI ── filesystem journals
```

There is no database, daemon protocol, scheduler, worker framework, or public
administration API.

> **Status:** early `v0` software. The command and deployment contracts may change
> before `v1.0.0`. Back up `/var/contextq` before upgrades.

## Why it exists

Local contextq queues coordinate agents sharing one filesystem. A small server makes
the same model useful when agents run:

- on several development machines
- in CI or scheduled loops
- on temporary workers
- outside the machine holding the queue journals

Each API key grants broad access to one namespace. Multiple labeled keys make it
straightforward to add an agent and later identify exactly which credential to
revoke.

## What it provides

- namespace-isolated contextq roots
- labeled, revocable bearer keys stored as SHA-256 digests
- one command RPC: `POST /v1/{namespace}/exec`
- a local client preserving contextq's queue and item command vocabulary
- idempotent Linux, systemd, and Caddy bootstrap over SSH
- automatic remote architecture detection and binary selection
- one-password SSH connection reuse for multi-step operations
- health and configuration diagnostics through `doctor`
- static, stripped Linux release bundles

See [Features](docs/features.md) for the complete boundary and deliberate omissions.

## Quick start

Download the local controller and matching Linux server bundle from the
[latest release](https://github.com/norlinga/contextq-server/releases/latest), or
build them from source:

```sh
# Build a local controller and the Linux deployment bundle.
make release TARGET_GOARCH=amd64

# Save the public origin, namespace, and SSH destination locally.
dist/contextq-server target add \
  --url https://q.example.com \
  --namespace personal \
  --ssh-host example.com \
  --ssh-user root \
  --use production

# Upload both binaries and converge the user, systemd, Caddy, namespace, and key.
dist/contextq-server remote-init -t production --apply \
  --label my-laptop

# Verify HTTPS, SSH, systemd, files, Caddy, and authenticated RPC access.
dist/contextq-server doctor -t production

# Run contextq commands remotely.
dist/contextq-server exec -t production queue list
```

The local target file is `~/.contextq-server` and is mode `0600` because it contains
the API key.

Release builds fetch the contextq version pinned in `CONTEXTQ_VERSION`; they do not
depend on an arbitrary sibling checkout.

Inspect build identity at any time:

```sh
contextq-server version
```

## Example workflow

```sh
contextq-server exec -t production queue create \
  "Claim work only with item pop. Update every claimed item." \
  --name release

contextq-server exec -t production item push release issue-123
contextq-server exec -t production item pop release
contextq-server exec -t production item update release issue-123 DONE
```

All queue behavior is defined by contextq itself. The server injects `--json` and an
absolute namespace `--root`, starts contextq inside that namespace directory, and
returns its JSON result.

## Documentation

- [Installing on a VPS](docs/installation.md)
- [Targets, namespaces, keys, and queues](docs/configuration.md)
- [Feature set and scope](docs/features.md)
- [HTTP API](docs/api.md)
- [Operations, backup, upgrade, and removal](docs/operations.md)
- [Security model](docs/security.md)
- [Security reporting policy](SECURITY.md)
- [Release process](docs/releasing.md)
- [Changelog](CHANGELOG.md)

The canonical contextq lifecycle and CLI specification remain in the
[contextq repository](https://github.com/norlinga/contextq). Agents should follow
contextq's rule that work is claimed only through `item pop`.

## Release size

`make release` uses a static, stripped, trimpath build with inlining disabled for
size. On Linux/amd64 during development:

| Artifact | Approximate size |
| --- | ---: |
| `contextq-server` | 6.8 MB |
| `contextq` | 3.2 MB |
| compressed two-binary bundle | 3.9 MB |

UPX is not used; the deployed programs remain conventional static Go executables.

## Development

```sh
make check
make build
make release
```

Tests cover namespace and key behavior, command validation, HTTP behavior,
concurrent key changes, generated shell syntax, SSH command construction, race
detection, and integration with the real contextq binary.

## License

MIT. See [LICENSE](LICENSE). Binary bundles also contain the pinned contextq license
and notices for its linked dependencies.
