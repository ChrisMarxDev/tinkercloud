# Global viewer identity and app-bound handoff contract

**Status:** V1 contract

## Purpose and trust boundary

TinyHost authenticates one viewer email identity for a browser profile on the
platform host, then authorizes that identity independently for each resolved
app. Authentication answers “which email identity is this?”; app policy answers
“may that identity access this server-derived app now?” Neither answer grants
control-plane or CLI authority.

The gateway derives the app exclusively from the canonical app hostname. It
derives the identity exclusively from an opaque platform-host identity session
or from an opaque app-local session. Browser parameters, headers, app code, and
the SDK never select an app ID, identity ID, user role, or handoff target.

## Credentials and persistence

`identity_sessions` are opaque, server-side viewer-identity records. Their
secrets are stored only as hashes and each belongs to one `identity_id` and one
server-generated `family_id`. The browser receives only the host-only
`__Host-tiny_identity` cookie on the configured platform host; it is `Secure`,
`HttpOnly`, `SameSite=Lax`, `Path=/`, and has no `Domain` attribute. It is never
a control credential, bearer, JWT, local-storage value, or app-host cookie.

The platform also keeps a separate opaque `__Host-tiny_browser` browser-binding
cookie. It is `Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`, host-only, and
has a bounded 30-day lifetime, but it has **no authentication or authorization
authority**: it is not accepted as an identity, control, app, or CLI
credential. The value is generated with cryptographic randomness only by the
platform identity flow and is never put in a URL, HTML form, template,
JavaScript value, app-host request, or log. It is passed only server-side to
the identity OTP request/verification persistence operations, which store a
hash. A handoff’s hash is nullable only until its OTP request; every issued
identity session has one. Missing or mismatched bindings fail with the same
generic OTP result and cannot consume or authorize a handoff. One unrevoked
family may exist per binding: it groups simultaneous OTP completions in one
browser profile so the first valid completion wins, while an expired
replacement revokes that family’s descendants and pending grants. Valid global
identity use remains authorized by the identity credential, not by this
binding. Global logout and known-invalid identity-cookie cleanup retain the
binding so later login/switch attempts remain in the same browser profile.

Global logout first revokes by a presented global identity credential. If that
credential is absent or known-invalid, it may use the binding hash only as a
server-side revocation-family lookup; the binding still never authenticates a
request or grants access. A real persistence error from either selected
revocation path returns the generic 5xx retry page, preserves browser cookies,
and does not claim logout success. Confirmed child-session references are
deduplicated and their live connections close only after the matching
revocation transaction commits. A request with neither recognized platform
credential receives the generic retry outcome rather than a successful global
logout.

An identity session has a 30-day absolute lifetime and rotates on eligible use
after 24 hours. Rotation issues a fresh secret in the same family. The immediately
previous secret may overlap for no more than 60 seconds to tolerate concurrent
tabs; use after that overlap, any revoked/expired token, malformed token, or
persistence ambiguity denies. Reuse evidence outside the overlap revokes the
affected identity-session family and its child app sessions before denial.

At the platform identity page, a known missing, expired, revoked, malformed, or
replay-revoked global credential is expired from the platform host and may
continue to the generic re-authentication form. A persistence/dependency
failure is not evidence that the cookie is invalid: it returns only the
generic no-store retry page with a 5xx response, preserves the cookie, and
does not issue an OTP or mutate a handoff.

`sessions.scope='app'` remains an opaque host-only `__Host-…` credential bound
to exactly one app and identity. It is the only viewer cookie an app hostname
receives. Every protected HTTP request and WebSocket upgrade still resolves the
current app policy and denies before app bytes or capability dispatch when the
policy no longer permits that identity.

The V4-to-V5 migration is additive: it adds identity-session tables and the
optional child-session link but never rewrites `sessions.revoked_at`. The new
runtime quarantines a parentless `scope='app'` session when its `created_at`
predates migration 5's persisted `schema_migrations.applied_at` cutoff; TinyHost
cannot safely invent a parent identity family for that legacy credential. A
parent-linked session remains valid under its normal checks. A parentless
session created at or after the cutoff retains its explicitly configured
brokerless compatibility semantics. Missing, malformed, or otherwise
unparseable cutoff/session timestamps deny access without mutation. The
corrected runtime retains the one historical destructive-v5 checksum for this
corrected additive migration so a binary rollback can reopen the database, but
never executes its old SQL again.

One browser profile has one active global viewer identity. A successful global
identity switch revokes the prior identity-session family and every child app
session before issuing the replacement global identity session. Per-app logout
remains local: it revokes only the current app session and does not destroy the
global identity session.

## Handoff protocol

When an app-host document request lacks a valid local app session, the gateway
creates a server-side, opaque handoff transaction from the already resolved app
and a validated safe relative return path. It redirects only to the exact
configured platform-host identity route. The random opaque handoff ID is the
one-time transaction reference; its corresponding random state value stays in
a host-only app cookie and is stored only as a hash. The transaction stores its
server-derived app ID, safe return path, creation/expiry timestamps, and no
client-selected destination.

The platform host validates its own global identity cookie. If no valid global
identity exists, its OTP flow creates one only after normal generic,
non-enumerating verification. It then rechecks the current policy for the
handoff’s stored app and marks the handoff authorized for that server-derived
identity. The browser returns only to the exact app-host callback represented
by the transaction. The app-host callback atomically consumes that handoff,
verifies state, expiry, app binding, authorization, and current policy again,
then creates the app-local host-only session and redirects to the stored safe
relative return path.

Handoffs expire in five minutes or less, are consumed once, and are never
reused as a session or refresh token. Every redirect target is server-derived
or a validated safe relative path; absolute URLs, protocol-relative paths,
cross-host paths, control paths, and malformed encodings deny or fall back to
the app root without revealing policy membership.

## Credential separation

There are four persisted credential classes:

| Class | Transport | Authority |
|---|---|---|
| Global viewer identity | platform host-only cookie | Email identity only; no app/control privilege |
| App viewer session | app host-only cookie | One app after current-policy evaluation |
| Browser control session | platform host-only control cookie | Operator/deployer dashboard only |
| CLI token | `Authorization: Bearer` | Scoped deployment/control API only |
| Browser binding | platform host-only cookie | OTP race grouping only; no credential authority |

No class is accepted in another class’s transport or validation path. In
particular, the global identity cookie never authenticates dashboard/control
routes directly, control cookies never complete a viewer handoff, and CLI
bearers never authenticate browser or app routes.

## Deny-path charter

Tests and integration evidence must prove all of the following before this
surface is complete:

1. A guessed, replayed, consumed, wrong-app, expired, malformed, unauthorized,
   or state-mismatched handoff denies without issuing an app session or serving
   release, blob, KV, SDK, or WebSocket bytes.
2. A handoff may not exchange on a sibling app, a platform/control hostname, or
   a client-provided app identifier. A valid identity for App A does not grant
   App B when App B policy denies it.
3. Missing, expired, revoked, rotated-out, or replayed global credentials deny;
   rotation overlap works only for the prior token and no longer than 60
   seconds. A replay detected after overlap revokes the family and child app
   sessions.
4. App, control, identity, and CLI credentials are mutually rejected across
   their routes and transports. Viewer identity never becomes deployer or
   operator authority solely because emails match.
5. Current policy is evaluated when issuing a grant, consuming a grant, and on
   every protected app request. Revocation, app suspension/deletion, and global
   sign-out close matching WebSockets and deny their next HTTP/capability
   request.
6. Per-app logout cannot revoke or reveal a global identity; global switch or
   global logout cannot leave old child app sessions usable.
7. Every redirect is exact server-derived platform/app routing plus a validated
   safe relative path. Open redirects, return-path reflection, and app/email
   enumeration are denied; OTP and handoff pages retain generic responses.
8. Pre-auth, identity-broker, rejected callback, and denial paths open no
   release tree and return no app bytes. They disclose neither app policy,
   email membership, credential family state, nor provider detail. A browser
   that has already proved its own valid global identity may see that escaped
   email in a generic denied notice; this does not disclose identity existence
   to an unauthenticated caller.
9. The additive V4-to-V5 migration leaves legacy rows' `revoked_at` unchanged,
   while the new runtime denies pre-cutoff parentless app sessions and retains
   post-cutoff brokerless compatibility sessions. Missing or malformed cutoff
   and session timestamps deny without mutation. Across
   stale or concurrent OTP forms for one platform browser binding, exactly one
   active global family may be issued, regardless of selected email or
   force-login mode; a losing completion creates neither a global session nor
   an authorized handoff. A pre-challenge force switch may revoke and replace
   the old bound family, but a completion at or after the selected challenge
   always wins. A non-force completion must not replace a valid presented
   global identity.
10. The browser-binding cookie is present only on the platform host after the
    initial broker form, never appears in browser-visible state or app-host
    cookies, survives global sign-out, and is rejected as every credential
    class. Missing, guessed, or mismatched bindings cannot send a usable OTP,
    consume a challenge, create a global identity, or authorize a handoff.
    On global logout, it may only recover a missing/known-invalid identity
    family for revocation; it never authenticates the logout request.

## E2E evidence

The unattended real-VPS suite must prove: first viewer OTP creates a global
identity; an allowed second app completes no additional OTP and gets an
app-local session; a denied second app receives the generic denial; a replayed
handoff and a wrong-app callback deny; app-local logout remains local; global
switch invalidates old app sessions; and a global-session restart/rotation still
preserves allowed access without exposing a cookie or app bytes on denial. It
also proves the non-authorizing browser binding remains exact-platform-only
across the initial form and a global account switch.
