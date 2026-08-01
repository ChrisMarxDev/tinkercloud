# ADR 0001: Host the landing page on Cloudflare Workers

## Status

Accepted 2026-07-31.

## Context

The vinext landing page already targets the Cloudflare Workers runtime, but its
build still carried OpenAI Sites scaffold metadata and had no checked-in
Wrangler deployment contract. Cloudflare recommends Workers Static Assets for
new static and full-stack applications; Pages remains supported but receives a
narrower set of new platform features.

## Decision

Deploy the landing page as one Cloudflare Worker with Workers Static Assets,
server rendering, generated binding types, and Workers observability. Attach
the operator-controlled `tinkercloud.fun` apex as the sole production endpoint;
disable the `workers.dev` and preview endpoints. A GitHub Actions workflow
deploys landing changes from `main` with environment-scoped Cloudflare
credentials and verifies the intended page through the public HTTPS endpoint.

## Consequences

- The site can add D1, R2, Durable Objects, Queues, Workflows, or other bindings
  without migrating from Pages Functions later.
- Cloudflare is a public boundary for the marketing site, not a second public
  listener or authority for the Tinkercloud application platform.
- Publishing and custom-domain attachment remain explicit external mutations;
  merging a landing change to `main` or manually dispatching the production
  workflow authorizes that mutation.
- Cloudflare owns the generated DNS record and certificate lifecycle for the
  `tinkercloud.fun` Worker custom domain.
