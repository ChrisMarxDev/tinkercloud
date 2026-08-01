# Control authentication deny charter

The browser has one global identity credential on the exact admin host. It
proves a normalized email only. The dashboard rechecks the current active
operator/deployer role for that identity on every request; app hosts separately
recheck the current policy before issuing or accepting an app-local child
session. There is no dashboard-specific browser OTP channel, control cookie, or
`sessions.scope='control'` credential.

CLI and deployment-agent login remain a separate OTP channel. Successful CLI
verification creates an `api_tokens` credential accepted only through
`Authorization: Bearer`. A browser identity or app session cannot become a
bearer, and a bearer is never accepted from a browser cookie. Authority is
issued only to a normalized active authorized deployer and every use rechecks
active status, expiry, revocation, scope, and resource ownership.

The interactive deployer CLI persists its bearer only in one Tinkercloud-owned
per-user credential file per normalized HTTPS platform server URL:
`os.UserConfigDir()/tinker/<sha256(normalized-server)>.json`. The
platform-specific Tinker configuration directory is mode `0700`; the credential
file is a regular, non-symlinked mode-`0600` file. Creation and replacement
reject symlinks and use an atomic same-directory replacement, so a partial
write can never be treated as a credential. The file is bounded, versioned JSON
with exactly the `version`, `server`, and `token` members; unknown or duplicate
JSON members, a stored-server mismatch, malformed input, and oversized input
deny rather than being silently repaired. The bearer is never supplied in argv
or environment variables and is never emitted in CLI output or logs. This local
persistence is only for CLI bearer reuse; it is neither a browser identity nor
an app credential.

`tinker login` first loads that exact server-bound credential and calls the
authenticated `GET /api/v1/whoami` endpoint. A complete, authorized response
reuses the existing bearer without requesting an OTP, issuing a new token, or
rewriting the credential file; the CLI confirms the server-derived identity.
An unauthorized or expired bearer is not reusable and falls back to the normal
CLI-channel OTP flow. Transport, dependency, malformed-response, unexpected
status, and ambiguous authorization failures fail closed: they neither start an
OTP request nor change the stored credential. `tinker login --force` is the
explicit account-switch path. It skips reuse, completes a fresh OTP, and
replaces the stored bearer only after a successful authenticated `whoami`
confirmation for the newly issued token; failure leaves the prior credential
intact. A `429 rate_limited` response is safe to report as a bounded retry
instruction, but never identifies an email address, authorization state, or
whether a challenge was created.

The CLI OTP request returns the same accepted transaction shape for syntactic
input whether or not the address is an active deployer. If the server cannot
create the durable CLI challenge, it returns only generic `503
temporarily_unavailable`; it must not invent a `login_` fallback transaction
that verification can never consume. Server logs may record exactly one fixed
failure category—`cli_otp_issuance_entropy` or one of
`cli_otp_issuance_persistence_begin_write_lock`,
`cli_otp_issuance_persistence_invalidate`,
`cli_otp_issuance_persistence_eligibility`,
`cli_otp_issuance_persistence_insert`, or
`cli_otp_issuance_persistence_commit`—with no email, transaction, provider
response, database error, path, or credential value.

After any successful `tinker login`—whether it reused a validated bearer or
completed fresh OTP—the CLI persists the normalized HTTPS platform URL as a
separate non-secret default in `os.UserConfigDir()/tinker/default-server.json`.
It uses exact bounded versioned JSON containing only `version` and `server`,
the same non-symlinked mode-`0700` directory and mode-`0600` regular-file
checks as credentials, atomic replacement, and directory fsync. A later CLI
command may omit `--server` only by resolving this exact normalized value.
An explicit `--server` wins for one invocation and cannot update the default.
For a recognized human command with no explicit server, exactly missing default
state starts one bounded `Server (https://...):` setup prompt. The proposed URL
must normalize as HTTPS and pass a direct no-redirect bounded `GET
/api/v1/version` proof with exactly API version 1 before it is stored. This is
platform selection, not authentication: after durable save the original command
continues and may still report `Login required`. `--json` never prompts or
stores; it retains the deterministic missing-server error. Malformed,
oversized, non-HTTPS, unknown/duplicate-field, unsafe, incompatible, redirect,
transport, 5xx, malformed-proof, and storage-failure state fails closed without
running the target command or altering the default.

`POST /api/v1/auth/logout` accepts only a currently authenticated global CLI
bearer. The route derives both deployer and bearer-row ID from authentication,
then atomically revokes exactly that bearer and records a safe audit event. It
has no token ID, app ID, cookie, or body input, and cannot revoke browser
identity families, app sessions, app-scoped bearer tokens, or any other CLI
bearer. A CLI `tinker logout` deletes only the matching local
server-bound credential after this success. `401 not_authorized` means the
bearer is already unusable and permits that same local cleanup; transport,
5xx, persistence, malformed-response, and other ambiguous failures retain the
local credential. Missing local credential is idempotent; corrupt or unsafe
local credential state is not.

Successful bearer-token authentication records only the token's nullable UTC
`last_used_at` timestamp. Authentication performs the active-user, token
revocation, expiry, app-binding, and exact-scope checks together with that
write; any failed check or persistence failure is a denial and leaves metadata
unchanged. Token list and dashboard read models may return `last_used_at` as a
nullable timestamp, but never raw credentials, credential hashes, IP addresses,
or user-agent metadata.

The deployer data-control routes use the same bearer authentication boundary,
but do not treat a passing control credential as sufficient. They require exact
`data:read` or `data:write` scope, current ownership of the route slug, and any
token app binding before the server derives the immutable app ID and creates a
typed deployer-data authorization context. A global browser identity cookie,
app viewer cookie, or generic matching email address cannot be converted into
this bearer authority. Deployment-agent tokens receive
no data scope by default. `data:read` may inspect an owned active/suspended app;
`data:write` is active-app only; deleting/deleted/unavailable apps deny both.
The control database must durably record metadata-only audit intent before an
app-data write begins; intent failure denies the write. L1–L3 make no
cross-database transaction, post-write outcome, reconciliation, or rollback
claim after the separate app-local SQLite mutation.

CLI and browser identity OTP verification use the configured bounded attempt
limit (default five). Each incorrect code durably increments the challenge
attempt count before the generic denial is returned; reaching the limit denies
later correct codes. Successful verification consumes the challenge and
creates its credential in one transaction, so concurrent submissions can issue
at most one credential.

## Admin-host HTML UI

The exact `admin.<domain>` host exposes a server-rendered login and dashboard at
`/login` and `/`. The form OTP flow has the same generic delivery response for
every syntactically valid email. Successful verification sets only the
host-only `__Host-tinker_identity` cookie (`Secure`, `HttpOnly`, `SameSite=Lax`,
`Path=/`, no `Domain`). The same identity can start app-bound handoffs without
another OTP, but it receives dashboard data only after a current role check.

The dashboard uses only server-derived control authority and bounded safe read
models. A deployer receives only a compact list of owned app status and safe
current summary metadata; policy, token, release-management, suspension,
deletion, provider, audit, and host-management controls remain in the scoped
CLI. Operators additionally receive the current private policy revision plus
its canonical email/domain allowlist, release metadata, token metadata,
deployer, and audit metadata. It never renders a raw token, token hash,
provider credential, secret reference, or release filesystem path.
Browser mutations require an exact HTTPS `Origin` check plus a fresh
double-submit CSRF token bound to the global identity family. Sibling app
origins are cross-origin and denied even though they are same-site. The V1
dashboard calls the same typed, audited control-service
boundary as the CLI/API; it does not proxy browser credentials to the bearer
API. Operators may authorize, suspend, and revoke deployers. The deployer
overview does not render management forms; deployers use the scoped CLI to
replace a policy, create/revoke a token, suspend/resume, and delete an owned
app. Operators may suspend/resume any app. App deletion requires the exact
form confirmation `delete:{slug}`.

An access-policy replacement carries the positive `expected_revision` rendered
from that app's current server-derived policy. The service compares it in the
same transaction that writes the next revision. A mismatch is a conflict: it
writes neither a policy/audit event nor a live revocation. If replacement adds
an email or domain beyond the current policy, `confirm_broadening: true` is
also required; the dashboard presents an explicit confirmation and a safe
post-success summary.

`tinker access set APP --file POLICY.json` first reads the caller's current
server-derived policy. It uses that read only to fill a missing
`expected_revision` and to identify additions in the requested complete policy;
it never computes authorization locally. When the requested revision still
matches and it adds an exact email or domain, an interactive invocation asks
one explicit `Continue? [y/N]` confirmation before its PUT and writes
`confirm_broadening: true` only after an affirmative answer. `--json` and
non-interactive automation never prompt: the policy file must contain the
explicit boolean `confirm_broadening: true` or the CLI returns the stable
`confirmation_required` result without sending a mutation. A stale revision
is sent unchanged and remains the server's `conflict` result. The server
continues to enforce confirmation inside its replacement transaction; a CLI
cannot turn a broadened write into an authorized one.

### UI deny charter

- An anonymous or invalid platform cookie redirects only to login and reveals
  no dashboard data.
- A deployer cannot receive operator-only deployer or audit read models.
- A missing, cross-origin, or mismatched CSRF form cannot change dashboard or
  global identity state.
- Dashboard “Sign out of Tinkercloud” succeeds only after it durably revokes the
  presented identity family and every derived app session. Missing revocation
  wiring or persistence failure leaves cookies intact, returns a safe server
  failure, and cannot revoke a CLI/agent bearer. App-local logout revokes only
  the current app session.
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
  replace the current revision with the selected immutable `tinker.yaml` policy.
- Login, dashboard, and error rendering never disclose OTPs, bearer values,
  token hashes, provider values, or filesystem paths.
- Incorrect OTP submissions consume the configured attempt budget even though
  their response is a generic denial; a later correct code cannot bypass an
  exhausted challenge.
- A browser identity challenge cannot mint a CLI bearer, and a CLI challenge
  cannot mint a browser identity, even with the correct code.
- A global identity or app cookie presented as a bearer, or a CLI bearer
  presented as a browser cookie, is denied without recording successful-use
  metadata.
- A global identity can access the dashboard only through a current
  operator/deployer role lookup. It cannot mint a CLI bearer, select an app, or
  turn matching email text into authority.
- A reusable CLI bearer is accepted only after a complete authenticated
  `whoami` response for its normalized server. A malformed response,
  dependency failure, unexpected status, or ambiguous authorization result
  cannot prompt for OTP or replace the old credential.
- An unauthorized or expired CLI bearer may fall back to one CLI-channel OTP;
  a forced login always requires that fresh OTP. Neither path replaces an
  existing credential until the newly issued bearer has passed `whoami`.
- A rate-limited CLI login response is actionable but non-enumerating. It
  reports only a retry instruction and never exposes email, deployer status,
  token existence, or OTP-challenge state.
- Missing default-server state may offer only the bounded human setup prompt;
  corrupt, unsafe, non-HTTPS, or ambiguous default-server state cannot select a
  host or start a network request.
- Only an exact missing default can open the bounded human setup prompt. JSON,
  explicit-server, invalid grammar, version, corrupt configuration, invalid
  input, incompatible/redirected/failed proof, and failed secure storage never
  prompt, cache, or execute the requested command.
- Logout denies anonymous, browser-cookie, viewer-session, app-scoped, wrong
  scope, malformed-route, and revoked-bearer requests. A successful logout
  revokes exactly the authenticated global bearer; another bearer remains
  usable. Persistence failure never produces a successful logout response.
- Data-control routes deny anonymous, browser/viewer cross-presentation,
  wrong owner, wrong app binding, missing/wrong/revoked scope, malformed data
  selector, suspended write, and deleting/unavailable app requests before an
  app database is opened. A data write with stale version, pre-write audit
  intent failure, or app-data persistence failure has no partial data change.
  The current audit row proves only the pre-write intent and is never
  misreported as an atomic cross-database outcome or rollback.

## Owned application lifecycle

`GET /api/v1/apps/{slug}/releases` returns a bounded metadata-only list of the
caller's releases for an active or suspended app. It never exposes archive or
release-file paths. A caller who does not own the slug receives the same
authorization denial as an unknown slug.

`DELETE /api/v1/apps/{slug}` requires an `app:delete` control scope, an
idempotency key, and the JSON confirmation value `delete:{slug}`. The service
derives both app and owner from the bearer credential and route; it does not
accept an app ID from JSON. A successful deletion permanently removes the app,
its policies, deployments/releases, tokens, viewer sessions, KV entries,
blobs, and app-specific audit metadata. It also removes the corresponding
server-derived release/blob bytes before reporting success. A transient
`deleting` state may fail closed during the operation, but no `deleted`
tombstone, restore record, or app-specific audit record remains after success.

### Deny charter

- Anonymous, malformed, wrong-scope, cross-owner, and unknown-app requests do
  not disclose release metadata or mutate state.
- Missing, mismatched, or malformed confirmation values and missing idempotency
  keys do not start deletion.
- Any database or owned-file removal failure denies completion, leaves the app
  unavailable while cleanup is incomplete, and never reports successful
  deletion prematurely.
- A retry may continue only the same server-derived deletion operation; after
  successful removal the slug has no retained deletion record and behaves as
  an unknown app.
