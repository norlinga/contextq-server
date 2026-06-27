# Configuring targets, namespaces, keys, and queues

contextq-server has two configuration layers:

1. a local target tells the controller how to reach one namespace
2. the remote namespace contains labeled API keys and contextq queue journals

## Local targets

Targets are stored in `~/.contextq-server` by default. Override the path with:

```sh
export CONTEXTQ_SERVER_CONFIG=/path/to/targets.json
```

The file is JSON and is written with mode `0600` because it contains plaintext API
keys. A representative target is:

```json
{
  "version": 1,
  "default": "production",
  "targets": {
    "production": {
      "url": "https://q.example.com",
      "namespace": "personal",
      "key": "cqk_k_..._<secret>",
      "ssh_host": "example.com",
      "ssh_user": "root",
      "remote_bin": "/usr/local/bin/contextq-server",
      "contextq_bin": "/usr/local/bin/contextq",
      "data_root": "/var/contextq",
      "caddyfile": "/etc/caddy/Caddyfile",
      "snippet_dir": "/etc/caddy/contextq.d",
      "service_user": "contextq"
    }
  }
}
```

Manage targets without exposing stored keys:

```sh
contextq-server target list
contextq-server target use production
contextq-server target remove production
```

Removing a local target does not revoke its remote key. Revoke the key first when
removing access permanently.

## Namespaces

A namespace is an isolation boundary for queues and keys. The default layout for
namespace `personal` is:

```text
/var/contextq/personal/
  namespace.json
  keys.json
  contextq/
    queues/
```

Every request executes contextq with:

```text
working directory: /var/contextq/personal
--root:            /var/contextq/personal/contextq
```

Client arguments cannot override `--root`, `--config`, or `--json`.

Initialize the namespace configured on a target:

```sh
contextq-server remote namespace init -t production
```

Initialization is idempotent.

## Application keys

Keys grant all queue and item commands within one namespace. They have no per-command
scopes in the current version.

Issue a key for the current machine and save it into the selected target:

```sh
contextq-server remote key add -t production \
  --label my-laptop
```

Issue a key for another agent without replacing the locally saved key:

```sh
contextq-server remote key add -t production \
  --label github-actions \
  --no-save
```

The token is printed once. Transfer it to that agent through an appropriate secret
manager.

List the immutable IDs and human-readable labels:

```sh
contextq-server remote key list -t production
```

Revoke by immutable ID:

```sh
contextq-server remote key revoke -t production k_4d172bc156d1
```

The remote `keys.json` stores only SHA-256 digests and is mode `0600`. Authentication
rereads it on every request, making revocation effective immediately without a
service restart.

## Queues

Queues are created through the ordinary contextq CLI vocabulary. The required queue
context is guidance for every agent consuming that queue:

```sh
contextq-server exec -t production queue create \
  "Read the queue before every claim. Run tests and update claimed work promptly." \
  --name maintenance
```

Read the queue and its current counts:

```sh
contextq-server exec -t production queue read maintenance
```

Add stable work keys:

```sh
contextq-server exec -t production item push maintenance issue-123
contextq-server exec -t production item push maintenance internal/store/store.go
```

Claim work only through `item pop`:

```sh
contextq-server exec -t production item pop maintenance
```

Complete the exact key returned by `pop`:

```sh
contextq-server exec -t production \
  item update maintenance issue-123 DONE
```

Terminal outcomes are `DONE`, `FAILED`, and `CANCELED`. Retrying terminal work means
pushing the key again, creating a new lifecycle at the back of the queue.

Inspect state and history:

```sh
contextq-server exec -t production item list maintenance
contextq-server exec -t production item read maintenance issue-123
contextq-server exec -t production item history maintenance issue-123
```

`item list` and `item read` are observational. They do not reserve work. The
canonical agent workflow and lifecycle specification are maintained by
[contextq](https://github.com/norlinga/contextq).

## Looping agents

The server intentionally returns `no_available_items` immediately instead of holding
the HTTP request open. A looping client should:

1. read the queue context
2. call `item pop`
3. work on the returned key
4. update it to a terminal state
5. use bounded backoff when no item is available

There are no leases or visibility timeouts. A claimed item remains claimed until a
client updates it.
