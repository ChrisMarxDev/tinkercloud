# Tinkercloud conversation decision compact

**Cutoff:** 2026-07-29
**Purpose:** Preserve product decisions made during the first complete local and
VPS test loop without preserving the conversational noise.

This file is a decision audit, not a new source-of-truth tier. When a prior
concept or topic document conflicts with a decision below, the reconciled
`PRINCIPLES.md` and `PRD.md` wording wins.

## Product and topology

- Tinkercloud remains a self-hosted, private-only static application platform for
  one dedicated small VPS.
- One `tinkercloud` gateway owns public TCP 80/443, TLS, hostname routing,
  authentication, authorization, static delivery, KV, blobs, and realtime.
- The operator should not need Docker, a reverse proxy, a separate database,
  or a second app-facing storage service.
- Application deletion is permanent hard deletion in V1. There is no recycle
  bin or recoverable deleted-app state.
- Deployer-selected application release rollback is not a V1 feature. Failed
  activation still preserves the previous known-good release, and server-binary
  update rollback remains an operator/recovery mechanism.

## Roles and identity

- `operator`, `deployer`, and `viewer` are distinct roles. The word `user`
  grants no authority by itself.
- The operator manages one exact global active-deployer allowlist in a
  revision-protected multiline field. Adding broadens authority and requires
  confirmation; removing immediately revokes deployer CLI/control credentials
  while preserving the immutable deployer ID and owned apps.
- A deployer logs in by email OTP once per still-valid CLI token. The client
  stores the server-bound bearer in a protected per-user Tinker file, not a
  project file and not an external credential-store command.
- `tinker login` reuses a valid bearer after `whoami`; `tinker login --force`
  deliberately changes account; `tinker logout` revokes the exact server-side
  bearer before deleting the local credential.
- The CLI remembers one verified default platform URL. Human commands with no
  explicit or cached server ask once, verify it without redirects, save it,
  and continue. JSON/automation never prompts.
- Viewer email identity is global to one Tinkercloud browser profile, not repeated
  independently for every app. App access is still evaluated separately from
  the current app policy, then converted through an app-bound one-time handoff
  into a host-only app session. The global identity cookie is never sent to an
  app origin.
- App denial and login screens are Tinkercloud-native HTML, not raw JSON. They
  explain denial, provide a safe sign-in/account-change route, and never expose
  policy membership.

## Deployer experience

- The intended first command is `tinker deploy .` from an already built static
  project.
- Tinkercloud does not run arbitrary application build scripts. The project’s own
  toolchain or coding agent builds first.
- A missing `tinker.yaml` starts bounded human onboarding. The CLI derives safe
  slug/output defaults, asks only for missing or ambiguous required state,
  shows a review/edit summary for optional description/access/capabilities/SPA
  fallback, and uses one final deploy action whose label names access
  broadening. It writes the manifest atomically as a consequence/receipt with
  no second confirmation.
- The safe default policy is owner-only. Public mode does not exist in V1.
- A deployment description is optional, bounded, and immutable with that
  deployment manifest. The dashboard app summary reflects the active
  deployment description.
- The dashboard supports local project search and status filtering and shows a
  safe direct-launch icon only for active apps. Launch always goes through the
  ordinary protected app gateway.
- The CLI should complete setup/login/deploy as one natural resumable flow
  rather than emitting a sequence of prerequisite errors.

## App platform

- The browser SDK is the default creator interface and accepts no app ID,
  viewer token, database credential, provider secret, or endpoint secret.
- V1 SDK capabilities are current viewer/app identity, capability discovery,
  bounded versioned JSON KV, utility-grade local blobs, and ephemeral
  app-scoped realtime.
- Realtime is a freshness hint, never history or durable state. Apps reread KV
  after connection, reconnect, and visibility changes.
- The example gallery must contain useful interactive apps—not marker files—and
  must compile and run as SDK contracts. Shared Checklist, Quick Poll, Team
  Pulse, and Attachment Shelf cover the main V1 surface.

## Operator and release experience

- A human operator should run a guided, resumable `tinkercloud setup`; the command
  generates non-secret config instead of requiring the operator to author it.
- Setup asks only for necessary non-discoverable information. It discovers the
  host and safe defaults, explains exact external DNS/email actions, and resumes
  after they are complete.
- Secrets remain root-owned and never appear in argv, normal config, logs,
  browser code, or deployed apps. The wizard can consume a root-readable file
  or write one from a no-echo prompt.
- Tinkercloud and Resend DNS records are collected before one combined
  DNS-provider checkpoint rather than causing two visits.
- Manual server update derives its health and anonymous-denial probe targets
  from installed state; the operator does not supply a config path, app slug,
  probe host, or probe URL on the normal path.
- Release artifacts are signed and verified before installation. First app
  certificate readiness has a bounded retry; redirects, wrong hosts, invalid
  TLS, or exhausted readiness still prevent activation.
- Operator diagnostics and full-stack tests must work without installing an
  agent on the server. SSH plus the shipped CLI, server diagnostics, browser
  automation, and test-only email retrieval are sufficient.

## Verification and automation

- The full-stack VPS suite must be able to complete OTP flows without a person
  relaying codes. Test email retrieval uses a deliberately scoped test setup;
  production auth is never disabled and no bypass flag ships.
- Reuse of an existing VPS release directory is fail-closed and explicitly
  validated. The live acceptance test runs exactly once per unattended run.
- Every deployment/security success requires denial evidence, not just a
  successful happy path.

## Guiding UX decision

> Ask only for information that is necessary, not already known, and unsafe to
> default.

Generated YAML/configuration is a reviewable result and automation interface,
not human prerequisite paperwork. Optional choices stay behind a review/edit
step. External requirements are surfaced as one exact action at the moment they
block progress. This rule applies to setup, login, deploy, dashboard, testing,
diagnostics, and future capabilities.

## Current test-environment facts, not product requirements

- `dev@christopher-marx.de` is an approved exploratory deployer identity and
  may appear in repository documentation.
- Staging hostnames and viewer identities are intentionally omitted. Repository
  documentation and fixtures use reserved `.example` or `.test` values.
- Environment-specific identities and domains must not become authority
  assumptions or product defaults.
