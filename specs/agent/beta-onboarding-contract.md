# Beta onboarding contract

## Purpose and boundary

This contract defines the bounded first-run path for the public beta: one
operator creates a private Tinkercloud host and authorizes one deployer; that
deployer installs the exact client and deploys one private static app. It is a
contract for the role-facing skills, beta guide, and acceptance work, not a
replacement for the signed-release, host-bootstrap, deployment, or role-skill
contracts.

The gateway remains the only public listener and authorization boundary. The
operator, deployer, and viewer are distinct actors; completing a step as one
does not confer another role. The outcome is a private, owner-only app with
fresh gateway-denial evidence, not a public app, a provider integration, or a
durability claim.

The public-beta onboarding run is pinned to `v0.1.6` from the immutable
directory below. Before any install, the role must prove that exact tag page
and its exact installer asset are available over HTTPS; otherwise it stops.

```text
https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/
```

If the exact release or the role's exact installer asset is unavailable, the
role stops with no install or deploy and no substitute version. Minimal HTTPS
readiness is the exact tag page and that role's installer asset returning HTTPS
success; the released installer remains checksum/signature authority. No command may replace that directory with `latest`, a branch, a tag selector,
an unpinned redirect target, or an operator/deployer-supplied release origin.

## Operator first run

| Item | Contract |
| --- | --- |
| Supplied inputs | A clean dedicated supported Hetzner Ubuntu 24.04/26.04 x86-64 VPS with root SSH; one controlled base domain and wildcard DNS; normalized operator email; a verified email sender; a root-readable protected email-provider credential file; and one normalized deployer email to authorize. |
| Derived values | `admin.<domain>`, app hostnames, internal ACME contact from the normalized operator email, service user, private data directory, generated HMAC material, generated non-secret config, and the exact dashboard URL. |
| Prompts allowed | `tinkercloud setup` may ask only for the base domain, operator email, provider choice/sender, and root-readable credential-file path when those values are absent. For the basic Resend beta path, its exact prompt order and labels are `Base domain:`, `Operator email:`, `Verified Resend sender email:`, and `Root-readable Resend API key file:`. It may pause for one exact DNS or sender-verification action. It must reuse a previously validated answer on resume. The operator may complete the dashboard OTP and confirm an allowlist edit that broadens authority. |
| Prompts forbidden | Provider credentials in chat, argv, ordinary config, browser state, or logs; an ACME-contact value; release URL/selector; app ID; viewer identity; a second listener; or questions whose answer is already discovered, derived, or stored. |
| Persisted state | Root-owned credential material, non-secret typed config, init-state, one control SQLite database, private release/app state, the operator identity, service unit, and the exact active-deployer allowlist revision. Secrets are never persisted in ordinary config. |
| Required evidence | Signature/checksum verification; successful `sudo tinkercloud status` output containing exactly one full line `version: 0.1.6`; local service health; no-redirect TLS `https://admin.<domain>/api/v1/version` proof with bounded API-version JSON and gateway headers; route classification, socket confinement, safe unknown-app-host denial; dashboard authorization of the deployer; and the printed root-only recovery/doctor commands. |

Before installation, the operator proves from the VPS provider console that the
target is a clean dedicated x86-64 Ubuntu 24.04 or 26.04 VPS and records the
console-derived SSH host-key fingerprint. DNS control is required for one
`*.<domain>` wildcard record: point its A record at the confirmed provider-
console IPv4, add AAAA only for a confirmed reachable IPv6, or use CNAME only
for the provider's stable hostname. Compare the target in the provider UI and
confirm both derived hostnames resolve before ACME. The operator supplies no
separate certificate input. The initial
active-deployer allowlist equals exactly the intended normalized deployer email
set and contains no other addresses; the operator email alone is sufficient
only when that is the exact intended set. Operator setup permits at most one
browser OTP, uses zero when a reusable browser identity is valid, and uses no
CLI deployer OTP.

The exact first-run command sequence is:

```sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 --head --location --fail --silent --show-error --max-time 15 -o /dev/null https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v0.1.6
curl --proto '=https' --proto-redir '=https' --tlsv1.2 --location --fail --silent --show-error --max-time 15 --max-filesize 32768 -o /dev/null https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-host.sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-host.sh | sh
sudo tinkercloud setup
```

After setup health succeeds, the operator signs in at the derived exact
`https://admin.<domain>/` dashboard, replaces the single active-deployer
allowlist with the reviewed normalized deployer email set, and confirms only
if that edit broadens deployer authority. A stale revision, audit failure, or
ambiguous dashboard/session state leaves the allowlist unchanged.

An operator browser OTP is terminal on failure: immediately after an operator
browser OTP attempt fails, is malformed, times out, or is denied, operator
onboarding stops. It does not retry or request another code, switch operator
identity or mailbox, clear browser cookies, or create another OTP path. A valid
exact browser identity uses zero OTP. This human boundary does not alter
unattended machine OTP acceptance.

A fully fresh successful human onboarding with neither a reusable browser
identity nor a saved CLI bearer requests exactly two codes total: exactly one
operator browser code and exactly one deployer CLI code. A reusable identity
reduces the relevant lane to zero.

For the basic Resend beta path, an already-safe VPS-resident root-owned,
non-symlink regular mode-`0600` credential file at any supplied exact path is
reused after this exact check and passed directly to setup:

```sh
test -f "<VPS_RESEND_KEY_FILE>" && test ! -L "<VPS_RESEND_KEY_FILE>" && test "$(stat -c '%U:%G %a' "<VPS_RESEND_KEY_FILE>")" = 'root:root 600'
```

Only a workstation-local or ambiguous file requires an SSH target and transfer
through verified root SSH to
`/root/.config/tinkercloud/resend-api-key`, then verifies that final destination
is a root-owned regular file with mode `0600` using the same check. The
credential contents never
enter argv, chat, config, browser state, or logs. The dashboard path is
**Deployers** → **Active deployer allowlist** → **Allowed deployer emails**;
an authority addition requires **I confirm that adding any email grants
deployment authority.** and **Save active deployers**.

Operator completion then requires a successful `sudo tinkercloud status` whose
captured output contains exactly one full line `version: 0.1.6`, while preserving
the command's unhealthy exit status; `sudo tinkercloud doctor`; and
`curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https://admin.<domain>/api/v1/version`.
There is no `tinkercloud version` command and onboarding must not invent one.
The bounded no-redirect request records included gateway headers and at most
32768 response-body bytes. Completion accepts only HTTP `200`, an
`application/json` media type, and exactly `{"api_version":1}` with no error or
extra fields. These curl flags
are supported by the curl version shipped with the supported Ubuntu hosts. A
clean operator host has no protected app yet, so its
completion does not fabricate protected-app anonymous-denial evidence. That
proof belongs to deployer deployment completion once an app exists.

Normalize email by trimming surrounding whitespace, preserving local-part case,
lowercasing only the domain, requiring exactly one `@`, nonempty local/domain,
a dotted domain, and no whitespace/control characters. Do not apply provider
plus/dot rewriting; setup and the dashboard review the exact normalized set.
An already VPS-local credential uses its supplied exact path directly after a
root-owned, non-symlink regular mode-`0600` check; the canonical path is only a
workstation-transfer destination.

## Deployer first run

| Item | Contract |
| --- | --- |
| Supplied inputs | A macOS or Linux workstation used as the deployer, a static project directory, and the operator-authorized deployer email. On a fully fresh workstation, `<SERVER>` comes only from the operator-provided exact normalized HTTPS admin URL. A saved default is reusable only when it was previously directly verified from such an operator handoff; current CLI state cannot invent or derive a server. |
| Derived values | The saved default server after the deploy command's direct no-redirect version proof; protected per-user credential-file location; inferred safe project output/slug choices; owner-only policy; generated strict `tinker.yaml` receipt when absent; deterministic archive; immutable release manifest; deployment ID; and protected app URL. |
| Prompts allowed | One bounded platform-URL setup prompt only when no valid default exists; required unresolved project choices; one combined review of optional description, access, capabilities, and SPA fallback; one final access-broadening confirmation; and normal CLI email/OTP prompts only after an unauthorized/expired bearer. |
| Prompts forbidden | A bearer, provider credential, app ID, viewer identity, arbitrary build command, manifest rewriting confirmation, repeated known server/email/policy questions, or a prompt in `--json` mode. The sole OTP exception is the supervised same-CLI handoff below. |
| Persisted state | Default server record plus a separate mode-`0700` configuration directory and mode-`0600`, regular, non-symlinked bearer file keyed by normalized HTTPS server. The generated manifest is a project receipt, never a secret store. |
| Required evidence | `tinker version` prints exactly `tinker 0.1.6`; the one deploy command internally proves the server and current authorized deployer; activation returns the immutable deployment ID and protected exact origin; private combined evidence always proves fresh anonymous root and representative reserved-route `401 not_authorized` denial with no app bytes, while the server candidate additionally proves denial of an actual immutable non-index asset only when one exists; a single-file app requires no fabricated asset evidence; authenticated platform health succeeds. |

The conditional CLI login prompts are exactly `Email: ` and `Code: `; their
position in the combined fresh prompt order is specified below. The code stays
in the same CLI process, except for the supervised same-process handoff below. Before invoking deploy the agent asks `Deploy this owner-only app to
<server> now? [y/N]`; only explicit yes proceeds.
Human success is `Deployment: <id>`, `State: active`, `URL: <exact-origin>`;
JSON is `valid:true` plus a bounded deployment object. Platform health and
anonymous probes are internal success preconditions.

The exact first-run command sequence is:

```sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 --head --location --fail --silent --show-error --max-time 15 -o /dev/null https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v0.1.6
curl --proto '=https' --proto-redir '=https' --tlsv1.2 --location --fail --silent --show-error --max-time 15 --max-filesize 32768 -o /dev/null https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-client.sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-client.sh | sh
tinker version
```

Run the deployer commands from the already-selected local static project root.
`tinker version` must print exactly `tinker 0.1.6`; any other output stops the
attempt. On a fully fresh workstation, `<SERVER>` comes only from the
operator-provided exact normalized HTTPS admin URL. A reusable saved default
must have been previously directly verified from such an operator handoff;
current CLI state cannot invent or derive a server. The one deploy command
verifies that URL directly without redirects and saves it, reuses a valid server-bound bearer, or performs exactly
one CLI email-and-OTP login only when the bearer is absent, unauthorized, or
expired. Do not run standalone `tinker whoami` or `tinker login` before this
fresh first deploy. The CLI never runs an inferred build; missing output stops
once with the exact project-owned action needed.

For a human deploy with explicit `--server`, saving the verified endpoint is
allowed only when the protected default-server record is exactly absent. A
saved endpoint, including one for another server, is preserved; an unreadable,
malformed, or failed default-store write stops before app creation or upload.
JSON/non-interactive deploys do not prompt or cache a server.

Human-supervised coding-agent OTP handoff: When a human-supervised coding
agent acts as the deployer and holds the CLI, it may, only when no reusable
server-bound bearer exists and after endpoint, email, and manifest validation
and after the same CLI process reaches its
normal `Code: ` prompt, ask the human exactly once for the short-lived emailed
OTP, accept it in the agent interaction, and immediately submit it only to
that same CLI process. Use this only with a trusted human-supervised agent; its
provider may retain the interaction. Do not restate it or copy it into files, source, argv,
logs, summaries, or final output; never ask for a bearer. A failed, malformed,
timed-out, or denied OTP is terminal: there is no second code, retry, forced
login, identity/account/server switch, or alternate collection path. The CLI
stores the resulting scoped bearer for later exact-server reuse. Fully
unattended `tinkercloud-deployment-agent` and VPS Resend-reader paths must not
fall back to an agent interaction OTP.

Immediately after a CLI authentication or OTP attempt fails, is malformed,
times out, or is denied, that deploy attempt is terminal. It MUST NOT retry
login, use `--force`, switch account or identity, log out, delete or clear saved
credentials, or create an alternate OTP path. An exact valid server-scoped saved
identity consumes zero OTP. The supervised handoff above is the only
agent-interaction exception; it submits the one accepted OTP only to the same
CLI process.

The six first-manifest prompts are exactly `App slug (<suggested>, Enter to
accept):`, `Description (optional):`, `Build output (<default>, Enter to
accept):`, `Allowed emails or domains, comma-separated (optional):`, `Features
(kv,blobs,realtime; optional):`, and `SPA fallback (optional):`. Empty optional
answers use their defaults: no description, owner-only access, no features, and
no fallback. They are not extra questions. Root `index.html` selects `.`,
exactly one safe conventional output with `index.html` selects that directory,
and multiple/none blocks for one output question. Invalid/ambiguous slugs block
for one stable lowercase slug.

For a fully fresh no-manifest/no-bearer deploy, the combined CLI prompt order is
exactly the six manifest prompts above, then—only after manifest creation
succeeds—`Email: ` and `Code: ` when authentication is needed. A valid saved bearer omits both
authentication prompts without changing manifest-prompt order. Invalid,
ambiguous, or unwritable local manifest state stops before requesting an OTP.
The agent's final `Deploy this owner-only app to <server> now? [y/N]` decision is
collected before the single CLI invocation; it is not another CLI prompt.

The bounded fresh path never runs standalone `tinker init` first. Its one
`tinker --server <SERVER> deploy .` invocation generates the reviewed receipt
inside deploy. A separately requested, non-fresh manifest-preparation workflow
may use `tinker init`, but it is not part of or preparation for this bounded
fresh deployment.

After final review of endpoint, slug, description, output, owner-only access,
no features, and SPA fallback—and affirmative go-ahead even for owner-only—the
agent invokes exactly once:

```sh
tinker --server <SERVER> deploy .
```

The public-confirmation form is reserved for its explicit later public flow.
Any CLI deploy
outcome—validation, upload, activation, verification, transport/TLS/redirect/
timeout/error, or success—ends that invocation. The agent MUST NOT rerun deploy,
upload another release, or retry from chat. The CLI's already-bounded transient
readiness retries remain internal to that one invocation.
`active_but_unverified` permits only the existing independent exact-URL recheck,
never a second deployment. A later deploy attempt requires an explicit new human
request after the cause is addressed; it is never an automatic retry.

That recheck is `curl --include --silent --show-error --no-location --cookie ''
--max-time 15 --max-filesize 32768 -H 'Accept: application/json' <returned-url>`
and is a fresh anonymous exact-URL request with no authentication. It requires
`401`, `Cache-Control: no-store`, JSON `not_authorized`, no `Set-Cookie` or
`Location`, and no app bytes. It is evidence only and never a second deploy.

## Human OTP budget and unattended acceptance

The beta onboarding path permits at most two human OTP requests in total:

1. one operator dashboard OTP, if no valid global browser identity exists, to
   authorize the deployer; and
2. one deployer CLI OTP, if the saved deployer bearer is absent, unauthorized,
   or expired. These ceilings are nonfungible: one browser OTP maximum for the
   operator and one CLI OTP maximum for the deployer; the two-role total never
   permits two OTPs for either role.

A completely fresh successful end-to-end onboarding with no reusable identity
requests exactly two human codes total: exactly one operator dashboard
OTP and exactly one deployer CLI OTP. Any additional code, retry, account
switch, or viewer login is a deployment-flow failure. A failed OTP is terminal;
there is no automatic human OTP retry. The clean two-code human flow MUST NOT
invoke the extended VPS security matrix. That matrix is separate and unattended;
it never authorizes asking the human for more codes.

Before issuing, relaying, requesting, or suggesting a third human OTP, the
flow MUST stop immediately and report the exceeded budget. It MUST NOT switch
accounts, clear credentials, create a viewer session, or retry through another
human mailbox to evade the limit. A valid reusable browser identity or bearer
uses zero additional OTP budget; malformed, transport, dependency, redirected,
or ambiguous state fails closed and also does not trigger OTP.

Opening the deployed protected app is outside this bounded operator/deployer
journey unless the exact browser identity is already proven reusable for that
app. Otherwise the run records its fresh anonymous-denial evidence and defers
opening to separate later viewer work; it MUST NOT request a viewer/browser OTP
or create a viewer session.

Extended security acceptance is unattended. Its local, separately protected
OTP reader may consume only the normal gateway-issued deployer/viewer OTP for
the configured exact recipient domain and fixed Resend HTTPS origin. That
recipient-domain allowlist is independent of the platform root domain: a
deployer at `dev@christopher-marx.de` requires the recipient domain
`christopher-marx.de`, while the platform may be
`https://admin.testing.tinkercloud.example` derived only from the root domain
`testing.tinkercloud.example`. A valid saved exact CLI identity is checked and
reused before reader configuration is required, using zero OTP. When forced
login is necessary, both the exact recipient-domain validation and the
server-derived-from-platform-root validation are required independently. Reader
failure, missing/unsafe reader state, an ambiguous provider response, or an
unavailable code is terminal for that run: it MUST NOT fall back to a human
relay, chat-pasted code, browser-based code collection, or a different provider
path. The reader is test infrastructure, never production authentication or an
authorization bypass.

## Completion and fail-closed rule

The onboarding run completes only when all required evidence in both role
tables is recorded without secrets and the deployer has a private URL with
fresh anonymous-denial proof. Missing evidence, an unpinned/mutable release,
missing dashboard authorization, failed verification, unresolved external
dependency, or any forbidden prompt denies completion and preserves the last
known-good state.
