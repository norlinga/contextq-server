# Installing contextq-server on a VPS

This guide installs contextq-server behind an existing Caddy service on a Linux VPS.
The bootstrap expects systemd, Caddy, root SSH access, and a DNS record already
pointing at the VPS.

## Resulting host layout

The default installation creates:

```text
/usr/local/bin/contextq
/usr/local/bin/contextq-server
/var/contextq/
/etc/systemd/system/contextq-server.service
/etc/caddy/contextq.d/<target>.caddy
```

It also adds this stable import to `/etc/caddy/Caddyfile` when absent:

```caddyfile
import /etc/caddy/contextq.d/*.caddy
```

The service runs as an unprivileged `contextq` user, listens on
`127.0.0.1:8787`, and can write only beneath `/var/contextq` under its systemd
sandbox.

## 1. Prepare DNS

Create an A, AAAA, or CNAME record for the public queue host. For example:

```text
q.example.com -> example.com
```

Caddy must be able to resolve the name publicly to obtain a TLS certificate.

## 2. Build the controller and deployment binaries

Keep the two source repositories adjacent:

```text
work/
  contextq/
  contextq-server/
```

Confirm the VPS architecture:

```sh
ssh root@example.com uname -m
```

Use `amd64` for `x86_64` and `arm64` for `aarch64`:

```sh
cd contextq-server
make release TARGET_GOARCH=amd64
```

The release layout separates the executable for the local machine from the Linux
artifacts intended for the VPS:

```text
dist/contextq-server
dist/linux-amd64/contextq-server
dist/linux-amd64/contextq
dist/contextq-linux-amd64.tar.gz
dist/SHA256SUMS
```

`remote-init` also asks the VPS for `uname -m` and refuses to upload a mismatched
ELF binary.

## 3. Add a local target

```sh
dist/contextq-server target add \
  --url https://q.example.com \
  --namespace personal \
  --ssh-host example.com \
  --ssh-user root \
  --use production
```

When a specific SSH key is required, add its absolute path:

```sh
--identity "$HOME/.ssh/id_ed25519"
```

Inspect the saved target without displaying its secret:

```sh
dist/contextq-server target list
```

## 4. Review the bootstrap

Without `--apply`, `remote-init` prints the deterministic POSIX shell script:

```sh
dist/contextq-server remote-init -t production \
  > /tmp/contextq-remote-init.sh

sh -n /tmp/contextq-remote-init.sh
sed -n '1,260p' /tmp/contextq-remote-init.sh
```

The generated script:

1. validates root privileges and required host commands
2. creates the restricted service identity and data root
3. installs both binaries atomically
4. writes the systemd unit atomically
5. writes the Caddy snippet and one guarded import
6. restarts the application service
7. validates and reloads Caddy

## 5. Apply the bootstrap

```sh
dist/contextq-server remote-init -t production --apply \
  --label my-laptop
```

The apply workflow uploads both binaries, executes the reviewed host script,
initializes the configured namespace as the service user, generates a labeled API
key, and stores that key in the local target file.

Multi-step commands use a temporary OpenSSH control socket. Password authentication
should prompt on the first connection only; subsequent SSH and SCP processes reuse
that connection. The socket is closed and removed when the command finishes.
SSH-key authentication remains preferable for unattended upgrades.

Re-running `remote-init --apply` upgrades the binaries and reconciles the same
configuration. It does not issue another API key when the target already has one.

## 6. Verify the installation

```sh
dist/contextq-server doctor -t production
```

The doctor checks:

- the public HTTPS health endpoint
- SSH connectivity
- `contextq-server.service`
- installed binaries and data root
- Caddyfile validity
- authenticated access to the configured namespace

Individual checks can also be run manually:

```sh
curl -sS https://q.example.com/healthz

ssh root@example.com \
  systemctl status contextq-server.service

ssh root@example.com \
  cat /etc/caddy/contextq.d/production.caddy

ssh root@example.com \
  caddy validate --config /etc/caddy/Caddyfile
```

Caddy may require a short period to obtain the first certificate. Rerun `doctor`
after certificate provisioning if HTTPS is the only initial failure.

## 7. Make the first request

```sh
dist/contextq-server exec -t production queue list
```

An unused namespace returns:

```json
[]
```

## Custom paths

The target file supports custom remote binary, data, Caddyfile, snippet, SSH port,
and service-user values. The initial CLI intentionally exposes the common target
fields; advanced values can be edited carefully in `~/.contextq-server`. Run
`doctor` after any manual change.
