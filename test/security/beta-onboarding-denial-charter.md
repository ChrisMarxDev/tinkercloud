# Beta onboarding denial charter

## Purpose

This charter is the negative acceptance matrix for the bounded public-beta
operator/deployer onboarding contract. A case below denies the run before it
can be reported as a secure onboarding success. Denial must not expose a
secret, grant a role, activate a release, widen policy, or replace verified
persisted state.

## Release and command integrity

- A placeholder, `latest`, branch, mutable tag selector, redirect-selected
  origin, unreviewed mirror, or caller-supplied release base is not an exact
  beta release. Do not install or claim release verification.
- A missing signature/checksum proof, mismatched client/server version, or
  incompatible API version denies before host setup or deployment proceeds.
- An onboarding document or command that substitutes a template value for a
  required supplied input (domain, email, credential-file boundary, or chosen
  project directory) denies rather than guessing or silently selecting one.

## Role, session, and question boundaries

- Operator, deployer, viewer, and deployment-agent authority never transfer
  because the same person, browser, mailbox, or host is involved.
- Missing, malformed, redirected, stale, or ambiguous dashboard/default-server/
  credential state denies. It does not select a server, reuse another account,
  or issue an OTP.
- A saved valid browser identity or CLI bearer must be proved and reused; asking
  again for its known domain, email, server, policy, OTP, or already-derived
  value denies as a repeated known question.
- A missing dashboard sign-in/allowlist replacement, stale allowlist revision,
  audit failure, or absent explicit confirmation for a broadening edit denies
  deployer authorization. Successful setup alone does not authorize a
  deployer.
- The first human OTP may be the operator dashboard flow and the second may be
  the deployer CLI flow. Before a third human request of any kind, stop
  immediately. No account switch, cache clearing, fresh browser profile,
  viewer login, or alternate mailbox may reset or evade this budget.
- A failed, malformed, timed-out, or denied CLI authentication or OTP attempt
  terminates that deploy attempt. It cannot retry login, use `--force`, switch
  account or identity, log out, delete or clear saved credentials, or create an
  alternate OTP path. A valid exact server-scoped saved identity requires zero
  OTP; any eligible OTP is entered directly in the CLI, never chat.
- After final review, a deploy attempt permits exactly one `tinker deploy .` (or
  explicit public-confirm variant) invocation. Validation, upload, activation,
  verification, transport/TLS/redirect/timeout/error, success, and
  `active_but_unverified` all end that invocation; none permits a second deploy,
  another release upload, or chat-driven retry. Bounded readiness retries remain
  internal to the CLI invocation, while `active_but_unverified` permits only the
  independent exact-URL recheck. A later attempt needs an explicit new human
  request after the cause is addressed.
- Bearers, OTPs, cookies, provider credentials, app IDs, and viewer identities
  supplied through chat, argv, ordinary config, output, logs, or browser app
  state deny the affected step.

## Deployment and evidence gates

- Missing operator `status`, root `doctor`, or no-redirect admin version
  evidence denies operator completion. A version probe without included headers,
  a finite timeout, or the 32768-byte response bound is not completion evidence.
  Claiming protected-app anonymous denial
  before any app exists is fabricated evidence and also denies completion; the
  deployer records that denial after deploying an app.
- A missing `tinker version`, authenticated `whoami`, immutable deployment
  receipt, protected exact app origin, anonymous HTML/asset/reserved-route
  denial proof, or authenticated platform-health proof denies deployment
  success.
- Missing project output does not authorize deployment of source files or an
  inferred build. `tinker deploy` must stop once with the exact project-owned
  build action required.
- Missing/invalid manifest, a path outside the project, symlinked output,
  unclear slug, unreviewed capability, ambiguous SPA fallback, or unresolved
  access policy denies activation rather than creating a permissive receipt.
- A failed, cancelled, redirected, wrong-host, unverified-chain, or exhausted
  certificate readiness check denies activation and retains the prior release.
- A dashboard, CLI, or reader result cannot substitute for fresh gateway
  verification. Any probe that returns app bytes anonymously denies success.

## External dependency and provider boundaries

- An undocumented external dependency—including DNS, ACME, a sender-verification
  action, public 80/443 reachability, supported email provider, credential-file
  ownership/mode, or recipient-domain control—denies readiness. The flow must
  name the exact operator action or stop; it must not manufacture credentials,
  DNS, or health evidence.
- A non-dedicated, non-x86-64, unsupported Ubuntu host; an SSH host key accepted
  without comparison to the VPS provider console; a missing single wildcard
  record or empty resolution for either derived host; a setup request for a
  separate certificate input; an initial allowlist that omits an intended
  normalized deployer or contains any other address; more than one operator
  browser OTP, an OTP despite a reusable browser identity, or a CLI deployer OTP
  during operator setup denies readiness.
- Provider API keys/passwords in argv, ordinary config, chat, Git, logs,
  browser state, SDK/app files, artifacts, or reports deny setup. Only the
  declared root-readable protected credential-file boundary is accepted.
- Provider, DNS, ACME, transport, or persistence failure cannot create an OTP
  bypass, remote recovery endpoint, alternate listener, raw storage URL, or
  permissive app activation.

## Unattended extended acceptance

- Missing, relative, symlinked, non-regular, wrongly owned, or non-`0600`
  local reader key/ledger; malformed or mismatched automation recipient domain;
  non-fixed provider origin; stale, unrelated, duplicate, malformed, or
  ambiguous provider messages; and reader/provider failure all stop unattended
  acceptance before a code is emitted.
- The automation recipient domain remains an exact allowlist separate from the
  platform root domain. A forced login requires both `dev@christopher-marx.de`
  to match recipient domain `christopher-marx.de` and the platform server to
  derive from root `testing.tinkercloud.fun`; it must not require those domains
  to be equal. A valid saved exact deployer identity is reused before reader
  configuration is required and consumes zero OTP.
- A reader failure cannot fall back to a human relay, copied code, chat prompt,
  browser OTP, or another provider. It is neither an OTP bypass nor a way to
  consume an extra human OTP budget.
- Unattended acceptance without the pinned SSH target/acknowledgement, explicit
  disposable-host gate, required offline checks, or private redacted report
  denies. It must not read VPS storage, SQLite, logs, or email-provider output
  as a substitute for public gateway evidence.
