# `@tinkercloud/sdk`

Typed browser SDK for Tinkercloud app identity, capability discovery, JSON KV,
private utility-grade blobs, and ephemeral realtime channels.

The SDK is designed for static apps served by Tinkercloud. It uses same-origin
platform endpoints and accepts neither an app ID nor a platform secret.

> The package is prepared but has not been published. Tinkercloud is pre-release
> software and the SDK API may change.

## Install

After the first npm release, use the package manager you already have:

```sh
npm install @tinkercloud/sdk
pnpm add @tinkercloud/sdk
yarn add @tinkercloud/sdk
bun add @tinkercloud/sdk
deno add npm:@tinkercloud/sdk
```

The npm artifact is one ESM package; npm, pnpm, Yarn, Bun, and Deno consume the
same reviewed files. A matching JSR source package is also prepared:

```sh
deno add jsr:@tinkercloud/sdk
```

## Use

```ts
import { tinker } from "@tinkercloud/sdk";

const current = await tinker.user.current();
const capabilities = await tinker.capabilities.list();

console.log(current.identity.email);
console.log(current.app.slug);
console.log(capabilities);
```

Use the SDK only from an app served by Tinkercloud. Authentication comes from the
current app's host-only browser session. There is no app ID, token, or endpoint
secret to configure.

For cancellation:

```ts
const controller = new AbortController();
const entry = await tinker.kv.get("example", {
  signal: controller.signal,
});
```

Attachments stay in the current app's shared private namespace. They have no
public URL and are utility-grade local VPS data:

```ts
const file = document.querySelector<HTMLInputElement>("input[type=file]")!.files![0];
const uploaded = await tinker.blobs.upload(file, { signal: controller.signal });
const bytes = await tinker.blobs.get(uploaded.id, { signal: controller.signal });
```

Realtime events are ephemeral hints. Call `channel.subscribe()` before
`connect()` and `channel.unsubscribe()` when delivery is no longer wanted.
Read current KV state after connecting or reconnecting; the SDK does not
promise history, replay, ordering, or durable delivery.

See the repository's
[client SDK documentation](https://github.com/ChrisMarxDev/tinkercloud/blob/main/docs/architecture/client-sdk.md)
for the complete API model, compatibility rules, examples, and security
boundary.

## Development

```sh
npm ci
npm test
npm publish --dry-run
deno publish --dry-run
```

The dry-run commands validate the registry artifacts without publishing them.
Release preparation and namespace blockers are documented in the repository's
[SDK distribution guide](https://github.com/ChrisMarxDev/tinkercloud/blob/main/docs/operations/sdk-distribution.md).

## License

Apache-2.0. See
[LICENSE](https://github.com/ChrisMarxDev/tinkercloud/blob/main/LICENSE).
