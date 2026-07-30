# M4 capability contract

This contract defines the app-capability seam. The gateway constructs a typed
authorization context after resolving the host, session, and current policy.
KV, blob, live, and capability dispatchers accept that context; callers never
supply an app ID or viewer identity.

## KV v1

- Keys are non-empty UTF-8 strings, at most 256 bytes, and cannot contain NUL.
- Values are valid JSON and at most 65,536 bytes.
- Reads, mutations, and prefix lists are scoped to the authorization context's
  app. Lists are bounded and use an opaque, key-based cursor.
- When the server-derived app capability is disabled, every KV operation
  (get, set, delete, and prefix list) returns `capability_unavailable` before
  it resolves or touches the KV repository.
- `expected_version` is optional; when supplied it must match the current
  version. A missing key has no version.
- Every successful mutation gets a monotonically increasing per-key version.
- Limits default to 10,000 keys and 100 MiB per app. This is utility-grade
  state, not a business-critical durability promise.
- Every KV request is admitted before repository access against bounded,
  server-derived viewer-within-app, app, and fixed global rolling-window
  buckets. The viewer bucket prevents one viewer from consuming an app's
  share, while the app and global buckets prevent a large allowlist from
  multiplying capacity. Dynamic viewer/app scope state has a configured hard
  maximum; a new scope at that bound, or any exhausted bucket, returns
  `rate_limited` without appending to any bucket or touching the repository.

## Live v1

- A transport adapter is authenticated by the gateway before attaching to the
  hub. The hub does not upgrade HTTP connections.
- Every connection is scoped to its authorization context's app and session.
- Channel names cannot be empty, reserved (`_tinker` prefix), or exceed 128
  bytes. Events are bounded JSON payloads.
- Delivery is best effort only: there is no history, replay, ordering, resume
  cursor, or durability guarantee. Reconnecting clients reread KV.
- Session, policy, or app revocation closes matching in-memory connections.
- Client frames and publish payloads are limited to 32 KiB by default. A
  connection may publish at most 20 events in a one-second window. Outbound
  delivery uses a bounded per-connection queue and bounded write deadline; a
  full queue or timed-out write closes only that slow consumer.
- The transport has explicit bounded V1 liveness defaults: 60 seconds idle
  lifetime, a ping every 20 seconds, a 10-second pong extension, and a
  3-second outbound write deadline. Operators may tighten these values in the
  server configuration, but unsafe zero, negative, or unbounded values are
  rejected. A successful pong extends liveness; a peer that does not read or
  pong is closed no later than the configured idle bound. These controls do
  not create a delivery, replay, history, ordering, or durability guarantee.

## Lightweight blobs v1

The V1 blob surface is defined normatively in
[`blob-contract.md`](blob-contract.md). It provides app-shared, opt-in,
local-disk-backed upload/get/list/delete through opaque server-issued IDs.
SQLite is the catalog and only ready metadata can serve bytes. There is no
public URL, path, bucket, mount, provider credential, remote adapter, replace,
resumable upload, inline-hosting promise, or per-viewer ACL in V1.

## Deny-path charter

Anonymous/malformed authorization contexts, cross-app keys/blob IDs/channels,
invalid JSON/blob metadata, over-limit values/uploads/lists, stale versions,
reserved channels, and revoked sessions must fail without a mutation, byte
disclosure, or delivery to another app.

Live transport denial and failure charter: authentication and exact
same-origin validation occur before the WebSocket upgrade; a missing or invalid
authorization context never creates a transport. A client that does not read
or pong must be disconnected within the bounded idle/pong interval without
leaking a reader or writer goroutine. A full outbound queue or expired write
deadline closes only that connection and does not block another app or
subscriber. Session, policy, and app revocation immediately close matching
connections and a buffered inbound frame from that detached connection cannot
publish afterward.

A capability-disabled app must not use KV or blobs as an existence, timing,
prefix, ID, or quota oracle: every operation denies before a repository or byte
store call.

Cookie-authenticated KV/blob mutations and WebSocket upgrades require an exact
same-origin `Origin` header; GET remains usable without one. The gateway emits
same-origin CSP (`default-src 'self'`, restricted `connect-src`, no object,
base, or frame ancestors) and never emits wildcard CORS.

## SDK/API compatibility

- App API clients MAY send `X-Tinker-SDK-Version` as a semantic version. The
  current V1 API accepts the supported SDK major version (`0` while the SDK is
  pre-1.0). A missing header remains compatible so raw HTTP and previously
  shipped clients are not locked out by header negotiation.
- An unsupported or malformed supplied major version is rejected after normal
  gateway authorization with `426` and the stable
  `sdk_version_incompatible` code. The response includes only an actionable
  upgrade message and safe request ID; it discloses no app or policy data.
- SDKs map that error to `TinkerVersionIncompatibleError`. Apps upgrade their
  dependency and retry; they do not add an app ID or control-plane credential.

The real-listener SDK contract test proves current user, current app,
capability discovery, KV get/set/list/delete, blob upload/get/list/delete,
typed error mapping, explicit live subscribe/unsubscribe behavior, no
caller-selected app/storage key, header negotiation, and the compatible
raw/absent-header path. It uses a real gateway, authorization context, session,
app-scoped repositories, and byte store rather than a mocked fetch handler.

## Public example application contract

The repository ships four human-readable, deployable SDK examples:

- a shared checklist demonstrates current viewer/app reads, capability
  discovery, bounded prefix pagination, create/update/delete with optimistic
  versions, cancellation, and KV change notifications;
- a team pulse demonstrates server-derived viewer identity, app-scoped current
  state, custom live publish/subscribe, connection status, close, and KV reread
  after reconnect; and
- a quick poll demonstrates one current-state record plus viewer-keyed records,
  prefix aggregation, optimistic vote changes, deletion, and ephemeral live
  refresh hints.

Before the V1 exit gate, one gallery example must also demonstrate blob
capability discovery, bounded upload, attachment retrieval, cursor listing,
deletion, cancellation, typed quota/error handling, and the local-disk
durability disclaimer.

Each example is an ordinary static Tinkercloud project with a private,
owner-only-by-default `tinker.yaml`. Its explicit build step copies the built ESM
SDK into the release and rewrites only the package import to that local file.
The deployable release contains no remote script, CDN dependency, app ID,
viewer token, deployer token, provider secret, or database credential.

Examples discover capabilities before calling KV, blob, or live methods. A
missing KV/blob grant leaves that feature unavailable without probing its
repository or byte store. A missing live grant degrades to explicit/manual
refresh where the app can remain useful. Live events carry bounded hints only;
rendering and reconnect recovery always read current KV. Every network-backed
boot or refresh path accepts an
`AbortSignal`, and typed errors show safe actionable copy plus a request ID
when one exists.

The example check compiles source against the supported SDK types, builds every
static release, verifies local SDK import resolution and required artifact
files, and rejects remote URLs or client-selected app/credential patterns.
