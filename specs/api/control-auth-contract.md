# Control authentication deny charter

Control browser sessions, CLI bearer tokens, app viewer sessions, and global
viewer identity sessions are four different persisted credential types. A control OTP challenge is server-bound
to either the `browser` or `cli` channel before it is sent; verification accepts
only that intended channel and atomically creates the matching credential type.
Browser verification creates a `sessions.scope='control'` credential used only
by the host-only control cookie. CLI verification creates an `api_tokens`
credential used only through `Authorization: Bearer`. Authority is issued only
to a normalized active authorized user and every use rechecks active status,
expiry and revocation. App-view permission never grants control authority;
browser control credentials are never accepted as CLI/API bearers and CLI
bearers are never accepted from browser cookies. A global viewer identity is
accepted only by the platform identity broker to issue a server-created,
app-bound viewer handoff; it is never dashboard/control authority.

Successful bearer-token authentication records only the token's nullable UTC
`last_used_at` timestamp. Authentication performs the active-user, token
revocation, expiry, app-binding, and exact-scope checks together with that
write; any failed check or persistence failure is a denial and leaves metadata
unchanged. Token list and dashboard read models may return `last_used_at` as a
nullable timestamp, but never raw credentials, credential hashes, IP addresses,
or user-agent metadata.

Control OTP verification uses the configured bounded attempt limit (default
five). Each incorrect code durably increments the challenge attempt count before
the generic denial is returned; reaching the limit denies later correct codes.
Successful verification consumes the challenge and creates its credential in
one transaction, so concurrent submissions can issue at most one credential.

## Platform-host HTML UI

The platform host exposes a server-rendered login and dashboard at `/login` and
`/`. The form OTP flow has the same generic delivery response for every valid
email. Successful verification sets only the host-only `__Host-tiny_control`
cookie (`Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`); it is distinct from
the app-viewer cookie and is not accepted by CLI/API bearer authentication.
It is also distinct from host-only `__Host-tiny_identity`, which proves a
viewer email only and cannot access dashboard/control read models.

The dashboard uses only server-derived control authority and bounded safe read
models: owned app status, the current private policy revision plus its
canonical email/domain allowlist, release metadata, token metadata,
and, for operators, deployer and audit metadata. It never renders a raw token,
token hash, provider credential, secret reference, or release filesystem path.
Browser mutations require an exact same-origin check plus a fresh double-submit
CSRF token. The V1 dashboard calls the same typed, audited control-service
boundary as the CLI/API; it does not proxy browser credentials to the bearer
API. Operators may authorize, suspend, and revoke deployers. Deployer-owned
app forms show the current policy's canonical additional viewers and may replace
that policy, create a scoped token (shown only in
the create response), revoke a token, roll back, suspend/resume, and delete an
app. Operators may suspend/resume any app. App deletion requires the exact
form confirmation `delete:{slug}`.

An access-policy replacement carries the positive `expected_revision` rendered
from that app's current server-derived policy. The service compares it in the
same transaction that writes the next revision. A mismatch is a conflict: it
writes neither a policy/audit event nor a live revocation. If replacement adds
an email or domain beyond the current policy, `confirm_broadening: true` is
also required; the dashboard presents an explicit confirmation and a safe
post-success summary.

### UI deny charter

- An anonymous or invalid platform cookie redirects only to login and reveals
  no dashboard data.
- A deployer cannot receive operator-only deployer or audit read models.
- A missing, cross-origin, or mismatched CSRF form cannot change browser
  control-session state.
- A deployer cannot invoke an operator deployer-status form; ownership and
  active role are rechecked in the service transaction, not trusted from HTML.
- A missing or mismatched app deletion confirmation cannot change app status.
- A newly created raw token is rendered only in its one create response and is
  never included in the dashboard read model or audit metadata.
- A dashboard policy view derives its app ownership, active revision, and
  allowlist from the server. Missing, non-private, malformed, cross-owner, or
  stale policy state is unavailable rather than rendered as an empty allowlist.
- The owner is always an implicit viewer. An empty displayed allowlist means
  owner-only, never public; the dashboard explains that a future activation can
  replace the current revision with the selected immutable `tiny.yaml` policy.
- Login, dashboard, and error rendering never disclose OTPs, bearer values,
  token hashes, provider values, or filesystem paths.
- Incorrect OTP submissions consume the configured attempt budget even though
  their response is a generic denial; a later correct code cannot bypass an
  exhausted challenge.
- A browser-channel challenge cannot mint a CLI bearer, and a CLI-channel
  challenge cannot mint a browser control session, even with the correct code.
- A control cookie presented as a bearer token, a CLI bearer presented as a
  control cookie, or an app viewer session presented on either control surface
  is denied without recording successful-use metadata.
- A global viewer identity cookie is not a control session, cannot access the
  dashboard, cannot mint a CLI bearer, and cannot select an app or user role.

## Owned application lifecycle

`GET /api/v1/apps/{slug}/releases` returns a bounded metadata-only list of the
caller's releases for an active or suspended app. It never exposes archive or
release-file paths. A caller who does not own the slug receives the same
authorization denial as an unknown slug.

`DELETE /api/v1/apps/{slug}` requires an `app:delete` control scope, an
idempotency key, and the JSON confirmation value `delete:{slug}`. The service
derives both app and owner from the bearer credential and route; it does not
accept an app ID from JSON. A successful deletion revokes every app-scoped
control token and viewer session in the same transaction, writes one
`app.deletion_requested` audit event, and makes the app unavailable before the
response. It records `active|suspended -> deleting -> deleted` inside that
transaction. Release files are retained for asynchronous cleanup and are never
deleted from the request path.

### Deny charter

- Anonymous, malformed, wrong-scope, cross-owner, and unknown-app requests do
  not disclose release metadata or mutate state.
- Missing, mismatched, or malformed confirmation values and missing idempotency
  keys do not start deletion.
- Audit insertion failure rolls back status and every revocation.
- Reusing an idempotency key succeeds only for the same owner and app target.
