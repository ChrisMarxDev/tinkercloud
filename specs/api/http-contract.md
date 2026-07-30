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
- SDK HTTP calls also send `X-Tiny-App-API-Version: 1`; live sockets offer
  `tiny.sdk.<semver>.api.1` as their single subprotocol. A supplied malformed or
  unsupported value is denied before capability dispatch or live hub attach.
- Released workstation clients send `X-Tiny-CLI-Version: <semver>` and
  `X-Tiny-Control-API-Version: 1` on protected control requests. Missing
  headers retain the V1 migration allowance; a supplied incomplete or
  unsupported pair receives `426 cli_version_incompatible`.
- `GET /api/v1/compatibility` returns only the static build compatibility
  matrix with no-store/nosniff headers. It is unauthenticated and contains no
  identity, app, host, credential, or persistence-derived data.

## App host: pre-authentication

```text
GET  /_tiny/auth/login
POST /_tiny/auth/logout
GET  /_tiny/auth/callback
```

`GET /_tiny/auth/login` creates a bounded, server-owned handoff and the
platform identity broker is the **only** browser viewer-session issuance route.
`POST /_tiny/auth/otp` and `POST /_tiny/auth/verify` do not exist on app hosts;
they are reserved gateway paths and deny before app content or identity/session
issuance. There is no brokerless compatibility harness.

The exact admin-host broker owns the browser-facing login flow:

```text
GET  /login
POST /login
GET  /login/verify
POST /login/verify
POST /logout
```

Every platform broker POST requires an exact HTTPS same-origin `Origin` header
matching the derived admin host, an URL-encoded form body, and a bounded
body. Missing, malformed, or cross-origin requests return the same generic
native retry response without issuing OTP, changing a handoff, rotating an
identity, clearing a valid cookie, or serving app bytes. Anonymous app-host
handoff creation is independently rate-limited in bounded in-memory state; it
does not spend or bypass the email OTP budget.

Successful admin-host verification sets only the global identity cookie and
continues an app-bound callback when the login began from an app. The callback
issues an app-local child cookie only after its atomic state and policy checks.

For a document navigation with no app cookie, the app host uses a
server-created, state-bound handoff through the admin host. The browser
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

Supported V1 operations are subscribe/unsubscribe and bounded publish to a
custom app channel, plus `subscribe_kv`/`unsubscribe_kv` by prefix and
`subscribe_collection`/`unsubscribe_collection` by collection name. The typed
unsubscribe frames remove server subscription state immediately. Policy,
session, or app revocation closes affected connections. Connections,
subscriptions, frame sizes, publish rates, and outbound queues are limited.
Delivery is best-effort: there is no persistence, history, replay, ordering
guarantee, or resume cursor. After reconnect, clients reread current KV or
collection state.

## Admin host: CLI control API and browser control UI

```text
POST /api/v1/auth/otp
POST /api/v1/auth/verify
GET  /api/v1/whoami

GET  /login
POST /login
GET  /login/verify
POST /login/verify
POST /logout

GET  /api/v1/apps
POST /api/v1/apps
GET  /api/v1/apps/{slug}
POST /api/v1/apps/{slug}/deployments
GET  /api/v1/apps/{slug}/deployments/{id}
POST /api/v1/apps/{slug}/deployments/{id}/activate
GET  /api/v1/apps/{slug}/access
PUT  /api/v1/apps/{slug}/access

GET    /api/v1/apps/{slug}/data/kv?prefix=&limit=&cursor=
GET    /api/v1/apps/{slug}/data/kv/{key}
PUT    /api/v1/apps/{slug}/data/kv/{key}
DELETE /api/v1/apps/{slug}/data/kv/{key}
GET    /api/v1/apps/{slug}/data/collections?limit=&cursor=
GET    /api/v1/apps/{slug}/data/collections/{collection}/documents?limit=&cursor=
GET    /api/v1/apps/{slug}/data/collections/{collection}/documents/{id}
POST   /api/v1/apps/{slug}/data/collections/{collection}/documents
PUT    /api/v1/apps/{slug}/data/collections/{collection}/documents/{id}
DELETE /api/v1/apps/{slug}/data/collections/{collection}/documents/{id}
```

The CLI may combine create/upload/activate into `tiny deploy`, but the server
states remain observable. An async deployment response returns IDs and a status
URL; “ready” is returned only after security probes and TLS readiness. The
client's successful activation response is authoritative for the committed
`active` state. Independent public-gateway evidence still gates CLI success; if
that bounded anonymous proof remains incomplete after activation, the CLI
returns the distinct non-success `active_but_unverified` receipt defined by the
control and deployment contract rather than collapsing it into
`deploy_failed`.

An activation that does not commit returns HTTP `409` and the ordinary safe
error envelope. Its `error.code` is exactly one of
`activation_policy_not_ready`, `activation_certificate_not_ready`,
`activation_candidate_probe_failed`, `activation_capability_not_ready`, or
`activation_commit_failed`. `error.request_id` exactly matches a syntactically
valid `X-Request-ID`. A deployer CLI may show only the already-known deployment
ID, literal state `verified`, that allowlisted code, and the matching request
ID as a non-success `activation_failed` receipt. Unknown codes, malformed
envelopes, absent/invalid/mismatched request IDs, other statuses, and transport
failures are the generic non-success `deploy_failed` result with no receipt.
Neither result claims `active` or weakens the required anonymous public probe.

The deployer-data routes are bearer-control API routes, never app-host SDK
routes. They require an active deployer, current token scope, optional token
app binding, and current ownership of `{slug}` before TinyHost resolves the
private immutable app ID. `data:read` permits bounded KV/document reads;
`data:write` permits one optimistic-versioned KV/document mutation at a time.
The route slug is not a database selector, and neither an operator browser
session nor a viewer credential has this authority. The full semantics,
responses, limits, audit behavior, and denials are defined in
[`deployer-data-contract.md`](deployer-data-contract.md).

## Credential split

- Global browser identity: host-only `__Host-tiny_identity` cookie on the
  exact admin host. It proves email identity only; dashboard role is a separate
  current server-side check.
- App plane: host-only app session cookie; never a parent-domain cookie.
- CLI/agent: bearer token, never accepted as an app viewer session.

Global identities and app sessions are opaque persisted credentials. One
browser profile has one global browser identity; it may hold independent app
sessions for allowed apps. The global identity is exchanged only via a
five-minute-or-less, state-bound, one-time app handoff, and the current policy
is checked again on every protected app request. Sessions survive a normal
server restart because only hashes and lifecycle state persist. Revoking an app
session is local; global switch/revocation removes all child app sessions.

## Compatibility rule

Additive response fields are allowed within V1. Removing/renaming fields,
changing authorization meaning, or changing idempotency requires a versioned
contract decision.
