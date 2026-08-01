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
- The operator controls `tinkercloud.fun` in the Cloudflare account. The
  checked-in deployment configuration owns its exact attachment to this Worker.

## Required behavior

- Use `wrangler.jsonc` as the deployment source of truth.
- Use a current compatibility date and `nodejs_compat`.
- Generate binding types after binding changes.
- Enable persisted production logs and sampled traces.
- Build and test before deployment; support a no-mutation dry run.
- Expose production only at the exact `tinkercloud.fun` custom domain; disable
  `workers.dev` and preview URLs.
- Add future Cloudflare products through typed bindings, not REST calls from the
  Worker when a binding exists.

## Deny paths

Deployment configuration must fail verification when it contains a Pages build
directory, OpenAI Sites project metadata, a wildcard, implicit, or additional
route, a credential-like variable, a missing asset/image binding, or disabled
observability. Deployment must not be reported complete from a local build or
dry run alone; the public hostname must return the intended page over HTTPS.
