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
- A client version result other than exactly `tinker 0.1.6` denies before the
  first deploy. `tinker v0.1.6`, a version inferred from a filename, or a
  compatible-but-different version is not sufficient evidence.
- An unavailable exact `v0.1.6` tag or the current role's required installer
  asset is terminal: no install, deploy, or substitute version. HTTPS readiness
  checks only the exact tag page and that role's asset; it does not replace the
  installer's checksum/signature verification.
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
- OTP ceilings are nonfungible: one browser OTP maximum belongs to the operator
  and one CLI OTP maximum belongs to the deployer. A combined total of two never
  permits two OTPs for one role.
- A fully fresh successful human onboarding with neither a reusable browser
  identity nor a saved CLI bearer requires exactly two codes total: exactly one
  operator browser code and exactly one deployer CLI code. A reusable identity
  reduces the relevant lane to zero; any other successful human-code count
  denies the bounded onboarding run.
- Invoking the extended VPS security matrix from the clean human onboarding
  denies the two-code flow. That matrix is separate and unattended; its primary
  deployer, second-owner, viewer, and account-switch OTP exercises never
  authorize requesting another human code.
- A failed, malformed, timed-out, or denied operator browser OTP attempt
  terminates operator onboarding. It cannot retry or request another code,
  switch operator identity or mailbox, clear browser cookies, or create another
  OTP path. A valid exact browser identity requires zero OTP. This human rule
  does not alter unattended machine OTP acceptance.
- Opening the protected app during the bounded operator/deployer run requires a
  proven reusable exact browser identity. Otherwise, defer it to separate later
  viewer work and record anonymous-denial evidence; do not request a
  viewer/browser OTP or create a viewer session.
- A failed, malformed, timed-out, or denied CLI authentication or OTP attempt
  terminates that deploy attempt. It cannot retry login, use `--force`, switch
  account or identity, log out, delete or clear saved credentials, or create an
  alternate OTP path. A valid exact server-scoped saved identity requires zero
  OTP; any eligible OTP is entered directly in the CLI, never chat.
- A fresh deployment that runs standalone `tinker whoami` or `tinker login`
  before its one deploy command denies this beta path. The deploy command alone
  verifies the exact normalized HTTPS admin URL without redirects, saves it,
  reuses a bearer when valid, and otherwise performs the one allowed CLI
  email-and-OTP login.
- On a fully fresh workstation, a server taken from unverified current CLI
  state, invented locally, or derived from an app hostname denies the path.
  `<SERVER>` must come from the operator-provided exact normalized HTTPS admin
  URL. A saved default is reusable only after a previous direct verification
  of that operator handoff.
- An explicit human deploy may save the verified server only when the
  default-server record is exactly absent. An existing default (including a
  different server), unreadable/corrupt default state, or failed write must not
  be overwritten and stops before app creation, upload, or activation. JSON
  deploys do not cache server state.
- Login labels other than `Email: ` and `Code: `, a code outside the CLI, or an
  implicit approval instead of exact affirmative pre-invocation consent deny.
- For a fresh no-manifest/no-bearer deploy, any `Email: ` or `Code: ` prompt
  before all six ordered manifest prompts complete denies the bounded path.
  Invalid, ambiguous, or unwritable manifest state must stop before OTP; a
  saved bearer may omit authentication prompts but cannot reorder the manifest
  prompts. The agent's final confirmation remains before the one CLI invocation
  and is not part of the CLI prompt sequence.
- Running standalone `tinker init` before the bounded fresh deploy denies that
  path. The reviewed receipt must be generated inside its one deploy
  invocation; any separate init workflow is explicitly non-fresh and cannot be
  promoted as preparation for the bounded path.
- `active_but_unverified` permits only a fresh anonymous, no-redirect,
  no-cookie exact-URL recheck returning `401` JSON `not_authorized`, `no-store`,
  and no app bytes; it never authorizes another deploy.
- After final review and affirmative go-ahead (including owner-only), a deploy
  attempt permits exactly one `tinker --server <remembered-server> deploy .`
  (or `tinker --server <remembered-server> --confirm-public deploy .`) invocation.
  Validation, upload, activation,
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
  A `status` command whose exit is masked, output with zero or multiple exact
  `version: 0.1.6` lines, a substring-only match, or an invented
  `tinkercloud version` command also denies operator completion.
  Claiming protected-app anonymous denial
  before any app exists is fabricated evidence and also denies completion; the
  deployer records that denial after deploying an app.
- A missing `tinker version`, authenticated `whoami`, immutable deployment
  receipt, protected exact app origin, anonymous HTML/asset/reserved-route
  denial proof, or authenticated platform-health proof denies deployment
  success.
- Private combined evidence always requires anonymous root plus representative
  reserved-route exact `401 not_authorized` denial before dispatch, with
  `no-store`, no redirect/cookie, and no release bytes. When the server
  candidate contains a servable non-index asset, that actual asset requires the
  same denial. A release containing only index content must remain valid without
  an asset probe and must not invent an asset path or claim fabricated evidence.
  The independent live client probe covers root plus a representative reserved
  route; optional real-asset proof remains server-owned unless a typed private
  activation receipt later exposes that path.
- Missing project output does not authorize deployment of source files or an
  inferred build. `tinker deploy` must stop once with the exact project-owned
  build action required.
- Missing/invalid manifest, a path outside the project, symlinked output,
  unclear slug, unreviewed capability, ambiguous SPA fallback, or unresolved
  access policy denies activation rather than creating a permissive receipt.
- A private v2 example or generated receipt containing `access.indexing`
  denies documentation/receipt acceptance; indexing belongs only to an
  explicitly public release.
- A transferred Resend credential without a final post-`chown`/`chmod` proof
  that the destination is a regular non-symlink with exact `root:root 600`
  ownership/mode denies operator credential readiness.
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
- A safe root-owned non-symlink regular mode-`0600` credential file already on
  the VPS must be reused after its exact check. Asking for a workstation transfer
  or SSH target in that case is an unnecessary question and denies readiness.
- A wildcard A record that is not compared with the confirmed provider-console
  IPv4, an AAAA record without confirmed reachable IPv6, a CNAME without the
  provider's stable target, or a missing provider-UI target comparison denies
  DNS readiness. A certificate input remains forbidden.
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
  derive from root `testing.tinkercloud.example`; it must not require those domains
  to be equal. A valid saved exact deployer identity is reused before reader
  configuration is required and consumes zero OTP.
- A reader failure cannot fall back to a human relay, copied code, chat prompt,
  browser OTP, or another provider. It is neither an OTP bypass nor a way to
  consume an extra human OTP budget.
- Unattended acceptance without the pinned SSH target/acknowledgement, explicit
  disposable-host gate, required offline checks, or private redacted report
  denies. It must not read VPS storage, SQLite, logs, or email-provider output
  as a substitute for public gateway evidence.
