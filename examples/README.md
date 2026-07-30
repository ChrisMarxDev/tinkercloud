# Examples

## Human starter app

[`starter-app/`](starter-app/) contains **Tinker Ritual**, a dependency-free,
owner-only static app designed to be copied, customized, and deployed as a
person's first Tinkercloud app.

Follow the [human setup guide](../docs/getting-started/first-app.md) to host it.

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
