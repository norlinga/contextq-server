# Security model

contextq-server exposes a local filesystem queue to authenticated clients. Its
security model is intentionally small, but the service still crosses three important
boundaries: public HTTPS, local process execution, and root-assisted installation.

## Trust boundaries

```text
untrusted network
      │ HTTPS
      ▼
    Caddy                 trusted TLS boundary
      │ loopback HTTP
      ▼
contextq-server           namespace authentication and argument validation
      │ child process
      ▼
  contextq CLI            queue semantics and per-queue filesystem locks
      │
      ▼
/var/contextq             service-user-owned durable state
```

## Application keys

- A key belongs to one namespace and grants every exposed queue and item command in
  that namespace.
- Secrets contain 256 bits of randomness.
- The server stores only SHA-256 digests and compares them in constant time.
- Key IDs and required labels support human-auditable revocation.
- The local target file contains the plaintext key and is written with mode `0600`.
- The server rereads the namespace key file for every request, so revocation does not
  depend on cache expiry or process restart.

Keys are bearer credentials. Anyone possessing one has its full namespace access.
Transfer keys through a secret manager and never commit `~/.contextq-server`.

## Remote command boundary

The public API does not accept shell text. It accepts a JSON array of contextq CLI
arguments and invokes `exec.CommandContext` directly.

The server:

- allowlists current queue and item command families
- injects `--json` and a server-controlled absolute `--root`
- rejects client root, config, JSON-mode, help, and version overrides
- validates namespace names before filesystem use
- sets the subprocess working directory to the namespace directory
- runs under bounded time, concurrency, input, and output limits

contextq remains responsible for state transitions, FIFO claims, duplicate-key
protection, journals, and filesystem locking.

## Host administration

Host bootstrap and key administration use the target's SSH administrator account,
which defaults to root. The service user itself has `/usr/sbin/nologin` and receives
no SSH key or `authorized_keys` file.

The bootstrap runs as root because it installs binaries, a systemd unit, and a Caddy
snippet. The generated script is deterministic and can be reviewed without
`--apply`. Binaries and configuration files are replaced atomically where practical.

OpenSSH multiplexing reduces repeated authentication prompts but does not broaden
remote privileges. Its temporary control socket is stored in a mode-`0700` temporary
directory and removed when the command exits.

## Service sandbox

The generated systemd unit uses:

- a dedicated `contextq` user and group
- `NoNewPrivileges=true`
- `PrivateTmp=true`
- `ProtectHome=true`
- `ProtectSystem=strict`
- `ReadWritePaths=/var/contextq`
- a restrictive process umask

Namespace separation is an application and filesystem organization boundary. It is
not intended to resist a complete compromise of the shared service process or its
operating-system account.

## Deliberate limitations

- No per-command or read-only key scopes.
- No rate limiter in the application. High-entropy credentials are the primary
  authentication defense; network controls may be added at Caddy when needed.
- No lease expiry for claimed queue items.
- No encrypted-at-rest queue journal.
- No multi-host replication or consensus.
- No public administration API.

Deployments with stronger tenant isolation requirements should run separate service
instances under separate operating-system accounts or hosts.
