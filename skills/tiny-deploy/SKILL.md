---
name: tiny-deploy
description: Deploy and independently verify private TinyHost apps.
---

<!-- shared:security:start -->
## Terminology

Use `operator` for a person who hosts and operates TinyHost, `deployer` for a
person authorized to create and manage their own Tiny apps, and `viewer` for a
person who accesses and interacts with a deployed app. Treat `user` as a
neutral umbrella term for any human; never infer a role, permission, ownership,
or credential type from it. When authority changes the answer or action and the
role is unclear, ask whether `user` means operator, deployer, or viewer. Do not
ask when context already establishes the role. Treat a deployment agent as
non-human automation using a scoped deployer token, not as a user. A person may
act in more than one role, but never transfer authority or credentials between
roles.

## Minimum-necessary human input

Start human workflows from the requested outcome. Inspect current state, reuse
verified values, and choose secure defaults before asking a question. Ask only
for a value or decision that is required, not already known, and impossible to
discover or safely default. Optional settings belong behind one review/edit
step instead of a mandatory questionnaire.

Treat generated config and `tiny.yaml` as reviewable receipts and automation
interfaces, not prerequisite paperwork. Never infer authorization broadening,
put secrets in argv/ordinary config, run an app build, follow an unverified
redirect, or add prompts/implicit writes to JSON and non-interactive commands.
When external DNS, email, or certificate state blocks progress, print one exact
action, persist only validated safe progress, and resume without re-asking prior
valid answers.

## Security boundary

TinyHost's gateway derives app identity from the hostname and viewer identity
from a host-only session. Never add an app ID, viewer ID, database credential,
provider secret, or deployer token to app code or SDK configuration. Protected
dispatchers require the gateway authorization context; malformed or missing
state denies access. V1 live events are best-effort hints, never history,
replay, ordering, or durable state.
Cookie-authenticated mutations and live upgrades require an exact same-origin
Origin; TinyHost emits restrictive same-origin CSP and never wildcard CORS.
Platform dashboard forms additionally require a double-submit CSRF value and
call the typed control service with a server-derived control actor. Never put a
bearer token in a form. Deletion requires `delete:{slug}`; show a newly created
control token only in its one create response, never in a dashboard read model.
The operator active-deployer allowlist is one revision-protected exact set:
validate bounded unique normalized emails, require explicit broadening consent,
deny operator-email collisions, and atomically revoke removed deployers' API
tokens/control sessions and pending OTPs. Reactivation invalidates pending OTPs
before activation; preserve immutable deployer IDs and owned data.
Control browser sessions, CLI bearer tokens, and app viewer sessions are
different server-persisted credential types. Bind each control OTP to its
server-selected browser or CLI channel before delivery; never accept a browser
control cookie as a bearer, a CLI bearer as a control cookie, or either as an
app viewer session. A credential-type separation upgrade revokes every legacy
`api_tokens` row because old values are unclassifiable; require a fresh CLI
login and never preserve a legacy token by guessing its channel.
For `tiny login`, load only the protected credential bound to the normalized
HTTPS server and validate it with authenticated `whoami`. Reuse it without OTP,
token issuance, or file rewrite only after a complete authorized response. An
unauthorized or expired bearer may use fresh CLI-channel OTP; transport,
dependency, malformed-response, unexpected-status, and ambiguous authorization
errors fail closed without requesting OTP or altering the stored credential.
`tiny login --force` is the explicit account-switch path: it always uses fresh
OTP and replaces the stored bearer only after the newly issued bearer passes
`whoami`; failure retains the previous credential. Treat `429 rate_limited` as
an actionable retry instruction, never as evidence of email eligibility,
deployer authority, token existence, or OTP-challenge state.
After any successful login, save the normalized HTTPS platform URL as the
non-secret bounded exact `default-server.json` beside server-bound credentials;
it has the same 0700/0600, no-symlink, atomic-replacement, and fsync boundary.
Commands may omit `--server` only through that exact saved URL. Explicit
`--server` is invocation-only. For a recognized human command, exactly missing
state opens one bounded setup prompt; cache only a normalized HTTPS URL that
passes direct no-redirect API-v1 proof, then continue (which may say Login
required). JSON never prompts or caches; malformed/unsafe/non-HTTPS,
incompatible, redirected, transport-failed, or storage-failed setup fails
closed. `tiny logout` is real bearer self-revocation: it calls the
authenticated control route, then removes only that server's local credential
after success or known unauthorized state. It retains the credential on any
transport, server, persistence, or local-delete ambiguity; it never revokes a
browser control session, viewer session, app-scoped token, or another bearer.
Dashboard access views render only the server-derived current canonical private
policy. Treat missing, non-private, malformed, cross-owner, or stale policy
state as unavailable, never as an empty allowlist. The owner is implicit; an
empty valid allowlist is owner-only, and the next activation may replace it
with the immutable selected release's `tiny.yaml` policy.
Access replacement carries the current server-rendered revision and compares it
inside the write transaction. A revision conflict creates no policy/audit/live
revocation; refresh instead of overwriting an activation or newer policy state.
Adding an email or domain requires explicit broadening
confirmation.
The default live boundary is 32 KiB frames/payloads, 20 publishes per second
per connection, and a bounded outbound queue: slow consumers are disconnected,
never allowed to block other viewers. Authenticated live sockets also have a
60-second idle bound, 20-second ping interval, 10-second pong extension, and
3-second write deadline by default; a silent peer is disconnected and a
successful pong keeps a healthy socket alive. Revocation closes matching live
sockets immediately rather than waiting for that liveness bound.
Viewer OTP requests (JSON or form) expose only an opaque transaction and the
same generic accepted shape. Keep form fields bounded and escaped. One global
viewer identity per browser profile is an opaque server-side platform-host
cookie, never a control credential, parent-domain cookie, JWT, local-storage
value, app-visible credential, or client-selected identity/app. It lasts at
most 30 days, rotates every 24 hours, and accepts its prior token for no more
than 60 seconds. A missing local app session uses only a server-created,
state-bound, app-bound, five-minute-or-less one-time handoff through the exact
platform host; recheck current app policy when issuing and consuming it, then
create the existing host-only app cookie. Guessed, replayed, wrong-app,
expired, or state-mismatched handoffs deny without app bytes. App logout stays
local; global identity switch happens only on platform-host POST verification
and revokes the old identity family plus every child app session. Recheck
policy on every protected request and close matching live sockets on app,
policy, session, or global-family revocation. Never accept global identity,
control cookies, app cookies, or CLI bearers in one another's routes.
The platform may also issue a host-only `__Host-tiny_browser` binding with a
30-day bounded lifetime. It is a cryptographically random, `Secure`,
`HttpOnly`, `SameSite=Lax` OTP race-grouping value only: never treat it as an
identity/control/app/CLI credential; never send it to an app host or expose it
in URLs, forms, templates, JavaScript, or logs. Pass it only server-side to
OTP request/verification persistence, which stores hashes and permits one
unrevoked identity family per binding so concurrent completions are first-wins.
Missing/mismatched bindings deny generically. Retain the binding through global
logout and known-invalid identity cleanup; it does not mean the browser is
signed in.
For global logout, revoke a valid global identity first. Only when it is absent
or known-invalid may the binding hash locate a family for revocation; it never
authenticates the request. A real selected-revocation error preserves cookies
and returns generic retry rather than claiming logout success.
Global-identity migration is additive: never rewrite legacy app-session
`revoked_at`. The runtime quarantines a parentless app session only when its
RFC3339 `created_at` predates migration 5's persisted `applied_at` cutoff;
parent-linked sessions validate normally and explicitly brokerless sessions
created at/after that cutoff retain app-local semantics. Missing or malformed
cutoff/session timestamps deny without mutation. Retain the known historical
v5 checksum for the corrected additive migration so rollback binaries can
reopen the database; never alter a VPS migration row to bypass it.
At the disk write-stop watermark, expect app creation, deployment, and KV
mutation and blob upload to fail safely; static/blob reads, deletion when safe,
and revocations must still work. Cleanup is server-side, database-led, and never
a reason to expose release or blob paths.
V1 lightweight blobs are an opt-in app-shared capability with the same viewer
authority model as KV. Use only opaque server-issued blob IDs: filenames are
bounded display metadata, never paths or storage keys. Uploads stream into
unreachable private staging and become readable only after exact byte evidence
and SQLite metadata reach `ready`; uncertain, staging, deleting, orphaned, or
metadata/storage-disagreeing state denies. Downloads stay behind the
authenticated gateway with attachment, no-sniff, and private/no-store behavior.
Never add a public/signed URL, app selector, bucket, mount, provider endpoint,
or credential to the SDK. V1 uses the local adapter; FUSE/rclone/s3fs/
Mountpoint, a standalone object-store server, and remote drivers remain out of
scope. Implement the V1 byte adapter as small Go standard-library code inside
`tinyhost`; do not add Go CDK, another runtime package, process, service,
listener, mount, provider credential, or storage-network dependency. Before
implementing or reporting blobs complete, follow
`specs/capabilities/blob-contract.md`, run the two-app and partial-write failure
matrix, update `features.blobs`, the SDK/examples, and every copied skill.
Archive uploads use the configured archive byte limit, independently of the
small JSON control-body limit. Keep deployment bundles streaming through a
mode-0600 temporary file; use a fresh random idempotency key per deploy
invocation and reuse it only for retries of that same invocation.
For local onboarding, `tiny init [DIR]` creates a strict deterministic V1
`tiny.yaml` without overwriting; `tiny deploy [DIR]` defaults to `.` and may
run the same bounded human wizard only when that manifest is missing. Inspect
the project first: use a valid directory-derived slug and one unambiguous
conventional output without asking; prompt only when either required value is
invalid or ambiguous. With no valid output, stop with the exact project-owned
build action; never select an arbitrary directory or run the build. Reuse an
existing same-owner manifest slug automatically; keep unavailable-name errors
generic. Default to owner-only and no capability unless an existing manifest or
deliberate review edit enables it; build inspection may warn but never grant
authority. Put
description, combined email/domain allowlist, capabilities, and SPA fallback
behind one review/edit step, and name access broadening in the same final deploy
action. Write a missing manifest as a consequence of that action; never add a
separate manifest confirmation. JSON mode must never
prompt, create a manifest, request OTP, or persist a credential. Before a human
deploy uses a saved bearer, prove it with API-version plus `whoami`; only a
definite unauthorized result may use OTP, and store only a freshly verified
token. Reject traversal, symlinks in any project/output/fallback component, and
non-regular fallback files before archive creation. `--server` is invocation
local and never changes the saved default.
Before activation, certificate readiness may retry only its exact app-origin
HTTPS proof: use the server's finite context-cancellable 45-second budget,
finite attempts, no redirects, exact host, verified TLS chain, and the normal
non-redirect readiness statuses. Redirect, wrong-host, unverified TLS,
cancellation, or budget exhaustion denies activation and preserves the previous
release. Never use this retry for policy, ownership, archive validation,
anonymous-denial, or post-activation checks.
`tiny deploy` may create a missing manifest-named app only after the server's
ownership-scoped app list confirms it is not already owned; a slug conflict is
never success. The validated manifest allowlist is immutable candidate metadata:
activation atomically installs the selected release pointer and its exact
canonical private policy. Never gate a candidate on an older current
policy or send an owner/app identity from browser code.
CLI deployment success requires its own fresh anonymous GET through the real
HTTP/TLS transport to the activated protected app URL. The authenticated
activation response supplies the server-derived app suffix, so validate exactly
`https://{slug}.{app_suffix}/`; do not infer that suffix from the control host.
Never send the deployer bearer token or cookies to that app origin. Accept only
the gateway's bounded `401` JSON `not_authorized` envelope with matching valid
request ID, `Cache-Control: no-store`, and `X-Content-Type-Options: nosniff`;
404, redirect, 2xx, malformed/oversized evidence, wrong host, TLS/timeout, or
transport failure means deployment verification failed.
For app sign-in UX, redirect only an unambiguous browser document navigation
from a protected static path to that same app host's login route. API, asset,
range, WebSocket, and ambiguous requests must keep the JSON denial. Account
switch is a same-origin POST logout that revokes the current app's host-only
session; never use GET mutation, platform cookies, or an external return URL.
Resolve an app with database metadata only before authorization: pre-auth,
reserved, anonymous, and wrong-session routes must never open or hash a
release tree. Only static dispatch after a sealed authorization context may
verify the active immutable tree against its deployment file evidence, before
returning any app byte; a mismatch denies closed.
For the opt-in external VPS acceptance run, require
`TINYHOST_VPS_E2E=1`, an exact dedicated-host acknowledgement bound to
`TINYHOST_VPS_SSH_TARGET`, and an explicit
`TINYHOST_VPS_KNOWN_HOSTS_FILE`. SSH uses strict pinned host-key checking;
never add `StrictHostKeyChecking=no`, `accept-new`, or a shell-evaluated SSH
target. Prove the separate deployer/viewer path plus anonymous HTML, asset,
API, and socket denial with zero app bytes before claiming VPS readiness.
Host preflight accepts only the explicit Ubuntu 24.04 LTS/amd64 and Ubuntu
26.04 LTS/amd64 allowlist. Never replace it with a numeric range: interim,
end-of-life, malformed, duplicated, other-distribution, and future unverified
OS metadata must deny before any host mutation.
Control bearer authentication may retain only a nullable UTC `last_used_at`
timestamp after an authorization succeeds. Recheck active user, revocation,
expiry, exact scope, and app binding atomically with that update; failed checks
or persistence failures deny and must not alter it. Token lists and dashboards
may show that timestamp, never token values or hashes, IPs, or user agents.
Before retaining an update, deterministically select the lexicographically
first locally verified active app and probe `/_tiny/api/v1/capabilities` on its
derived host through the composed HTTPS gateway. Accept no caller app slug,
host, path, or URL. Only its `401` JSON `not_authorized` envelope with no-store
security headers is health evidence; 404, redirect, 2xx, malformed denial,
timeout, and transport failure require rollback. If no active app exists, prove
that exact database state plus platform health, socket confinement, and safe
unknown-app-host denial; ambiguous state requires rollback, and the first
deployment still owns the full protected-app denial proof.
The replacement binary's candidate doctor may tolerate only the updater-owned,
private, non-symlinked `update-rollback/previous` snapshot that remains until
that same update commits. It must still run every other doctor check; ordinary
`tinyhost status` and `tinyhost doctor` keep rollback-pending degraded, and an
unsafe or incomplete snapshot never becomes healthy.
After a self-update restart, wait only for local TCP connection establishment
on the configured HTTP and HTTPS listeners, with a bounded cancellable retry.
Run doctor, public health, and anonymous-denial gates once afterward; do not
retry or soften their database, credential, DNS, TLS, provider, or gateway
failures.
Before treating `tinyhost init` as complete, derive the sole public proof from
the configured platform host: `https://{platform_host}/api/v1/version`. Use
verified TLS and no redirects; accept only the exact final host, `200`, bounded
`{"api_version":1}` JSON, and `Cache-Control: no-store` plus
`X-Content-Type-Options: nosniff`. A 401/404/5xx, arbitrary 2xx/HTML,
malformed/oversized JSON, host/version mismatch, redirect, TLS, timeout, or
transport failure leaves initialization retryable.
The opt-in VPS acceptance suite may rerun the exact init argv at most eight
times with a cancellation-aware 15-second wait, but only when the binary's
stable `tinyhost: public_health_failed` result and the root-owned parsed init
state both prove that the next incomplete step is `verified`. Never retry
preflight, configuration, credential, install, local-service, SSH, or state
read failures, and do not repeat install or secret-copy work between those
final-readiness attempts.
The service remains the unprivileged `tinyhost` user. Its systemd ambient and
bounding capability sets must contain exactly `CAP_NET_BIND_SERVICE` to bind
the gateway's 80/443 listeners; retain `NoNewPrivileges=yes`, strict filesystem
protections, and only the configured data and ACME cache writable paths. Never
run the gateway as root or add a broader capability to solve a bind failure.
Default-deny the service's socket binds, allow only TCP 80 and TCP 443, and
restrict address families to `AF_UNIX`, `AF_INET`, and `AF_INET6`. TinyHost
does not manage the operator's firewall, SSH, or pre-existing listeners. VPS
evidence must reject any additional non-loopback listener owned by `tinyhost`
without attributing an operator-owned listener to TinyHost.
Canonical V1 setup requires public DNS and Internet-reachable TCP 80/443 for
per-host ACME HTTP-01 and the public gateway proof. Do not claim strict
VPN-only support, temporarily open renewal ports, bypass TLS/probes, or treat
VPN membership as app identity. The accepted first post-V1 VPN direction uses
one operator-supplied certificate/key pair covering the platform and wildcard
app hostnames, private DNS, trusted-network verification, and VPN acceptance
tests. Users still authenticate with TinyHost email OTP and current per-app
policy; VPN membership grants no identity. VPN-only remains unsupported until
those gates are implemented.
For the human operator path, `tinyhost setup` discovers host facts, asks for one
controlled base domain and initial operator email, derives conventional
platform/app/sender/ACME values, and pauses with exact DNS/Resend actions. It
resumes without re-asking valid answers and generates the explicit
non-interactive config/credential references. Keep
`tinyhost init --non-interactive` strict and non-prompting for automation.
Secrets remain root-owned: accept a protected file, or a no-echo prompt only
after supported-shell handling passes audit; never accept a secret in argv or
ordinary config.
`tinyhost status` is offline and must never read provider credentials. Run
`sudo tinyhost doctor` for Resend diagnostics: it reads only the root-owned
mode-0600 credential file (default `/etc/tinyhost/credentials/tinyhost.env`),
validates its absolute no-symlink path, owner, and exact configured shape, and
never prints the credential path, environment reference, key, or provider body.
For root-only `tinyhost deployers authorize|suspend|revoke`, validate root first
but perform the SQLite mutation as `tinyhost`: only narrowly validated legacy
DB/WAL/SHM artifacts may be handed off, and never repair ownership with a
permissive mode change.
Install the deployer client only as the unprivileged user with the reviewed
`packaging/install-client.sh`; it must select a supported client target and
verify HTTPS-fetched checksums, signed metadata, and the embedded release key
before replacing `tiny`. It never installs `tinyhost` or accepts an insecure
release origin.
Production releases require the operator's external Ed25519 private key through
an explicit `TINYHOST_RELEASE_SIGNING_KEY` file path. Keep its directory mode
0700 and file mode 0600, never commit or copy it to the VPS, and verify that it
derives to `packaging/release-public-key.pem` before building. A trust-root
rotation is a deliberate local-root recovery event with a known-good binary
snapshot and the ordinary health and anonymous-denial gates; never bypass
signature verification as a routine update shortcut. Build the SDK artifact
with the release task's temporary npm cache; do not depend on or mutate an
operator's user-level package cache.
SDK capability calls are same-origin and never accept an app ID or secret.
Use capability discovery before KV, blob, or live work. Handle
`TinyVersionIncompatibleError` by upgrading `@tinyhost/sdk`; do not add an app
selector or fall back to control credentials. Raw HTTP clients may omit the SDK
version header, while a supplied unsupported major receives a typed upgrade
error. Blob uploads use exactly one `file` multipart part; blob IDs are opaque,
downloads are attachment bytes, and callers never pass a path, bucket, or
storage key. Custom live channels call `subscribe()` before `connect()` and
`unsubscribe()` when delivery is no longer wanted; reconnect recovery rereads
KV rather than replaying events. Compile examples and run the real-listener SDK contract after SDK
changes. Verify the packed SDK contains only its README, Apache-2.0 license,
declarations, and runtime module. Keep the npm manifest, JSR manifest, exported
SDK version, and release version identical; install-test the npm tarball and
run registry dry runs without publishing. Realtime reconnects reread current KV: no
history, replay, ordering, or durability is available.
Before calling a release candidate ready, run the offline continuous security
gate: version-controlled-candidate secret scan (including first-commit files)
plus its private-key/live-token deny and placeholder/public-key allow self-test,
and bounded host/archive/manifest fuzz targets. CI also runs locked SDK
audit/tests and the pinned Go vulnerability scan; none of these checks need a
VPS credential.
Deployment descriptions are optional immutable `tiny.yaml` presentation text:
trim edge whitespace, permit empty, and reject invalid UTF-8, Unicode controls,
U+2028/U+2029, or more than 280 Unicode code points after trimming. Persist it
only inside the release manifest; dashboard release rows may show it and the
app summary comes only from the current deployment; failed activation preserves
the existing summary.
Dashboard launch controls may link only the server-derived stable
`https://{slug}.{app_suffix}/` origin in a new tab with `noopener noreferrer`;
never expose a release hash/path or imply a gateway-auth bypass. A malformed
final manifest makes the read model unavailable, while intermediate records
without final metadata have no description.
<!-- shared:security:end -->

Do not accept deployment success when an anonymous denial probe fails.
