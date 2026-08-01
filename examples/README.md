# Examples

## Human starter app

[`starter-app/`](starter-app/) contains **Tinker Ritual**, a dependency-free,
owner-only static app designed to be copied, customized, and deployed as a
person's first Tinkercloud app.

Follow the [human setup guide](../docs/getting-started/first-app.md) to host it.

## Public static example

[`public-static-product-story/`](public-static-product-story/) is a deliberately
capability-free v2 public-static product story. It opts into search indexing to
demonstrate that indexing is a separate, reviewed release choice. It is not a
starter: deployment requires an operator-enabled public gate and the deployer's
explicit public acknowledgement. The starter app and SDK gallery remain private
and owner-only by default.

## SDK application gallery

[`sdk-apps/`](sdk-apps/) contains three polished, deployable private apps that
collectively exercise the V1 SDK:

- **Shared Checklist** — KV create/list/update/delete, cursors, optimistic
  concurrency, cancellation, and KV change subscriptions.
- **Team Pulse** — current viewer/app state, custom live publish/subscribe,
  connection status, close, and reconnect recovery through KV.
- **Quick Poll** — `get`, viewer-keyed records, aggregation, vote replacement
  and deletion, typed errors, and ephemeral live refresh hints.

Run `npm run build` in `sdk-apps/` to create each app's local `dist/` release,
then deploy from the selected app directory.

## Test applications

[`test-apps/`](test-apps/) contains contract and VPS acceptance fixtures. Those
apps favor narrow executable evidence over presentation and are not the
beginner template.
