# HTTP API

The HTTP surface is intentionally limited to health and one command RPC.

## Health

```http
GET /healthz
```

Successful response:

```json
{"status":"ok"}
```

This endpoint does not require authentication and verifies only that the HTTP process
is serving requests. Use `contextq-server doctor` for end-to-end diagnostics.

## Execute a contextq command

```http
POST /v1/{namespace}/exec
Authorization: Bearer <application-key>
Content-Type: application/json
```

Request body:

```json
{
  "args": ["item", "pop", "release"]
}
```

The exposed command families are:

```text
queue create|list|read|destroy
item  push|pop|list|read|update|history
```

The body is limited to 64 KiB and 64 arguments. Server-controlled contextq flags
cannot be overridden.

### Example

```sh
KEY="$(jq -r '.targets.production.key' "$HOME/.contextq-server")"

curl -sS https://q.example.com/v1/personal/exec \
  -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' \
  --data '{"args":["queue","list"]}'
```

Successful responses are the JSON written by `contextq --json`, not an additional
server envelope.

## Status codes

| Status | Meaning |
| --- | --- |
| `200` | Contextq command succeeded |
| `400` | Invalid request, protected argument, or ordinary CLI validation error |
| `401` | Missing or invalid namespace key |
| `404` | Authenticated request referenced a missing queue |
| `409` | Contextq coordination conflict such as no available item or invalid transition |
| `502` | Contextq could not execute or returned invalid/oversized output |
| `504` | Contextq command exceeded its request timeout |

Expected contextq failures preserve its machine-readable shape:

```json
{
  "code": "no_available_items",
  "error": "no available items"
}
```

Common error codes include:

- `queue_not_found`
- `queue_name_ambiguous`
- `duplicate_available_key`
- `no_available_items`
- `invalid_state_transition`
- `no_active_lifecycle`
- `force_required`

## Authentication model

The server hashes the complete bearer token and compares it against the digests in
the requested namespace. The key grants access only to that namespace but permits
all exposed queue and item commands there.

Redirects are not followed by the local HTTP client, preventing credentials from
being forwarded to an unexpected origin.
