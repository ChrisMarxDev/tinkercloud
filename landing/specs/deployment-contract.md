# Landing deployment contract

## Outcome

Build and deploy the public Tinkercloud landing page as one Cloudflare Worker
with Workers Static Assets and a server-rendered vinext entry point.

## Trust boundary and ownership

- Cloudflare terminates public traffic for the marketing site only.
- The landing Worker has no authority over the Tinkercloud gateway, operator,
  deployer, viewer sessions, application storage, or provider credentials.
- Wrangler configuration owns non-secret deployment metadata. Cloudflare's
  encrypted secret store owns any future Worker secret.
- GitHub's `landing-production` Environment owns the CI-only Cloudflare API
  token and account ID. They are deployment credentials, never Worker bindings
  or browser-visible configuration.
- The operator controls `tinkercloud.fun` in the Cloudflare account. The
  checked-in deployment configuration owns its exact attachment to this Worker.

## Required behavior

- Use `wrangler.jsonc` as the deployment source of truth.
- Use a current compatibility date and `nodejs_compat`.
- Generate binding types after binding changes.
- Enable persisted production logs and sampled traces.
- Build and test before deployment; support a no-mutation dry run.
- Deploy landing changes from `main` through the environment-gated GitHub
  Actions workflow using a narrowly scoped Cloudflare API token.
- Expose production only at the exact `tinkercloud.fun` custom domain; disable
  `workers.dev` and preview URLs.
- After Wrangler reports success, fetch the public HTTPS endpoint and verify
  landing-page-specific content before the workflow reports success.
- Publish only social metadata whose referenced asset exists and has been
  reviewed against the current landing-page visual system. Omit the image and
  use text-only sharing metadata while no approved asset exists.
- Add future Cloudflare products through typed bindings, not REST calls from the
  Worker when a binding exists.

## Deny paths

Deployment configuration must fail verification when it contains a Pages build
directory, OpenAI Sites project metadata, a wildcard, implicit, or additional
route, a credential-like variable, a missing asset/image binding, or disabled
observability. Deployment must not be reported complete from a local build or
dry run alone; the public hostname must return the intended page over HTTPS.
Landing metadata must not reference a deleted, stale, or visually superseded
social image.
The production workflow must deny pull-request execution, missing
environment-scoped credentials, non-`main` refs, and public verification
failures.
