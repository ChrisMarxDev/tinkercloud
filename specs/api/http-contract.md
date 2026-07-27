# HTTP Contract Sketch

This is intentionally technology-neutral and incomplete enough to evolve before
implementation. All JSON endpoints use versioned `/v1` routes on the platform
host or reserved `/_tiny/api/v1` routes on an app host.

## Common rules

- Request/response IDs use `X-Request-ID`; untrusted values are validated.
- JSON errors use a stable code and safe message:

```json
{
  "error": {
    "code": "not_authorized",
    "message": "This request is not authorized.",
    "request_id": "req_..."
  }
}
```

- No error includes filesystem paths, SQL, policy membership, secret material,
  or stack traces.
- Mutating control-plane requests accept an idempotency key.
- Body, header, duration, and response limits are explicit per endpoint.
- Browser SDKs send `X-Tiny-SDK-Version: <semver>`. Omission remains compatible
  for raw HTTP and already shipped clients. A supplied malformed or unsupported
  major returns `426 sdk_version_incompatible` with a safe request ID and an
  actionable upgrade message after the normal app authorization boundary.

## App host: pre-authentication

```text
GET  /_tiny/auth/login
POST /_tiny/auth/otp
POST /_tiny/auth/verify
POST /_tiny/auth/logout
```

OTP request:

```json
{ "email": "alice@example.com" }
```

Always returns a generic accepted representation if syntactically valid.

OTP verify:

```json
{
  "transaction": "login_...",
  "code": "483921"
}
```

Success sets a host-only secure session cookie and redirects to a validated
relative return path.

## App host: protected capabilities

```text
GET    /_tiny/api/v1/me
GET    /_tiny/api/v1/app
GET    /_tiny/api/v1/capabilities
GET    /_tiny/api/v1/kv/{key}
PUT    /_tiny/api/v1/kv/{key}
DELETE /_tiny/api/v1/kv/{key}
GET    /_tiny/api/v1/kv?prefix=&limit=&cursor=
POST   /_tiny/api/v1/blobs
GET    /_tiny/api/v1/blobs?limit=&cursor=
GET    /_tiny/api/v1/blobs/{id}
DELETE /_tiny/api/v1/blobs/{id}
GET    /_tiny/ws/v1
```

Current viewer:

```json
{
  "identity": {
    "id": "idn_...",
    "email": "alice@example.com"
  },
  "app": {
    "slug": "invoice-review"
  }
}
```

Current app:

```json
{
  "slug": "invoice-review",
  "features": { "kv": true, "blobs": true, "realtime": true }
}
```

KV write:

```json
{
  "value": { "approved": true },
  "expected_version": 3
}
```

Response:

```json
{
  "key": "reviews/invoice-42",
  "value": { "approved": true },
  "version": 4,
  "updated_at": "2026-07-24T12:00:00Z"
}
```

Conflicting version returns `409 version_conflict`.

KV list response:

```json
{
  "entries": [],
  "next_cursor": ""
}
```

`entries` is always a JSON array, including an empty page. `next_cursor` is
always present and is the empty string when there is no following page.

Blob upload is `multipart/form-data` with exactly one bounded `file` part and
no extra fields or parts. The server issues the blob ID; the submitted filename
and content type are display metadata only.

Blob metadata response:

```json
{
  "id": "blb_...",
  "name": "invoice.pdf",
  "size": 48213,
  "content_type": "application/pdf",
  "created_at": "2026-07-27T12:00:00Z"
}
```

Blob list returns `{ "blobs": [], "next_cursor": "" }`; `blobs` is always an
array. Blob download returns the recorded bytes only after ordinary app
authorization and ready-state validation, with `Content-Disposition:
attachment`, `X-Content-Type-Options: nosniff`, and private/no-store caching.
It never redirects to a local/provider URL. Missing or unavailable IDs use the
generic bounded not-found/unavailable envelope and disclose no cross-app
existence.

Capability discovery:

```json
{
  "capabilities": [
    {
      "name": "user",
      "version": 1
    },
    {
      "name": "kv",
      "version": 1,
      "limits": {
        "value_bytes": 65536,
        "keys_per_app": 10000
      }
    },
    {
      "name": "blobs",
      "version": 1,
      "limits": {
        "blob_bytes": 25000000,
        "blobs_per_app": 1000,
        "total_bytes_per_app": 250000000,
        "list_limit": 100
      }
    },
    {
      "name": "live",
      "version": 1,
      "limits": {
        "connections_per_app": 100,
        "subscriptions_per_connection": 32
      }
    }
  ]
}
```

The response describes the current app's effective grants. It contains no
provider connection IDs or secret-bearing configuration.

### WebSocket protocol

`GET /_tiny/ws/v1` authenticates and authorizes before upgrade. The server
binds the connection to its derived app, identity, and session; the client
cannot select any of them. Frames use a versioned JSON envelope:

```json
{ "v": 1, "type": "subscribe", "channel": "reviews" }
```

Supported V1 operations are subscribe/unsubscribe, bounded publish to a custom
app channel, and subscribe to KV changes by prefix. Policy, session, or app
revocation closes affected connections. Connections, subscriptions, frame
sizes, publish rates, and outbound queues are limited. Delivery is best-effort:
there is no persistence, history, replay, ordering guarantee, or resume cursor.
After reconnect, clients reread current KV state.

## Platform host: authentication and control

```text
POST /api/v1/auth/otp
POST /api/v1/auth/verify
GET  /api/v1/whoami

GET  /api/v1/apps
POST /api/v1/apps
GET  /api/v1/apps/{slug}
POST /api/v1/apps/{slug}/deployments
GET  /api/v1/apps/{slug}/deployments/{id}
POST /api/v1/apps/{slug}/deployments/{id}/activate
POST /api/v1/apps/{slug}/rollbacks
GET  /api/v1/apps/{slug}/access
PUT  /api/v1/apps/{slug}/access
```

The CLI may combine create/upload/activate into `tiny deploy`, but the server
states remain observable. An async deployment response returns IDs and a status
URL; “ready” is returned only after security probes and TLS readiness.

## Cookie split

- Control plane: host-only platform cookie with a distinct name.
- App plane: host-only app session cookie; never a parent-domain cookie.
- CLI/agent: bearer token, never accepted as an app viewer session.

App sessions are independent opaque credentials: one viewer may hold multiple
simultaneous sessions for the same app and sessions for different apps. They
survive a normal server restart because only their hashes and lifecycle state
are persisted. Revoking one app session denies that credential on the next
request without revoking the viewer's other app sessions; wrong-app and expired
credentials deny.

## Compatibility rule

Additive response fields are allowed within V1. Removing/renaming fields,
changing authorization meaning, or changing idempotency requires a versioned
contract decision.
