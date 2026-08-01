# ADR 0051: One root domain and a single browser identity broker

**Status:** Accepted, implemented, and live verified

## Context

Tinkercloud currently describes a configured platform host, app hosts beneath an
app suffix, a platform-host global viewer identity, host-only app sessions, and
a distinct dashboard/control browser session. The separate dashboard browser
session duplicates OTP/browser credential machinery and makes one person’s
browser state harder to explain, revoke, and audit. It also risks future
compatibility layers where different browser credentials overlap.

Tinkercloud must keep the gateway as the only public security boundary and keep
untrusted app origins isolated. Browser authentication cannot turn a matching
email into dashboard role or app access; deployer CLI credentials must remain
separate.

## Decision

The operator supplies one root domain. Tinkercloud reserves the exact hostname
`admin.<domain>` for the dashboard and the **sole browser identity broker**.
Deployed apps use exactly `<slug>.<domain>` under `*.<domain>`. `admin`, `api`,
`auth`, `status`, `www`, `docs`, and `install` are reserved labels and cannot be
app slugs. Unknown, malformed, or reserved non-route hosts fail closed before
app content lookup.

`admin.<domain>` issues one opaque platform browser identity through the OTP
flow. Its `__Host-` cookie is `Secure`, `HttpOnly`, `Path=/`, host-only (no
`Domain` attribute), and is never sent to an app host. It proves only a verified
viewer identity. Dashboard access requires an independent current role check
for that identity; each app requires an independent current app-policy/allowlist
check for the server-derived app. Matching normalized email text carries no
authority between those decisions.

An app-host navigation lacking a valid local session starts a short-lived,
one-time, server-created handoff through the exact admin broker. The gateway
stores the resolved app, exact callback, state, and validated safe relative
path plus query. It checks app policy before grant and again when the exact app
callback atomically consumes it. Only then does the app host set its own opaque,
host-only `__Host-` child session. The identity cookie is never accepted on an
app request as app authority. Redirect preservation applies to a safe path and
query only; fragments are not guaranteed.

App-local logout revokes only the current app’s child session. Global Tinkercloud
logout and browser identity switching happen only at `admin.<domain>` and, after
the revocation transaction commits, revoke every derived child app session and
close affected live connections. They do not revoke CLI bearer tokens. CLI and
agent tokens remain a separate bearer credential class and are never accepted
as browser or viewer credentials.

This is a replacement, not a compatibility layer. The old separate dashboard
browser login/control-session issuance model is removed during implementation;
it must not coexist as a second browser issuance path.

Wildcard DNS routing and TLS are distinct requirements. The operator needs one
wildcard DNS record. Tinkercloud may continue issuing certificates on demand for
each exact admin/app hostname; a wildcard certificate is optional. Setup and
activation evidence must prove trusted certificate coverage for the exact host
being served because wildcard DNS alone is insufficient.

## Consequences

- One browser OTP identity makes dashboard and app navigation simpler without
  merging dashboard role, app policy, or CLI authority.
- App origins remain isolated: there is no parent-domain credential, shared app
  session, client-selected app identifier, or credentialed admin-to-app CORS.
- Because sibling subdomains are same-site but not the same origin, every
  state-changing route needs exact HTTPS `Origin` validation and its own CSRF
  protection. SameSite is defence in depth, not authorization.
- `__Host-` cookie invariants, exact-host redirect construction, canonical host
  parsing, deny-by-default CORS, reserved-label routing, app/callback state,
  and wildcard DNS/TLS readiness become mandatory security verification.
- Contract, schema, UI, migration, skill, and negative E2E work must replace
  the previous control cookie/dashboard login semantics before this decision is
  reported implemented. Existing control cookies and direct app OTP sessions
  require explicit invalidation or a bounded migration that cannot become a
  parallel issuance path.

## Supersedes and follow-up

This ADR supersedes the separate dashboard browser-session portion of ADR 0024
and the platform-host naming/issuance topology assumed by ADR 0033. ADR 0002's
host-only app-session isolation and ADR 0004's per-host TLS-first decision
remain in force; only the DNS setup is consolidated to one wildcard record.

The replacement contract is
[`browser-identity-handoff-contract.md`](../../specs/api/browser-identity-handoff-contract.md),
with a deny-path charter for cross-origin requests, cookie scope/overwrite,
handoff replay, reserved hosts, role/app-policy separation, logout scope,
path-plus-query preservation, and DNS/TLS readiness.

## Implementation evidence

The signed `0.1.0` build completed the guarded clean-host VPS acceptance run on
2026-07-31 at the public-safe `testing.tinkercloud.example` placeholder. The black-box run proved one dashboard
OTP identity across two allowed app hosts without a second OTP, independent
host-only app sessions, preserved path and query state, replay and wrong-app
handoff denial, app-local logout, global logout with child-session revocation,
account switching, private static denial, cross-app data isolation, and
restart persistence. The redacted owner-only result is retained under
`.tinker/vps/unattended-reports/` as
`vps-e2e-<timestamp>-<run>.status`.
