# TinyHost SDK example apps

These three dependency-free static apps collectively exercise the V1 browser
SDK:

| App | Main SDK ideas |
| --- | --- |
| [`shared-checklist`](shared-checklist/) | Viewer/app identity, capability discovery, KV create/list/update/delete, cursors, optimistic versions, cancellation, KV change subscriptions |
| [`team-pulse`](team-pulse/) | Viewer-keyed current state, custom live channels, publish/subscribe, connection status, close, reconnect recovery through KV |
| [`quick-poll`](quick-poll/) | Current poll state, viewer-keyed votes, prefix aggregation, vote changes/deletion, typed errors, ephemeral refresh hints |

All manifests are private and owner-only by default. Add viewer emails or
domains to the selected app's `tiny.yaml` before sharing it.

## Build

From this directory:

```sh
npm run build
```

The build compiles the repository SDK, then creates a `dist/` directory inside
each app. Every release receives a local `tiny-sdk.js`; no CDN or runtime
package resolution is required.

Deploy one app from its project directory:

```sh
cd shared-checklist
tiny deploy .
```

KV is utility-grade current state. Live events are best-effort hints: the
examples always reread KV after events, reconnects, and tab visibility changes.
