# Operations

## Service status and logs

```sh
ssh root@example.com systemctl status contextq-server.service
ssh root@example.com journalctl -u contextq-server.service -n 200
```

The service logs the namespace, key ID and label, command family, result, and
duration. It does not log bearer tokens.

Report local and installed build identities with:

```sh
contextq-server version
contextq-server doctor -t production
```

## Upgrade

Build or download the desired release artifacts, then rerun the idempotent bootstrap:

```sh
contextq-server remote-init -t production --apply
contextq-server doctor -t production
```

The installer selects binaries matching the remote architecture, replaces them
atomically, reloads the unit, restarts the service, validates Caddy, and preserves
namespace state and keys.

## Backup

All durable application state is beneath the configured data root, `/var/contextq`
by default. For a consistent offline backup:

```sh
ssh root@example.com 'systemctl stop contextq-server.service && \
  tar -C /var -czf /root/contextq-backup.tgz contextq && \
  systemctl start contextq-server.service'
```

Copy `/root/contextq-backup.tgz` off the VPS and protect it as a secret: it contains
queue data and API-key digests.

## Restore

Restoring replaces current queue state. Stop the service and retain the current tree
until the restored service passes diagnostics:

```sh
ssh root@example.com 'set -eu
systemctl stop contextq-server.service
mv /var/contextq /var/contextq.before-restore
tar -C /var -xzf /root/contextq-backup.tgz
chown -R contextq:contextq /var/contextq
chmod 750 /var/contextq
systemctl start contextq-server.service'

contextq-server doctor -t production
```

Delete `/var/contextq.before-restore` only after verifying the restored queues and
keys.

## Roll back binaries

Use the current local controller with the prior release's Linux artifacts:

```sh
contextq-server remote-init -t production --apply \
  --server-binary /path/to/previous/contextq-server \
  --contextq-binary /path/to/previous/contextq

contextq-server doctor -t production
```

Database migrations do not exist. Compatibility risk is limited to the filesystem
journal and CLI contract, but backups remain required before upgrades.

## Rotate an application key

Issue and save the replacement first:

```sh
contextq-server remote key add -t production --label replacement-laptop
contextq-server exec -t production queue list
```

Then list and revoke the previous immutable ID:

```sh
contextq-server remote key list -t production
contextq-server remote key revoke -t production k_previous
```

For external agents, use `--no-save`, update their secret manager, verify the new
credential, and revoke the old ID.

## Disable without deleting data

```sh
ssh root@example.com systemctl disable --now contextq-server.service
```

The queue journals and keys remain under `/var/contextq`. Re-enable with:

```sh
ssh root@example.com systemctl enable --now contextq-server.service
```

## Remove the service

Back up first. Then disable the unit and remove the Caddy snippet and installed
binaries. Do not delete `/var/contextq` until its contents are no longer needed.

```sh
ssh root@example.com 'set -eu
systemctl disable --now contextq-server.service
rm -f /etc/systemd/system/contextq-server.service
rm -f /etc/caddy/contextq.d/production.caddy
rm -f /usr/local/bin/contextq-server /usr/local/bin/contextq
systemctl daemon-reload
caddy validate --config /etc/caddy/Caddyfile
systemctl reload caddy'
```

The guarded wildcard import can remain in the main Caddyfile for future
contextq-server sites.
