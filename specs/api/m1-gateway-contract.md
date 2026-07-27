# M1 gateway contract

## Trust boundary

Only the public gateway accepts HTTP. It derives the application from a
canonical `Host` and the viewer from an opaque host-only session cookie. A
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

M1 uses repository interfaces and an in-memory implementation for executable
gateway evidence. SQLite, Resend, ACME, and WebSocket remain unselected M0
spikes; adding their dependencies/providers needs approval under PRD §25.

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

For a genuine unauthenticated browser document navigation to a protected static
path, the gateway may redirect only to that same app host's
`/_tiny/auth/login?return={safe-relative-request-uri}`. It does not use a
platform cookie as app identity and does not read release content first. API,
asset, WebSocket, range, non-document, and ambiguous requests continue to
return the normal JSON `401 not_authorized` denial without a redirect.

`POST /_tiny/auth/logout` is the only app-host logout/account-switch mutation.
It requires an exact same-origin `Origin`, validates any form return value as a
safe relative path, revokes only the current app's host-only session, expires
that app cookie, and redirects a form submission to its app-host login page.
JSON callers receive no-content success. `GET` logout is reserved/denied and
never mutates state.

## OTP abuse controls

Before policy lookup, challenge creation, or email-provider work, app and
control OTP request and verify routes pass a bounded, in-memory rolling-window
guard. It derives the source only from the server-provided `RemoteAddr` (never
forwarded request headers), uses HMAC-keyed normalized-email values, and layers
IP, email, app, and global budgets. Raw addresses are neither retained nor
logged. Request throttling retains the normal generic acceptance response;
JSON verification/control callers receive only the typed `rate_limited` error,
which never reveals eligibility. Rejected dimensions are indistinguishable.
