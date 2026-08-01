# Tinkercloud landing page

A small, single-page introduction to Tinkercloud. It explains the product
promise and the folder-to-private-URL workflow without adding forms, analytics,
or application state.

## Local preview

```sh
npm install
npm run dev
```

## Verification

```sh
npm test
npm run deploy:dry-run
```

## Cloudflare deployment

The landing page runs as the `tinkercloud-landing` Cloudflare Worker. Workers
Static Assets is the source-of-truth hosting model; no Pages project or OpenAI
Sites metadata is required.

```sh
npm run cf:whoami
npm run deploy
```

The checked-in Wrangler config publishes the Worker only at the exact
`tinkercloud.fun` custom domain. The account's `workers.dev` and preview URLs
are disabled. Cloudflare creates the apex DNS record and manages its certificate
when Wrangler deploys the Worker.

Add future Cloudflare capabilities as Worker bindings in `wrangler.jsonc`.
Keep non-secret configuration there, generate bindings with `npm run types`,
and store credentials with `wrangler secret put`; never commit secrets or put
them in command arguments.
