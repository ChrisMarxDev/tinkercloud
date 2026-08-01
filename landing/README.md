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

Production deploys also run automatically after landing changes reach `main`,
or by manually dispatching `.github/workflows/deploy-landing.yml`. Configure
these secrets in the GitHub `landing-production` Environment:

- `CLOUDFLARE_API_TOKEN`: a narrowly scoped token that can deploy this Worker
  and manage its exact custom-domain route.
- `CLOUDFLARE_ACCOUNT_ID`: the Cloudflare account that owns the Worker and
  `tinkercloud.fun`.

The action tests and builds before deployment, then verifies the public HTTPS
page and its identifying content before reporting success.

The checked-in Wrangler config publishes the Worker only at the exact
`tinkercloud.fun` custom domain. The account's `workers.dev` and preview URLs
are disabled. Cloudflare creates the apex DNS record and manages its certificate
when Wrangler deploys the Worker.

Add future Cloudflare capabilities as Worker bindings in `wrangler.jsonc`.
Keep non-secret configuration there, generate bindings with `npm run types`,
and store credentials with `wrangler secret put`; never commit secrets or put
them in command arguments.
