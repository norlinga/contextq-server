# Feature set and scope

contextq-server is deliberately a transport and operations layer around contextq,
not a new queue engine.

## Remote command execution

- One versioned command endpoint: `POST /v1/{namespace}/exec`
- Queue and item subcommands are allowlisted.
- `--json` and the absolute namespace `--root` are injected by the server.
- `--root`, `--config`, JSON-mode overrides, help, and version flags are rejected
  from remote arguments.
- Commands run through `exec.CommandContext`, never through a shell.
- The subprocess working directory is the authenticated namespace directory.

## Namespace isolation

- Namespace names are validated before filesystem use.
- Each namespace has independent queue journals and API keys.
- Authentication failures do not reveal whether a namespace exists.
- A key from one namespace cannot authenticate to another.

Namespace isolation is an application boundary, not a container boundary. The
systemd service account and filesystem permissions provide the operating-system
boundary.

## Authentication

- 256-bit random bearer-key secrets
- immutable key IDs
- required, case-insensitively unique labels
- SHA-256 digests stored remotely
- constant-time digest comparison
- multiple keys per namespace
- immediate revocation without caches or restarts
- broad namespace access rather than per-command scopes

TLS terminates at Caddy. contextq-server binds to loopback by default and the
bootstrap refuses public listen addresses.

## Concurrency and failure bounds

- Contextq's per-queue filesystem lock remains authoritative.
- `item pop` retains contextq's atomic FIFO claim behavior.
- Unrelated requests can execute concurrently.
- The server bounds simultaneous subprocesses.
- Request bodies, stdout, stderr, and execution duration are bounded.
- Request cancellation kills the subprocess and releases operating-system locks.

## Provisioning and operations

- Local named targets with a selectable default
- Cross-compiled Linux release bundles for amd64 and arm64
- Remote architecture detection and ELF validation
- SCP binary upload over the configured SSH identity
- Temporary OpenSSH multiplexing for one authentication prompt per multi-step command
- Idempotent service user, directory, systemd, and Caddy convergence
- Atomic binary and configuration-file replacement where practical
- Labeled initial key issuance and local storage
- Read-only diagnostics through `doctor`

## Contextq behavior preserved

- required agent-facing queue context
- FIFO availability and atomic claims
- distinct lifecycle ID for every enqueue
- states: `AVAILABLE`, `CLAIMED`, `DONE`, `FAILED`, `CANCELED`
- duplicate available-key protection
- append-only event history
- unique-name-or-UUID queue references
- stable JSON error codes

Refer to the [contextq repository](https://github.com/norlinga/contextq) for the
canonical state machine and CLI contract.

## Deliberately absent

- database or alternate server-side state store
- daemon-specific queue implementation
- long polling or WebSockets
- leases, heartbeats, or visibility timeouts
- delayed scheduling
- arbitrary remote shell execution
- public namespace or key administration API
- per-key command scopes
- multi-host consensus or failover
- web dashboard

These omissions keep the service inspectable and small. They should be added only
when a concrete coordination workflow requires them.
