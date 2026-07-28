# ADR 0033: Global viewer identity with app-bound handoffs

**Status:** Accepted for V1

## Context

ADR 0002 correctly rejected a shared parent-domain app cookie: deployed apps
are untrusted independent origins and one app must not receive another app’s
credential. Its original app-by-app OTP consequence, however, makes an email
identity feel logged out when moving between allowed TinyHost apps. The
platform needs to authenticate an email once per browser profile while retaining
app-specific authorization and browser cookie isolation.

## Decision

TinyHost introduces one opaque, server-persisted global viewer identity session
on the platform host. The host-only `__Host-tiny_identity` cookie proves only a
viewer email identity; it is distinct from the control-plane cookie and CLI
bearers, grants no deployer/operator authority, and is never sent to app
hosts.

The platform additionally sets `__Host-tiny_browser`, an opaque 30-day,
host-only `Secure`/`HttpOnly`/`SameSite=Lax` browser-profile binding. It is not
an identity, control, app, or CLI credential and has no authorization effect.
It never reaches deployed app hosts or browser-visible URL, form, template,
JavaScript, or logging state. The broker passes it only server-side during OTP
request and verification; persistence stores its hash with the handoff and
newly created identity family (nullable on a handoff only before its OTP
request). One unrevoked family may use a binding, so concurrent completions in
one profile are first-wins; a replacement revokes the prior family’s
descendants and pending grants. Missing/mismatched bindings fail generically.
The binding remains after global logout or a known-invalid identity-cookie
cleanup: it groups the browser profile but cannot authenticate it.

An app with no local session starts a server-created, state-bound, one-time
handoff. The platform host validates the global identity, then the gateway
rechecks current app policy before grant issuance and again while atomically
consuming the grant on the exact app callback. A successful exchange creates
the existing host-only app-local opaque session. The app cookie remains bound
to one app and is the only credential presented to app requests.

When this broker is configured, it is the sole deployed browser viewer-session
issuance route. Legacy direct app-host OTP request/verify endpoints are retired
with generic form/JSON denials; they remain only for an explicit brokerless
local compatibility harness. This prevents newly issued parentless app
sessions from escaping global logout and account-switch revocation. Platform
OTP, verification, account-switch, and logout POSTs require an exact HTTPS
same-origin `Origin` header. Global logout clears the identity cookie and
closes child live connections only after persistence commits; a persistence
failure leaves the cookie and live children untouched and reports no success.
When the global identity cookie is absent or known-invalid, logout may use the
non-authorizing browser-binding hash only to find and revoke that browser
profile's family; it is never an authentication fallback. A real error from
either selected revocation path preserves cookies and reports the generic retry
state rather than a successful logout.
Anonymous app-host handoff creation has its own bounded in-memory abuse limit,
separate from the email OTP budget.

The platform identity page distinguishes a known-invalid global cookie from a
persistence failure. It expires only the known-invalid/replay-revoked cookie
before presenting generic re-authentication; a dependency failure preserves
the cookie, returns a generic 5xx retry page, and cannot advance OTP or
handoff state.

Global sessions last at most 30 days, rotate every 24 hours, and permit the
immediately previous opaque secret for at most 60 seconds. One browser profile
has one active global identity. Switching identity after OTP verification
revokes the previous global session family and every derived app session;
per-app logout only revokes its app-local session.

The normative protocol and failure requirements are
[`specs/api/global-identity-handoff-contract.md`](../../specs/api/global-identity-handoff-contract.md).

## Consequences

- A viewer verifies an email once per browser profile, then sees a fast,
  policy-checked handoff rather than another OTP at each allowed app.
- No parent-domain app cookie, JWT, local-storage token, client-selected app
  identifier, or app-visible global credential is introduced.
- App policy/revocation remains effective on the next request and revokes
  affected WebSockets.
- The database gains identity-session families, one-time handoff state, and
  child-session linkage. The forward-only migration is additive and never
  mutates legacy app-session revocation state. The new runtime quarantines
  parentless app sessions created before migration 5's persisted application
  cutoff rather than guessing a parent identity/family; explicit brokerless
  compatibility sessions created after that cutoff retain app-local semantics.
  Missing or malformed persisted cutoff/session timestamps deny without
  mutation. The corrected additive migration retains the one historical
  destructive-v5 checksum so a binary rollback/update can reopen the database
  without a VPS database mutation. Stale or concurrent non-force OTP completions are
  first-wins: a completion cannot create a second active family after another
  global identity was issued for that identity since its challenge was created.
- Existing control browser sessions and CLI bearer tokens retain their separate
  validation paths. Matching email text does not bridge credential authority.
