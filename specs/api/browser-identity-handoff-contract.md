# Unified browser identity and app-bound handoff contract

**Status:** V1 replacement contract

## Purpose and topology

The operator supplies one canonical root domain. Tinkercloud derives:

- `admin.<domain>` as the exact dashboard, browser OTP, identity switch, and
  global-logout host; and
- `<slug>.<domain>` as one deployed app host.

`admin`, `api`, `auth`, `status`, `www`, `docs`, and `install` are reserved
case-insensitive labels and can never identify an app. An app hostname contains
exactly one canonical lowercase label below the configured root domain.
Unknown, malformed, nested, reserved, or ambiguous hosts deny before app,
policy, release, or capability lookup.

Authentication answers only “which normalized email identity is this browser?”
Dashboard role checks and app access-policy checks answer separate
authorization questions on every relevant request. Matching email text does not
transfer authority between them.

## Browser credentials

`admin.<domain>` issues one opaque `__Host-tinker_identity` cookie after the
generic OTP flow. It is `Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`,
host-only, and has no `Domain` attribute. It is the only browser credential
accepted by the dashboard and the identity broker. Dashboard authentication
maps its server-derived identity to the current `users` row on every request:
an active operator or deployer may receive the corresponding dashboard; a
viewer with no active role receives no dashboard data.

The old dashboard-specific OTP channel, `__Host-tinker_control` cookie, CSRF
cookie tied to that control credential, and `sessions.scope='control'`
credential class do not exist in this architecture. CLI and deployment-agent
bearers remain separate credentials and are never accepted from browser
cookies.

The platform may retain one opaque `__Host-tinker_browser` binding cookie solely
to serialize concurrent OTP completions in a browser profile. It is host-only,
HTTP-only, non-authorizing, absent from URLs/forms/JavaScript/logs/app requests,
and rejected as an identity, app session, dashboard credential, or bearer.

Global identity sessions are hashed at rest, have a 30-day absolute lifetime,
rotate after 24 hours, and allow the immediately previous secret for at most 60
seconds. Replay after that overlap revokes the identity family and every child
app session before denial.

## Dashboard authorization

Every dashboard request validates the global identity session, applies any
credential rotation, and looks up the current active operator/deployer role by
normalized identity email. Role removal affects the next request without
revoking the identity itself: the person may remain a viewer of apps whose
current policy allows them. Operator/deployer authority never comes from an app
session, browser binding, matching client input, or CLI token presented as a
cookie.

Dashboard mutations use exact HTTPS `Origin` validation plus a CSRF token bound
to the global identity family. Sibling app origins are same-site but
cross-origin, so `SameSite` is only defence in depth. Credentialed CORS between
admin and app origins is denied.

## App handoff and URLs

An app document navigation without a valid local session creates one
server-side, opaque, short-lived handoff. The gateway stores the canonical
server-resolved app, one-time state hash, exact callback host, and validated
safe relative request URI. The request URI preserves the path and raw query.
Absolute URLs, protocol-relative paths, control characters, backslashes,
cross-host targets, malformed encodings, and `/_tinker/*` return targets deny or
fall back to `/`. URL fragments are browser-only and are not guaranteed across
authentication.

The browser navigates only to the exact admin identity broker. An existing
global identity can authorize the handoff without another OTP. Otherwise the
admin host completes the same global OTP flow used by dashboard login. The
current app policy is checked before authorization and again when the exact app
callback atomically consumes the handoff. The app then issues one opaque
host-only `__Host-tinker_app` child session for that server-derived app and
redirects to the stored safe path and query.

The admin identity cookie is never sent to or accepted by an app origin. An app
child session is never sent to or accepted by the admin origin or a sibling
app. Protected HTTP, SDK, blob, database, and WebSocket requests continue to
recheck the current app and policy.

## Logout and account switching

`POST /_tinker/auth/logout` on an app host revokes only that app's current child
session and visibly means “Sign out of this app.”

`POST /logout` on `admin.<domain>` is “Sign out of Tinkercloud.” After exact
Origin/CSRF validation it revokes the presented global identity family and
every derived child app session in one durable operation, closes affected live
connections after commit, clears the admin identity and CSRF cookies, and
redirects to the admin login page. It never revokes CLI or agent bearers.
Persistence failure preserves cookies and reports a safe retry state rather
than claiming logout.

An explicit “Use another email” flow follows the same family-wide revocation
rule before issuing the replacement identity. App-local logout never changes
the global identity or sibling app sessions.

## DNS and TLS

Setup asks only for the root domain and derives the admin and app hosts. The
operator creates one wildcard DNS record `*.<domain>` pointing at the VPS.
Wildcard DNS and TLS are independent: Tinkercloud must prove a trusted certificate
for the exact admin host and every activated app host, or prove equivalent
wildcard certificate coverage. DNS readiness alone is not certificate
readiness.

## Deny-path charter

Executable evidence must prove:

1. Anonymous, invalid, expired, revoked, replayed, browser-binding-only,
   app-session, and bearer credentials cannot receive dashboard data.
2. An authenticated viewer without a current active operator/deployer role is
   denied the dashboard; role suspension takes effect on the next request while
   independently allowed app access can remain.
3. A dashboard-authorized identity receives no app access unless the resolved
   app's current policy allows it.
4. The admin cookie is absent from app requests; App A's cookie cannot
   authenticate App B or admin; domain cookies and duplicate cookie names cannot
   override the `__Host-` credentials.
5. Guessed, expired, replayed, consumed, wrong-app, state-mismatched, or
   policy-revoked handoffs issue no app session and expose no app bytes.
6. Exact Origin and CSRF checks reject every sibling-app mutation attempt.
   CORS never turns an app origin into an admin credential transport.
7. Safe path and query survive one successful OTP/handoff round trip.
   Absolute/cross-host/reserved/malformed returns deny; fragments are explicitly
   outside the guarantee.
8. App-local logout leaves the global identity and sibling app sessions usable.
   Global logout/switch invalidates dashboard access and all child sessions,
   closes live connections, and leaves CLI tokens usable.
9. Reserved, mixed-case, nested, unknown, and malformed hosts deny before
   persistence or release dispatch. No reserved label can be created by API,
   CLI wizard, manifest, migration, or direct service call.
10. Old dashboard OTP routes, control cookies, and control-session rows cannot
    mint, preserve, or authenticate a parallel browser credential.
11. Missing DNS, wrong wildcard target, missing admin certificate, or missing
    app certificate fails setup/activation without exposing app content.

## Required E2E evidence

The live VPS suite must initialize from one root domain; authenticate one
browser identity at the admin host; open the authorized dashboard when that
identity has a current role; deploy an app; preserve a non-root path and query
through first OTP; open a second allowed app without another OTP; deny an
excluded app; prove app-local logout remains local; prove global logout revokes
dashboard and every child app session; prove a later login restores only
currently authorized surfaces; and prove the identity cookie never reaches an
app request. CLI login/logout and deployment tokens remain independently
usable throughout.
