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
POST /_tiny/auth/logout
GET  /_tiny/auth/callback
```

In a deployed global-viewer-identity configuration, `GET /_tiny/auth/login`
creates a bounded, server-owned handoff and the platform identity broker is the
**only** browser viewer-session issuance route. Direct app-host
`POST /_tiny/auth/otp` and `POST /_tiny/auth/verify` are retired: form callers
receive the generic native retry page and JSON callers receive the normal safe
`not_authorized` envelope. They remain available only to an explicitly
brokerless local compatibility harness.

The platform-host broker owns:

```text
GET  /_tiny/identity?handoff=<opaque>
POST /_tiny/identity/otp
POST /_tiny/identity/verify
POST /_tiny/identity/use-another
POST /_tiny/identity/logout
```

Every platform broker POST requires an exact HTTPS same-origin `Origin` header
matching the configured platform host, an URL-encoded form body, and a bounded
body. Missing, malformed, or cross-origin requests return the same generic
native retry response without issuing OTP, changing a handoff, rotating an
identity, clearing a valid cookie, or serving app bytes. Anonymous app-host
handoff creation is independently rate-limited in bounded in-memory state; it
does not spend or bypass the email OTP budget.

The legacy/local-harness OTP request representation is:

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

In a brokerless local compatibility harness only, successful verification sets
a host-only app session cookie and redirects to a validated relative return
path. In a deployed broker, successful platform verification sets only the
platform identity cookie and redirects to an app-bound callback, which issues
the app-local child cookie after atomic policy/state checks.

For a document navigation with no app cookie, the app host uses a
server-created, state-bound handoff through the platform host. The browser
never supplies an app ID or callback host.
The app callback consumes an opaque one-time grant and state after current
policy rechecks; it is not an API, SDK, or app-content route.

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

GET  /auth/viewer
POST /auth/viewer/otp
POST /auth/viewer/verify
GET  /auth/viewer/handoff

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

- Global viewer identity: host-only platform cookie with a distinct name;
  email identity only, never a control credential.
- Control plane: host-only platform cookie with a distinct name.
- App plane: host-only app session cookie; never a parent-domain cookie.
- CLI/agent: bearer token, never accepted as an app viewer session.

Global identities and app sessions are opaque persisted credentials. One
browser profile has one global viewer identity; it may hold independent app
sessions for allowed apps. The global identity is exchanged only via a
five-minute-or-less, state-bound, one-time app handoff, and the current policy
is checked again on every protected app request. Sessions survive a normal
server restart because only hashes and lifecycle state persist. Revoking an app
session is local; global switch/revocation removes all child app sessions.

## Compatibility rule

Additive response fields are allowed within V1. Removing/renaming fields,
changing authorization meaning, or changing idempotency requires a versioned
contract decision.
