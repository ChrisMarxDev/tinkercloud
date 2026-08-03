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

Release preparation pins the next public beta onboarding run to `v0.1.6` from
the immutable directory below. It is not a claim that `v0.1.6` is published:
run its commands only after that exact prerelease has been published.

```text
https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/
```

No command may replace that directory with `latest`, a branch, a tag selector,
an unpinned redirect target, or an operator/deployer-supplied release origin.

## Operator first run

| Item | Contract |
| --- | --- |
| Supplied inputs | A clean dedicated supported Hetzner Ubuntu 24.04/26.04 x86-64 VPS with root SSH; one controlled base domain and wildcard DNS; normalized operator email; a verified email sender; a root-readable protected email-provider credential file; and one normalized deployer email to authorize. |
| Derived values | `admin.<domain>`, app hostnames, internal ACME contact from the normalized operator email, service user, private data directory, generated HMAC material, generated non-secret config, and the exact dashboard URL. |
| Prompts allowed | `tinkercloud setup` may ask only for the base domain, operator email, provider choice/sender, and root-readable credential-file path when those values are absent. For the basic Resend beta path, its exact prompt order and labels are `Base domain:`, `Operator email:`, `Verified Resend sender email:`, and `Root-readable Resend API key file:`. It may pause for one exact DNS or sender-verification action. It must reuse a previously validated answer on resume. The operator may complete the dashboard OTP and confirm an allowlist edit that broadens authority. |
| Prompts forbidden | Provider credentials in chat, argv, ordinary config, browser state, or logs; an ACME-contact value; release URL/selector; app ID; viewer identity; a second listener; or questions whose answer is already discovered, derived, or stored. |
| Persisted state | Root-owned credential material, non-secret typed config, init-state, one control SQLite database, private release/app state, the operator identity, service unit, and the exact active-deployer allowlist revision. Secrets are never persisted in ordinary config. |
| Required evidence | Signature/checksum verification; local service health; no-redirect TLS `https://admin.<domain>/api/v1/version` proof with bounded API-version JSON and gateway headers; route classification, socket confinement, safe unknown-app-host denial; dashboard authorization of the deployer; and the printed root-only recovery/doctor commands. |

Before installation, the operator proves from the VPS provider console that the
target is a clean dedicated x86-64 Ubuntu 24.04 or 26.04 VPS and records the
console-derived SSH host-key fingerprint. DNS control is required for one
`*.<domain>` wildcard record using A/AAAA or CNAME as the provider supports;
both derived hostnames must resolve nonempty before ACME without comparison to
a public IP. The operator supplies no separate certificate input. The initial
active-deployer allowlist equals exactly the intended normalized deployer email
set and contains no other addresses; the operator email alone is sufficient
only when that is the exact intended set. Operator setup permits at most one
browser OTP, uses zero when a reusable browser identity is valid, and uses no
CLI deployer OTP.

The exact first-run command sequence is:

```sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-host.sh | sh
sudo tinkercloud setup
```

After setup health succeeds, the operator signs in at the derived exact
`https://admin.<domain>/` dashboard, replaces the single active-deployer
allowlist with the reviewed normalized deployer email set, and confirms only
if that edit broadens deployer authority. A stale revision, audit failure, or
ambiguous dashboard/session state leaves the allowlist unchanged.

For the basic Resend beta path, the operator transfers the supplied local
credential file only through their verified root SSH target to
`/root/.config/tinkercloud/resend-api-key`, then verifies that final destination
is a root-owned regular file with mode `0600`. The credential contents never
enter argv, chat, config, browser state, or logs. The dashboard path is
**Deployers** → **Active deployer allowlist** → **Allowed deployer emails**;
an authority addition requires **I confirm that adding any email grants
deployment authority.** and **Save active deployers**.

Operator completion then requires `sudo tinkercloud status`, `sudo tinkercloud
doctor`, and
`curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https://admin.<domain>/api/v1/version`.
The bounded no-redirect request records included gateway headers and at most
32768 response-body bytes containing bounded API-version JSON. These curl flags
are supported by the curl version shipped with the supported Ubuntu hosts. A
clean operator host has no protected app yet, so its
completion does not fabricate protected-app anonymous-denial evidence. That
proof belongs to deployer deployment completion once an app exists.

## Deployer first run

| Item | Contract |
| --- | --- |
| Supplied inputs | A macOS or Linux workstation used as the deployer, a static project directory, and the operator-authorized deployer email. The platform URL is discovered from saved state or supplied once as normalized HTTPS when absent. |
| Derived values | The default server after direct no-redirect version proof; protected per-user credential-file location; inferred safe project output/slug choices; owner-only policy; generated strict `tinker.yaml` receipt when absent; deterministic archive; immutable release manifest; deployment ID; and protected app URL. |
| Prompts allowed | One bounded platform-URL setup prompt only when no valid default exists; required unresolved project choices; one combined review of optional description, access, capabilities, and SPA fallback; one final access-broadening confirmation; and normal CLI email/OTP prompts only after an unauthorized/expired bearer. |
| Prompts forbidden | A bearer, OTP, provider credential, app ID, viewer identity, arbitrary build command, manifest rewriting confirmation, repeated known server/email/policy questions, or a prompt in `--json` mode. |
| Persisted state | Default server record plus a separate mode-`0700` configuration directory and mode-`0600`, regular, non-symlinked bearer file keyed by normalized HTTPS server. The generated manifest is a project receipt, never a secret store. |
| Required evidence | `tinker version` proves the pinned client; `whoami` proves the current authorized deployer; activation returns the immutable deployment ID and protected exact origin; fresh anonymous HTML, asset, and reserved API denial probes return no app bytes; authenticated platform health succeeds. |

The exact first-run command sequence is:

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-client.sh | sh
tinker version
tinker deploy .
```

Run the deployer commands from the already-selected local static project root.
The human deployment flow may collect the first valid HTTPS platform URL,
deployer email, and OTP within `tinker deploy .`; it reuses a valid saved bearer
after `whoami` and does not request OTP again. The CLI never runs an inferred
build; missing output stops once with the exact project-owned action needed.

Immediately after a CLI authentication or OTP attempt fails, is malformed,
times out, or is denied, that deploy attempt is terminal. It MUST NOT retry
login, use `--force`, switch account or identity, log out, delete or clear saved
credentials, or create an alternate OTP path. An exact valid server-scoped saved
identity consumes zero OTP; an eligible human OTP is entered directly in the
CLI and never through chat.

After final review, the agent invokes `tinker deploy .` (or the explicit
public-confirm variant) exactly once for that deploy attempt. Any CLI deploy
outcome—validation, upload, activation, verification, transport/TLS/redirect/
timeout/error, or success—ends that invocation. The agent MUST NOT rerun deploy,
upload another release, or retry from chat. The CLI's already-bounded transient
readiness retries remain internal to that one invocation.
`active_but_unverified` permits only the existing independent exact-URL recheck,
never a second deployment. A later deploy attempt requires an explicit new human
request after the cause is addressed; it is never an automatic retry.

## Human OTP budget and unattended acceptance

The beta onboarding path permits at most two human OTP requests in total:

1. one operator dashboard OTP, if no valid global browser identity exists, to
   authorize the deployer; and
2. one deployer CLI OTP, if the saved deployer bearer is absent, unauthorized,
   or expired.

Before issuing, relaying, requesting, or suggesting a third human OTP, the
flow MUST stop immediately and report the exceeded budget. It MUST NOT switch
accounts, clear credentials, create a viewer session, or retry through another
human mailbox to evade the limit. A valid reusable browser identity or bearer
uses zero additional OTP budget; malformed, transport, dependency, redirected,
or ambiguous state fails closed and also does not trigger OTP.

Extended security acceptance is unattended. Its local, separately protected
OTP reader may consume only the normal gateway-issued deployer/viewer OTP for
the configured exact recipient domain and fixed Resend HTTPS origin. That
recipient-domain allowlist is independent of the platform root domain: a
deployer at `dev@christopher-marx.de` requires the recipient domain
`christopher-marx.de`, while the platform may be
`https://admin.testing.tinkercloud.fun` derived only from the root domain
`testing.tinkercloud.fun`. A valid saved exact CLI identity is checked and
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
