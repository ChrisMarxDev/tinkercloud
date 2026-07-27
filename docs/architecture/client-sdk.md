# Tiny Client SDK

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
@tinyhost/sdk
```

Browser-first TypeScript with ESM output, strong types, no framework dependency,
and a deliberately small runtime.

```ts
import { tiny } from "@tinyhost/sdk";

const viewer = await tiny.user.current();
const current = await tiny.kv.get("reviews/invoice-42");
await tiny.kv.set("reviews/invoice-42", {
  status: "approved",
  by: viewer.identity.email,
}, {
  expectedVersion: current?.version,
});

const stop = tiny.live.onKvChange(
  { prefix: "reviews/" },
  ({ key }) => refresh(key),
);
```

## Initial modules

```ts
tiny.user.current()

tiny.kv.get(key)
tiny.kv.set(key, value, options?)
tiny.kv.delete(key, options?)
tiny.kv.list({ prefix, limit, cursor })

tiny.blobs.upload(file, options?)
tiny.blobs.get(id, options?)
tiny.blobs.list({ limit, cursor })
tiny.blobs.delete(id, options?)

tiny.live.channel(name).connect()
tiny.live.channel(name).on(event, handler)
tiny.live.channel(name).publish(event, payload)
tiny.live.onKvChange({ prefix }, handler)

tiny.capabilities.list()
tiny.app.info()
```

`capabilities.list()` lets an app and coding agent discover what the operator
and manifest enabled without probing endpoints.

## Transport

The SDK calls same-origin reserved endpoints:

```text
/_tiny/api/v1/me
/_tiny/api/v1/kv/*
/_tiny/api/v1/blobs/*
/_tiny/api/v1/capabilities
/_tiny/ws/v1
```

The browser automatically sends the host-only app session cookie. The SDK does
not store or accept:

- application IDs;
- database credentials;
- viewer tokens;
- provider/API secrets;
- TinyHost operator or deployer tokens.

TinyHost derives the app from the hostname and the viewer from the session.

## Error model

Methods throw typed, actionable errors:

```ts
try {
  await tiny.kv.set("settings/default", next, { expectedVersion });
} catch (error) {
  if (error instanceof TinyVersionConflictError) {
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
`TinyVersionIncompatibleError` means the installed SDK major cannot safely use
the server API; upgrade `@tinyhost/sdk` and retry. Raw callers that omit the
version header remain compatible during V1, while a supplied unsupported or
malformed major receives the same actionable `426` error.

## Compatibility

- The SDK exposes its version in requests.
- The server returns supported capability/API versions.
- Capability modules are independently versionable.
- Realtime reconnect does not imply replay; apps reread relevant KV state.
- Additive response fields do not break older clients.
- A mismatch produces an actionable build/dependency message.

## Capability extension contract

Future modules should feel native:

```ts
const response = await tiny.llm.generate({
  model: "operator-default",
  prompt: "Summarize this review",
});

const issue = await tiny.jira.createIssue({
  project: "OPS",
  summary: "Follow up",
});
```

These calls go to TinyHost. TinyHost resolves the app grant and operator-managed
connection, injects credentials server-side, calls the provider, applies quotas
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

- Contract tests run the SDK against a real TinyHost HTTP test server.
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
canonical repository and `@tinyhost` registry scopes are confirmed. See the
[SDK distribution guide](../operations/sdk-distribution.md) and
[distribution contract](../../specs/sdk/distribution-contract.md).

## Deployable example gallery

[`examples/sdk-apps/`](../../examples/sdk-apps/) contains three private static
apps that use the package surface in realistic flows:

- Shared Checklist covers capability discovery, prefix pagination, KV
  create/update/delete, optimistic conflicts, cancellation, and
  `live.onKvChange`.
- Team Pulse covers server-derived viewer identity, custom live channel
  connect/on/publish/status/close, and KV recovery after reconnect.
- Quick Poll covers a current record with `kv.get`, viewer-keyed records,
  versioned vote changes/deletion, prefix aggregation, and typed failures.

Their build copies the already-built ESM SDK into each release and converts the
package import to a same-release module path. No runtime CDN, API origin, app
selector, or credential is added. `npm test` compiles the sources and builds
all releases in a temporary directory as part of the SDK contract.
