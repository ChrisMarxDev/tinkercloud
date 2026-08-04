# Tinkercloud Client SDK

## Product role

The SDK is the primary programming surface for deployed apps. Raw HTTP remains
a documented protocol and test boundary, but staff, non-technical deployers,
and coding agents should normally work through one small, predictable SDK.

The usability north star is
[Shopify Quick](../product/north-star-quick.md): minimal setup, obvious methods,
useful defaults, browser-callable capabilities, and errors that tell the creator
what to do next.

## V1 shape

Package:

```text
@tinkercloud/sdk
```

Browser-first TypeScript with ESM output, strong types, no framework dependency,
and a deliberately small runtime.

```ts
import { tinker } from "@tinkercloud/sdk";

const viewer = await tinker.user.current();
const current = await tinker.kv.get("reviews/invoice-42");
await tinker.kv.set("reviews/invoice-42", {
  status: "approved",
  by: viewer.identity.email,
}, {
  expectedVersion: current?.version,
});

const stop = tinker.live.onKvChange(
  { prefix: "reviews/" },
  ({ key }) => refresh(key),
);
```

## Initial modules

```ts
tinker.user.current()

tinker.kv.get(key)
tinker.kv.set(key, value, options?)
tinker.kv.delete(key, options?)
tinker.kv.list({ prefix, limit, cursor })

tinker.blobs.upload(file, options?)
tinker.blobs.get(id, options?)
tinker.blobs.list({ limit, cursor })
tinker.blobs.delete(id, options?)

tinker.live.channel(name).subscribe()
tinker.live.channel(name).connect()
tinker.live.channel(name).unsubscribe()
tinker.live.channel(name).on(event, handler)
tinker.live.channel(name).publish(event, payload)
tinker.live.onKvChange({ prefix }, handler)

tinker.capabilities.list()
tinker.app.info()
```

`capabilities.list()` lets an app and coding agent discover what the operator
and manifest enabled without probing endpoints.

`subscribe()` and `unsubscribe()` are explicit, local intent for the connected
channel: call `subscribe()` before `connect()` (or while connected) to receive
events, and `unsubscribe()` when the UI no longer needs them. Managed KV and
collection listeners share one SDK-owned app socket; explicit named channels
remain independent. They do not make events durable, ordered, or replayable.
After reconnect, read current KV or collection state again before rendering.

## Transport

The SDK calls same-origin reserved endpoints:

```text
/_tinker/api/v1/me
/_tinker/api/v1/kv/*
/_tinker/api/v1/blobs/*
/_tinker/api/v1/capabilities
/_tinker/ws/v1
```

The browser automatically sends the host-only app session cookie. The SDK does
not store or accept:

- application IDs;
- database credentials;
- viewer tokens;
- provider/API secrets;
- Tinkercloud operator or deployer tokens.

Tinkercloud derives the app from the hostname and the viewer from the session.

## Error model

Methods throw typed, actionable errors:

```ts
try {
  await tinker.kv.set("settings/default", next, { expectedVersion });
} catch (error) {
  if (error instanceof TinkerVersionConflictError) {
    // reload or merge
  }
}
```

Common categories:

```text
NotAuthenticated
NotAuthorized
CapabilityUnavailable
ValidationFailed
VersionConflict
QuotaExceeded
RateLimited
TemporarilyUnavailable
VersionIncompatible
```

Errors include a safe request ID for operator diagnosis, never secret details.
`TinkerVersionIncompatibleError` means the installed SDK major cannot safely use
the server API; upgrade `@tinkercloud/sdk` and retry. Raw callers that omit the
version header remain compatible during V1, while a supplied unsupported or
malformed major receives the same actionable `426` error.

## Compatibility

- The SDK exposes its strict version and app-API version in HTTP requests and
  the WebSocket subprotocol.
- The server publishes the bounded static compatibility matrix at
  `/api/v1/compatibility`; the exact `/api/v1/version` health document remains
  stable.
- Capability modules are independently versionable.
- Realtime reconnect does not imply replay; apps reread relevant KV state.
- Additive response fields do not break older clients.
- A mismatch produces an actionable build/dependency message.

## Capability extension contract

Future modules should feel native:

```ts
const response = await tinker.llm.generate({
  model: "operator-default",
  prompt: "Summarize this review",
});

const issue = await tinker.jira.createIssue({
  project: "OPS",
  summary: "Follow up",
});
```

These calls go to Tinkercloud. Tinkercloud resolves the current operator-managed
default connection and reactive app policy, injects credentials server-side, calls the provider, applies quotas
and redaction, and returns a bounded result.

## SDK design constraints

- Tree-shakeable and small enough for simple static apps.
- No generated configuration file containing environment secrets.
- Abort/cancellation support for all network operations.
- Stable serializable request/response types.
- Framework adapters may exist later but the core remains framework-neutral.
- SDK documentation examples are executable contract tests.
- Every capability documents permissions, quotas, data sent externally, and
  failure behavior.

## Testing

- Contract tests run the SDK against a real Tinkercloud HTTP test server.
- WebSocket contract tests cover upgrade authentication, app binding,
  revocation, quotas, slow consumers, and reconnect recovery.
- Browser tests cover cookies, redirects, CSP, network cancellation, and errors.
- Two-app tests prove no SDK input can select another app.
- Blob tests prove opaque server-issued IDs, app-shared authorization, bounded
  streaming/cancellation, attachment-only downloads, quotas, and no
  path/bucket/provider input.
- Examples compile against the minimum supported TypeScript configuration.
- A real-listener contract runs the built SDK through the composed gateway with
  a host-derived app session; it covers identity, capabilities, KV CRUD, blob
  upload/get/list/delete, cross-app denial, and compatibility errors.
- Bundle-size regression and dependency/license checks run in CI.
- Distribution checks synchronize npm, JSR, and exported versions, inspect the
  exact package contents, and install/import the npm tarball offline.

## Distribution

The SDK has one ESM API across registries. npm, pnpm, Yarn, Bun, and Deno use
the compiled npm-registry package; JSR publishes the same API from reviewed
TypeScript source. Neither format has runtime dependencies or install scripts.

Package preparation is complete, but publication remains blocked until the
canonical repository and `@tinkercloud` registry scopes are confirmed. See the
[SDK distribution guide](../operations/sdk-distribution.md) and
[distribution contract](../../specs/sdk/distribution-contract.md).

## Deployable example gallery

[`examples/sdk-apps/`](../../examples/sdk-apps/) contains four private static
apps that use the package surface in realistic flows:

- Shared Checklist covers capability discovery, prefix pagination, KV
  create/update/delete, optimistic conflicts, cancellation, and
  `live.onKvChange`.
- Team Pulse covers server-derived viewer identity, custom live channel
  connect/on/publish/status/close, and KV recovery after reconnect.
- Quick Poll covers a current record with `kv.get`, viewer-keyed records,
  versioned vote changes/deletion, prefix aggregation, and typed failures.
- Attachment Shelf covers blob capability discovery, bounded upload and
  attachment download, cursor listing, deletion, cancellation, typed quota
  handling, and the local-VPS durability disclaimer.

The attachment example obtains bytes with `tinker.blobs.get(id)`, creates a
browser download locally, and uses the returned display name only for that
download. It never receives a raw storage URL or path. The gateway delivers
the bytes as `attachment` with `nosniff` and `private, no-store`; uploaded
HTML, SVG, or JavaScript is not an inline Tinkercloud document surface.

Their build copies the already-built ESM SDK into each release and converts the
package import to a same-release module path. No runtime CDN, API origin, app
selector, or credential is added. `npm test` compiles the sources and builds
all releases in a temporary directory as part of the SDK contract.
