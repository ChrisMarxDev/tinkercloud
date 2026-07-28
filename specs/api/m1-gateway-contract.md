# M1 gateway contract

## Trust boundary

Only the public gateway accepts HTTP. It derives the application from a
canonical `Host` and the viewer from an opaque host-only app session cookie or
from the platform-host global identity during a server-created handoff. A
protected dispatcher receives an `appauth.AuthorizationContext`, which only
the `appauth` package can implement. It never accepts an app identifier from a
request parameter, cookie, or header.

## Deny-path charter

App resolution before authorization is metadata-only: it may query the active
app, deployment, manifest, and durable file evidence, but must not open,
stat, hash, or otherwise read the release tree. Before any release path is
opened, the gateway must deny malformed or unknown hosts, inactive
applications, login/OTP routes, reserved unknown routes, missing/wrong/revoked
sessions, absent/corrupt policy, and policy-store failure. These responses
contain no release bytes. `/_tiny/*` is reserved and cannot use SPA fallback.

After the gateway produces a sealed `AuthorizationContext`, protected static
dispatch verifies the current immutable release tree against the active
deployment's hash and complete durable file evidence before opening or serving
any app file. Missing, changed, symlinked, malformed, or mismatched release
evidence fails closed. Tests must count release inspections: every pre-auth,
reserved, anonymous, and wrong-session route performs zero inspections; an
authorized static request verifies once before returning content.

## Current implementation seam

The composed V1 server uses repository interfaces with SQLite persistence,
the configured OTP delivery adapter, and the single public gateway. Test
harnesses may substitute in-memory repositories, but that seam must not alter
the gateway's sealed authorization-context boundary or permit a second public
listener. Static, capability, and WebSocket routes remain gateway-owned.

Static files are served only from a private immutable release root through
`os.OpenRoot` with Go 1.25.12 pinned, after the evidence verification above.
The M0 open-beneath gate is production: rooted resolution prevents path escape
during symlink races.

## Viewer login

JSON and form OTP requests return an opaque `otp_...` transaction with the
same accepted shape whether delivery is eligible, ineligible, or unavailable.
Forms are bounded and HTML-escaped. Verification atomically consumes the
challenge, rechecks current policy, and issues only a host-only Secure,
HttpOnly, SameSite=Lax app cookie with a safe relative return target.

The global viewer identity and app-bound handoff are governed by
[`global-identity-handoff-contract.md`](global-identity-handoff-contract.md).
The platform-host global cookie proves email identity only; it is never used as
app request authority. A missing app session may redirect an unambiguous
document navigation through the exact platform identity broker only with a
server-created state-bound handoff. Handoff issuance and consumption both
recheck current policy, consume the grant once, and create the existing
app-host cookie only after success.

For a genuine unauthenticated browser document navigation to a protected static
path, the gateway may redirect only to that same app host's
`/_tiny/auth/login?return={safe-relative-request-uri}`. It does not use a
platform cookie as app identity and does not read release content first. API,
asset, WebSocket, range, non-document, and ambiguous requests continue to
return the normal JSON `401 not_authorized` denial without a redirect.

`POST /_tiny/auth/logout` is the only app-host *local* logout mutation.
It requires an exact same-origin `Origin`, validates any form return value as a
safe relative path, revokes only the current app's host-only session, expires
that app cookie, and redirects a form submission to its app-host login page.
JSON callers receive no-content success. `GET` logout is reserved/denied and
never mutates state.

Global identity switch is a platform-host POST verification outcome, not an
app-host GET/POST parameter. It revokes the old global family and child app
sessions before issuing the replacement identity; no app-local logout can
select or reveal a global identity.

## OTP abuse controls

Before policy lookup, challenge creation, or email-provider work, app and
control OTP request and verify routes pass a bounded, in-memory rolling-window
guard. It layers IP, bounded request-fingerprint, HMAC-keyed normalized-email,
app, and global budgets. The fingerprint is a keyed, bounded abuse-control
hint derived from the canonical `RemoteAddr` peer IP plus bounded
`User-Agent`/`Accept-Language`; it excludes cookies, URLs, and every forwarded
header, is never an identity claim, and is never retained raw. Verification
binds the opaque transaction to keyed email and fingerprint digests so changing
browser hints cannot bypass the original request's email budget. Raw addresses,
emails, and fingerprints are neither retained nor logged. Exhausted dynamic
state fails closed instead of evicting an active bucket. Request throttling
retains the normal generic acceptance response; JSON verification/control
callers receive only the typed `rate_limited` error, which never reveals
eligibility. Rejected dimensions are indistinguishable.
