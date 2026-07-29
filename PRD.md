# TinyHost Product Requirements Document

**Status:** Locked for manual implementation handoff

**Target:** First production-capable release (“V1”)

**Product:** TinyHost server, Tiny deployer CLI, Tiny browser SDK, and agent skills

**Naming:** “TinyHost”, `tinyhost`, `tiny`, and `@tinyhost/sdk` are replaceable
working labels, not architectural identifiers

**North star:** [Shopify Quick](docs/product/north-star-quick.md)

**Last updated:** 2026-07-29

## 0. How to use this document

This PRD is the canonical product and implementation handoff for the first
TinyHost repository.

Source-of-truth order:

1. [`PRINCIPLES.md`](PRINCIPLES.md) — immutable decision gates.
2. This PRD — release scope, behavior, acceptance, and sequencing.
3. `specs/` — versioned interface contracts.
4. `docs/decisions/` — accepted architecture decisions.
5. Topic documents under `docs/` — deeper rationale and test charters.
6. `concept/` — browsable planning and feature ranking.

When documents conflict, update the lower-priority document to match the
higher-priority source. An implementation agent must not silently resolve a
conflict by coding one interpretation.

The implementation target is V1 in §5, including KV and realtime. Post-V1
direction in §21 informs seams but is not authorization to implement those
features.

---

## 1. Product summary

TinyHost is a self-hosted platform for deploying small private web applications.

A technical operator installs one self-contained server binary on a dedicated
Hetzner VPS. Authorized deployers use a separate small CLI to upload static
applications. Each app receives a stable HTTPS URL and is private by default.

```text
tiny login
tiny deploy ./dist --allow alice@example.com
```

TinyHost—not the deployed app—owns:

- public ports 80 and 443;
- TLS termination and hostname routing;
- viewer authentication and app authorization;
- release storage and static asset serving;
- app-scoped data capabilities;
- deployment activation and failed-activation preservation;
- resource limits, audit, and operator diagnostics.

The application is an untrusted static bundle and never receives a request until
the gateway has approved it.

## 2. Core promise

> No private application file, API route, capability request, WebSocket
> connection, blob, or future backend request is reachable until TinyHost has
> authenticated and authorized the request.

This guarantee must be structural. It must not depend on generated middleware,
framework configuration, client-side checks, app route guards, or developer
discipline.

The future Go code must make bypass difficult:

- protected handlers require a typed `AuthorizationContext`;
- pre-authentication routes have no dependency on app content/data repositories;
- app and identity are derived by the server;
- missing, stale, malformed, or unavailable state denies access;
- route-registry tests prove every public route has a classification.

## 3. Why this product should exist

AI makes small applications cheap to create. Safely sharing them remains
expensive because every app otherwise needs hosting, authentication, deployment,
storage, secrets, and operational judgment.

TinyHost moves the repeated security and infrastructure work into one small
platform. Its users can focus on dashboards, prototypes, review tools, forms,
games, and staff utilities.

Shopify Quick demonstrates the desired cultural and usability outcome:

```text
folder of files
→ secure URL in seconds
→ useful backend capabilities through a client API
→ coding agents already know how to use the platform
```

TinyHost adopts that simplicity for self-hosted and smaller organizations while
using stricter app ownership and per-app access rules.

## 4. Product goals and success measures

### 4.1 Goals

1. A technical operator can initialize a clean dedicated Hetzner VPS without
   assembling Docker, a reverse proxy, a database service, or an auth provider.
2. A deployer can publish a protected static app with one command.
3. A non-technical viewer can authenticate by email and access only allowed apps.
4. A static app can identify its viewer and persist app-scoped JSON state and
   bounded files through one typed SDK.
5. A static app can subscribe and publish app-scoped live events without
   provisioning a backend.
6. A coding agent can build, deploy, and independently verify an app using
   shipped skills.
7. Security and update failures preserve the last known-good state and never
   make private content public.
8. Human setup and deployment ask only for information that cannot be
   discovered or safely defaulted; generated configuration is an output, not a
   prerequisite ceremony.

### 4.2 V1 success measures

These are release gates, not aspirational analytics:

- Median clean-host setup, after DNS exists: under 10 minutes.
- Median static deployment excluding local build: under 30 seconds for a 25 MB
  bundle on a typical European connection.
- Anonymous protected-surface tests: 100% denied with zero app-content bytes.
- Access/session/app revocation: effective on the next request after mutation
  success.
- Failed activation: previous release remains active in 100% of injected
  failure cases.
- Failed self-update health gate: prior healthy binary/state automatically
  restored.
- Cross-app KV/blob isolation matrix: zero unauthorized reads or writes.
- Every public SDK method has compiled examples and an agent-skill reference.
- Wrong-app, revoked, suspended, and anonymous WebSocket attempts never establish
  a usable channel.

Product analytics and telemetry are not required to measure these gates. They
can be established through local test and release evidence.

## 5. V1 release boundary

### 5.1 Included

#### Server and operations

- One `tinyhost` Go server/operator binary.
- Hetzner-first systemd installation on a clean dedicated Ubuntu 24.04 LTS or
  Ubuntu 26.04 LTS x86-64 VPS.
- One SQLite database in WAL mode.
- One private data directory containing immutable releases and keys.
- Embedded migrations, login/admin/deployer UI, and local assets.
- Automatic per-host ACME certificates.
- Resend email adapter behind a provider-neutral interface.
- Root-only operator recovery without email.
- Signed manual self-update with health-gated automatic rollback.
- Status, doctor, logs, audit, quotas, cleanup, and disk-watermark controls.
- A small operator-only host resource chart for bounded recent CPU, RAM, and
  TinyHost data-volume storage usage, collected by the existing server.

#### Authentication and authorization

- One initial operator; multiple deployers.
- Operator reconciles one exact active-deployer allowlist by normalized email.
- Viewer authentication through email one-time codes.
- Separate control-plane, CLI-token, and app-viewer credentials.
- One host-only global opaque viewer identity on the platform host plus
  host-only, app-scoped opaque viewer sessions issued through app-bound
  one-time handoffs.
- Private app policies with owner, exact email, and email-domain rules.
- Immediate policy/session/deployer/app revocation.
- Generic responses that resist app and email enumeration.

#### Apps and deployments

- Static HTML/CSS/JavaScript/assets only.
- Stable app hostname under one configured wildcard suffix.
- A generated `tiny.yaml` receipt plus secure defaults and CLI flags for
  repeatable automation.
- Streaming, quota-limited archive upload.
- Hostile archive inspection and staged extraction.
- Immutable release manifests and content hashes.
- Atomic activation and failure recovery.
- SPA fallback, MIME, range, ETag, and safe cache behavior.
- Public gateway denial probes before deployment success.

#### App SDK and built-in capabilities

- Browser-first TypeScript/ESM package: `@tinyhost/sdk`.
- Current app information and capability discovery.
- Current viewer identity.
- Rudimentary app-scoped JSON key-value storage.
- Lightweight app-scoped blob storage with bounded upload, download, list,
  metadata, and delete operations backed by the private local data directory.
- App-scoped WebSocket channels and KV change events.
- Typed errors, versions, limits, cancellation, and compiled examples.
- No SDK secret, database credential, app ID, viewer token, or deployer token.

#### Interfaces for creators and agents

- Separate `tiny` Go client for macOS and Linux.
- Interactive OTP login and protected per-user credential-file token storage.
- Scoped non-interactive agent/CI tokens.
- Deterministic human and JSON CLI output.
- First-party skills for app development, deployment, and operation.
- Executable verification that anonymous requests cannot retrieve app content.

### 5.2 Explicitly excluded from V1

- Arbitrary backend processes, containers, serverless functions, cron, or jobs.
- LLM, Jira, data warehouse, or internal-service adapters.
- Operator/provider secret management beyond TinyHost’s own required secrets.
- Generic authenticated HTTP proxy or raw secret injection.
- Custom domains per app.
- Teams, organizations, advanced roles, billing, marketplace, or discovery.
- Multi-node operation, clustering, high availability, external PostgreSQL,
  Redis, queues, or remote object-storage backends.
- Operator backups, remote disaster recovery, or durability guarantees suitable
  for business-critical apps.
- Docker as the primary installation.
- Windows server support.
- Durable event streams, history, replay, delivery guarantees, or multi-node
  realtime.

### 5.3 Product disclaimer

V1 is for replaceable toy, prototype, and utility applications. Losing the VPS
can lose app releases, KV state, and blobs. The UI and documentation must state
this before operators or deployers store important data.

## 6. Actors and permissions

### 6.0 Terminology

Role names describe authority, not necessarily distinct people. One person may
act in more than one role, but authority, credentials, and permissions never
carry from one role to another.

- **Operator:** a person responsible for hosting and operating TinyHost.
  Operators install, configure, update, diagnose, and recover the server and
  manage deployer authorization.
- **Deployer:** a person authorized to create, deploy, and manage their own Tiny
  apps. TinyHost is designed so deployers do not need infrastructure knowledge,
  although a deployer may still be technically skilled.
- **Viewer:** a person who accesses and interacts with a deployed app. Viewer
  access grants no deployment or operator authority.
- **User:** a neutral umbrella term for any person interacting with TinyHost.
  It implies no role, permission, ownership, or credential type.
- **Deployment agent:** non-human automation acting through a scoped token on
  behalf of a deployer. A deployment agent is not included by the term
  “user” unless explicitly stated.

Use `user` only when the person’s role is irrelevant. When a request,
requirement, contract, or interface depends on permissions, ownership,
credentials, or available actions and the role is unclear, ask whether it means
an operator, deployer, or viewer. Authorization code and contracts must use the
specific role or actor type rather than granting meaning to the generic term
`user`.

### 6.1 Operator

The operator controls the VPS and TinyHost trust root.

May:

- initialize and update TinyHost;
- reconcile the exact active deployer allowlist by normalized email;
- inspect and suspend every app;
- configure global resource and security limits;
- inspect audit and service health;
- perform root-only operator recovery;
- create future provider connections.

May not obtain a remote HTTP recovery bypass. Root access is required when
normal operator authentication is unavailable.

### 6.2 Deployer

A deployer owns only their apps, policies, releases, and scoped tokens.

May:

- authenticate to the CLI/dashboard;
- create apps and stable slugs;
- upload, inspect, and activate releases;
- edit their apps’ access rules;
- inspect bounded app/deployment logs and usage;
- create app-scoped or otherwise limited agent tokens;
- delete their apps through a confirmed lifecycle.

May not:

- inspect or mutate another deployer’s app;
- manage deployers or server configuration;
- read platform/provider secrets;
- access the host filesystem or bind ports;
- convert viewer access into deployment authority.

### 6.3 Viewer

A viewer is an email identity, not necessarily a platform user.

May:

- authenticate one email identity for a browser profile through OTP;
- receive host-only sessions scoped to allowed apps without repeating OTP;
- access content and capabilities allowed by current app policy.

May not:

- deploy or administer because they can view;
- reuse an App A session on App B;
- choose their server-side identity or app ID.

### 6.4 Deployment agent

An agent acts through a hashed-at-rest scoped token.

Scopes may include:

```text
app:read
deploy:create
deploy:activate
access:read
access:write
logs:read
```

Tokens may be restricted by app, operation, expiry, and app-creation count.
Skills require agents to independently verify anonymous denial before reporting
success.

## 7. Canonical user journeys

### 7.1 Clean-host installation

Prerequisites:

- clean supported Hetzner VPS with root SSH access;
- one operator-controlled domain; and
- access to a Resend account and the domain’s DNS control plane.

Platform/wildcard DNS, Resend domain verification, and the API key are required
external state, but the human assistant derives and presents their exact values
at resumable checkpoints rather than requiring the operator to prepare them
from config documentation.

Journey:

```text
curl thin installer | sudo sh
→ detect OS/architecture
→ download tinyhost
→ verify checksum and signature
→ tinyhost setup
→ discover host, ports, time, disk, and supported OS
→ ask only for the base domain, operator email, and Resend credential source
  that cannot be derived
→ derive conventional platform/app hosts and sending defaults; allow an
  explicit edit only when the operator needs a non-default topology
→ show the exact DNS records and pause until their public values are correct
→ create service user, generated config, credentials, and data directory
→ initialize SQLite and operator
→ test Resend
→ provision TLS
→ enable/start systemd service
→ run platform health, route-classification, socket-confinement, and safe
  unknown-app-host denial probes
→ print dashboard and recovery commands
```

Meaningful setup logic belongs in the signed binary. The shell script only
downloads, verifies, installs, and invokes it. Initialization is resumable and
idempotent. The generated config records the resulting server state for
automation and recovery; the operator does not have to author it before setup.
Secrets are never accepted in argv or ordinary config. The assistant accepts a
root-readable credential file, or creates the root-owned credential file from a
no-echo prompt without echoing or logging the value.

### 7.2 Authorize a deployer

```text
operator signs in
→ opens the single active-deployer allowlist
→ reviews all currently active normalized emails in one multiline field
→ adds or removes addresses
→ confirms only when the edit broadens deployment authority
→ TinyHost revision-checks and atomically reconciles users, credentials, and
  one audit event
```

No invitation email is required: the operator can share the platform URL
through any existing channel and the deployer requests OTP when ready. Removing
an active address immediately revokes its CLI tokens and control sessions while
preserving its immutable identity and owned apps. Stale snapshots and audit
failure leave the entire allowlist unchanged. An email address that is not
active cannot receive a deployer token, create an app, upload a release, or
mutate access policy.

### 7.3 Deployer CLI login

```text
tiny login --server https://tiny.example.com
→ load the server-bound credential file
→ authenticated whoami reuses a valid bearer and confirms identity
→ otherwise, only an unauthorized/expired bearer falls back to OTP
→ prompt for email, generic OTP request response, and prompt for code
→ server verifies current deployer authorization and issues a server-bound token
→ authenticated whoami confirms the new identity
→ atomically save the new bearer in the protected per-user Tiny credential file
→ save the normalized HTTPS platform URL as the Tiny default server
```

The token is displayed only when explicitly using a non-interactive token
creation workflow and is never logged. Every mutating CLI request sends this
token to the platform host; the server re-evaluates deployer status, token
scope, expiry, and target ownership before acting.

Interactive login persists the bearer by normalized HTTPS platform URL in one
Tiny-owned per-user file at `os.UserConfigDir()/tiny/<sha256(server)>.json`.
The credential directory is mode `0700`, its regular non-symlinked file is
mode `0600`, and updates use atomic replacement in that directory. Its bounded,
versioned JSON format has the exact `version`, `server`, and `token` members;
unknown or duplicate JSON members and a server mismatch deny. The raw bearer
never appears in argv, environment variables, human/JSON output, logs, or a
project file.
`tiny login` is consequently a one-time interactive action for a still-valid
token; later invocations first reuse it silently after `whoami`. An
unauthorized or expired token falls back to a normal CLI OTP login. Transport,
dependency, malformed-response, unexpected-status, or ambiguous authorization
failures fail closed without sending OTP or altering the existing credential.
`tiny login --force` is the explicit account-switch flow: it skips reuse,
requires fresh OTP, and replaces the old credential only after the new bearer
passes `whoami`; failure retains the old credential. A `429` tells the deployer
to wait and retry but never reveals email eligibility, token state, or whether
an OTP was created. The file is readable by the same OS user, unlike an OS
credential store, so server-side scopes, expiry, and prompt revocation remain
mandatory safeguards.

The same protected Tiny configuration directory keeps one separate non-secret
default server record (`default-server.json`) with exact versioned normalized
HTTPS URL data. After successful login, later deployer CLI commands may omit
`--server`; an explicit `--server` wins only for that invocation and cannot
change the default. For a recognized human command, exactly missing default
state offers one bounded server-setup prompt. Tiny verifies the proposed
normalized HTTPS URL through a direct no-redirect API-version proof before
saving it, then continues the original command; authentication remains
separate, so that command may next report `Login required`. JSON commands
never prompt or cache. Malformed, unsafe, non-HTTPS, incompatible, redirected,
transport-failed, or ambiguous default/setup state fails closed rather than
selecting a host or running the target command.

`tiny logout` sends the current global CLI bearer to an authenticated
self-revocation route. The server derives and revokes exactly that bearer row;
it never revokes browser control sessions, viewer sessions, app-scoped tokens,
or another CLI bearer. The CLI removes only the matching local credential after
confirmed revocation, or after a `401` proves it is already unusable. Ambiguous
network/server/persistence/local-storage failures retain the credential; a
missing local credential is idempotently logged out and leaves the default URL
available for the next `tiny login`.

### 7.4 Deploy an app

Recommended V1 behavior:

```text
creator/deployer has a static project
→ tiny deploy [project directory]
→ reuse valid existing output, or stop once with the exact project-owned build
  action when output is absent
→ use the verified saved platform and bearer when available
→ inspect the project and infer safe app slug/output defaults
→ if required state is missing or ambiguous, human CLI asks one bounded
  question at a time
→ default to owner-only access and no unproven capability
→ show one deployment summary with an edit path for optional description,
  viewer rules, capabilities, or SPA fallback
→ one final action names any access broadening and starts deployment
→ if tiny.yaml is missing, atomically write the generated receipt without a
  second manifest confirmation
→ prove saved deployer bearer with whoami; only unauthorized state uses OTP
→ load tiny.yaml and flags
→ validate directory and create deterministic archive
→ stream upload with idempotency key
→ server validates policy before activation
→ inspect/extract archive in staging
→ create immutable release manifest
→ ensure the exact app HTTPS origin has a verified certificate (bounded retry window)
→ atomically activate
→ probe anonymous HTML/asset/API denial
→ probe authenticated platform health
→ report protected URL + deployment ID + policy summary
```

The CLI does not execute arbitrary build scripts in V1. Coding agents or the
developer’s existing toolchain produce the deploy directory.

`tiny init [DIR]` is the explicit equivalent of the missing-manifest setup and
never overwrites an existing file. The human path does not ask the deployer to
author YAML or repeat a server URL, email, slug, output directory, or policy
already verified or safely inferred. `tiny deploy` defaults to the current project
directory; `--json` never prompts, creates a manifest, starts OTP, or changes
credentials. A generated manifest is strict and deterministic, and all build
output/fallback paths are checked to remain inside the local project without
symlinks before archiving.

First certificate issuance may take longer than one probe. Before activation,
TinyHost therefore retries only the exact app-origin certificate-readiness
check within one cancellable, finite 45-second budget. Redirects, wrong hosts,
unverified chains, cancellation, or exhaustion still deny activation and retain
the prior release; policy and post-activation evidence never inherit this retry.

### 7.5 Viewer login

```text
viewer opens app URL
→ gateway resolves active app
→ no valid app session
→ server-created app-bound handoff to platform identity broker
→ existing global viewer identity, or generic OTP when absent
→ current policy rechecked at grant issue and callback consumption
→ create app-scoped session only for that resolved app
→ redirect to safe relative app path
```

No app asset is read while presenting or processing login.

### 7.6 App SDK request

```text
app JavaScript calls @tinyhost/sdk
→ same-origin reserved endpoint
→ gateway resolves app from host
→ validate app-scoped session
→ load current app policy
→ produce AuthorizationContext
→ validate capability/version/input/quota
→ repository query includes server-derived AppID
→ return typed bounded result
```

### 7.7 App realtime connection

```text
app calls tiny.live.channel("updates").subscribe(...)
→ open same-origin WebSocket at /_tiny/ws/v1
→ gateway resolves active app from host
→ validate app-scoped session and current policy before upgrade
→ bind connection to AppID + IdentityID + SessionID
→ validate channel and connection limits
→ subscribe/publish only within the bound app
→ close connection when app, policy, or session is revoked
```

The in-memory hub provides best-effort live delivery only. Reconnects and server
restarts can lose events. Clients read current KV state after connecting or
reconnecting; realtime is a notification layer, not the source of truth.

### 7.8 Server update

```text
sudo tinyhost update
→ check release compatibility
→ download and verify signed artifact
→ create bounded local update-rollback state
→ drain and stop service
→ apply binary and compatible migrations
→ restart
→ health + anonymous-denial probes
→ commit update or automatically restore prior version/state
```

No unattended silent updates in V1.

## 8. Functional requirements

Requirement keywords use MUST, SHOULD, and MAY in their normal normative sense.

### 8.1 Installation and operations

- **FR-OPS-001:** `tinyhost` MUST be a self-contained server executable.
- **FR-OPS-002:** Installation MUST support Hetzner Ubuntu 24.04 LTS and 26.04
  LTS on x86-64 and create a dedicated unprivileged service user. Other,
  interim, end-of-life, and unverified Ubuntu releases MUST fail preflight.
- **FR-OPS-003:** Only TinyHost MUST listen publicly on ports 80 and 443.
- **FR-OPS-004:** `tinyhost init` MUST be safely resumable after interruption.
- **FR-OPS-005:** `tinyhost doctor` MUST diagnose DNS, TLS, Resend, database,
  disk, permissions, clock, ports, version, and update health without exposing
  secrets.
- **FR-OPS-006:** Root recovery MUST allow replacing the operator email and
  revoking control sessions without email access.
- **FR-OPS-007:** Updates MUST verify signed artifacts and pass a post-restart
  health gate before deleting rollback state.
- **FR-OPS-008:** V1 MUST NOT claim backup or disaster-recovery guarantees.

### 8.2 Host and app routing

- **FR-ROUTE-001:** The platform host MUST match exactly.
- **FR-ROUTE-002:** An app host MUST be exactly one valid label below the
  configured app suffix.
- **FR-ROUTE-003:** Host input MUST be canonicalized for port, case, trailing
  dot, encoding, and trusted-proxy behavior.
- **FR-ROUTE-004:** Unknown, malformed, missing, suspended, deleting, and failed
  apps MUST NOT dispatch app content.
- **FR-ROUTE-005:** Reserved `/_tiny/*` routes MUST never fall through to app
  static files or SPA fallback.

### 8.3 OTP authentication

- **FR-AUTH-001:** V1 MUST support numeric email OTP login through Resend.
- **FR-AUTH-002:** OTP request responses MUST not reveal policy membership.
- **FR-AUTH-003:** Challenges MUST be random, short-lived, one-time, hashed at
  rest, attempt-limited, and invalidated by a newer challenge.
- **FR-AUTH-004:** Limits MUST apply by IP/fingerprint, email hash, app, and
  globally before expensive work.
- **FR-AUTH-005:** OTP verification MUST re-evaluate current authorization before
  creating a session.
- **FR-AUTH-006:** Challenge consumption and session creation MUST be atomic.
- **FR-AUTH-007:** Existing valid sessions MAY continue during a Resend outage;
  new authentication MUST fail closed.
- **FR-AUTH-008:** A successful viewer OTP MAY establish exactly one opaque,
  platform-host global viewer identity per browser profile. It MUST grant no
  app or control authority without a separate current authorization check.
- **FR-AUTH-009:** An app handoff MUST be server-created, state-bound,
  app-bound, one-time, and no longer than five minutes. Its issue and consume
  paths MUST re-evaluate current app policy.

### 8.4 Sessions

- **FR-SESSION-001:** Viewer sessions MUST be opaque and server-side.
- **FR-SESSION-002:** Viewer sessions MUST be scoped to exactly one app.
- **FR-SESSION-003:** App cookies MUST be host-only, `Secure`, `HttpOnly`,
  `SameSite=Lax`, and `Path=/`.
- **FR-SESSION-004:** Control-plane sessions MUST use a distinct type and cookie.
- **FR-SESSION-005:** CLI/agent tokens MUST never be accepted as viewer sessions.
- **FR-SESSION-006:** Revoked or expired sessions MUST fail the next request.
- **FR-SESSION-007:** The global viewer identity cookie MUST be host-only on
  the platform host, opaque, `Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`,
  and never use `Domain`, JWT, or browser storage.
- **FR-SESSION-008:** Global viewer identity is not a control-plane credential.
  Global sessions last at most 30 days, rotate every 24 hours, and accept the
  immediately prior secret for no more than 60 seconds.
- **FR-SESSION-009:** Global identity switch revokes the prior identity family
  and all derived app sessions before issuing the replacement identity; per-app
  logout revokes only its app session.

### 8.5 Access policies

- **FR-POLICY-001:** An active private app MUST have an activatable current
  policy revision.
- **FR-POLICY-002:** Absence or corruption of policy MUST mean deny.
- **FR-POLICY-003:** V1 rules MUST support owner, exact normalized email, and
  normalized email domain.
- **FR-POLICY-004:** An active deployer owner MUST be an implicit viewer of
  their app. `tiny deploy .` with no viewer rules creates an owner-only private
  app.
- **FR-POLICY-005:** Policy mutations MUST be revisioned, transactional, audited,
  and effective on the next request.
- **FR-POLICY-006:** V1 MUST expose no public-app mode. The policy model MAY
  reserve a future enum value, but absence or corruption always denies.

### 8.6 Deployer and token authorization

- **FR-RBAC-001:** Operator and deployer authority MUST be separate from viewer
  identity.
- **FR-RBAC-002:** Only an operator-authorized active deployer email may complete
  CLI login and receive a deployer token.
- **FR-RBAC-003:** Every control command MUST declare actor, permission, target,
  transaction boundary, audit event, and idempotency behavior.
- **FR-RBAC-004:** API tokens MUST be random, hash-at-rest, display-once, scoped,
  expirable, revocable, and record safe last-used metadata.
- **FR-RBAC-005:** Repository and service tests MUST prove deployer A cannot
  observe or mutate deployer B’s resources.

### 8.7 Deployment pipeline

- **FR-DEPLOY-001:** The client MUST upload only an explicit directory.
- **FR-DEPLOY-002:** Upload MUST stream to a quota-limited non-served staging
  location while hashing content.
- **FR-DEPLOY-003:** Extraction MUST reject traversal, absolute paths, links,
  devices, sockets, unsafe permissions, collisions, excessive entries/depth,
  oversized files, and expansion bombs.
- **FR-DEPLOY-004:** Deployment states MUST be explicit and transition through
  validated domain operations.
- **FR-DEPLOY-005:** A release directory MUST become immutable after validation.
- **FR-DEPLOY-006:** A deployment MUST NOT activate before current policy and TLS
  readiness exist.
- **FR-DEPLOY-007:** Activation MUST serialize per app and preserve the previous
  release until verification succeeds.
- **FR-DEPLOY-008:** Upload and activation MUST support idempotency.
- **FR-DEPLOY-009:** The CLI MUST NOT report success until public gateway probes
  verify anonymous denial.

### 8.8 Static runtime

- **FR-STATIC-001:** Authorization MUST occur before path resolution or file
  opening.
- **FR-STATIC-002:** The static runtime MUST accept an
  `AuthorizationContext`, not an untrusted app ID.
- **FR-STATIC-003:** Files MUST open beneath the immutable release root without
  following symlinks.
- **FR-STATIC-004:** SPA fallback MUST remain inside the release and MUST never
  apply to reserved platform paths.
- **FR-STATIC-005:** Dotfiles, source maps, directory listings, MIME sniffing,
  cache rules, range requests, and error documents MUST have explicit tested
  behavior.
- **FR-STATIC-006:** Error responses MUST not expose host paths, release IDs,
  SQL, policy membership, or stack traces.

### 8.9 Client SDK

- **FR-SDK-001:** `@tinyhost/sdk` MUST be the documented default app interface.
- **FR-SDK-002:** The core SDK MUST be browser-first TypeScript, ESM,
  framework-neutral, tree-shakeable, and dependency-light.
- **FR-SDK-003:** The SDK MUST expose current user, app information, capability
  discovery, KV, blobs, and live channels.
- **FR-SDK-004:** The SDK MUST use same-origin relative endpoints.
- **FR-SDK-005:** The SDK MUST NOT accept an app ID or any database/provider/
  platform secret.
- **FR-SDK-006:** Methods MUST support cancellation where network work occurs.
- **FR-SDK-007:** Failures MUST map to stable typed categories:
  `NotAuthenticated`, `NotAuthorized`, `CapabilityUnavailable`,
  `ValidationFailed`, `VersionConflict`, `QuotaExceeded`, `RateLimited`, and
  `TemporarilyUnavailable`.
- **FR-SDK-008:** Errors MUST include a safe request ID.
- **FR-SDK-009:** Server and SDK MUST negotiate capability/API versions and
  return actionable incompatibility errors.
- **FR-SDK-010:** Public examples MUST compile and execute as contract tests.

### 8.10 Key-value capability

- **FR-KV-001:** The SDK MUST expose `tiny.kv.get`, `set`, `delete`, and bounded
  prefix `list`.
- **FR-KV-002:** Keys MUST be UTF-8 strings with explicit length and character
  constraints. Values MUST be JSON.
- **FR-KV-003:** App ID MUST come from `AuthorizationContext`.
- **FR-KV-004:** Every primary/unique lookup MUST begin with app ID.
- **FR-KV-005:** Key count, prefix/list bounds, value size, total app storage,
  and request rates MUST be limited.
- **FR-KV-006:** Mutations MUST support optimistic concurrency/versioning.
- **FR-KV-007:** Successful mutations MUST emit app-scoped best-effort
  `kv.created`, `kv.updated`, or `kv.deleted` live events after commit.
- **FR-KV-008:** Cross-app reads/writes MUST fail at HTTP, service, and
  repository layers.
- **FR-KV-009:** The API MUST NOT expose SQL, schema migration, connection
  string, or database-file concepts.
- **FR-KV-010:** V1 persistence MUST be described as utility-grade, not
  business-critical durability.

### 8.11 Realtime capability

- **FR-LIVE-001:** The SDK MUST expose app-scoped WebSocket channels with
  subscribe, unsubscribe, publish, reconnect status, and close.
- **FR-LIVE-002:** The HTTP gateway MUST authenticate and authorize before
  upgrading a connection.
- **FR-LIVE-003:** App, viewer identity, and session MUST be server-derived and
  permanently bound to the connection.
- **FR-LIVE-004:** Clients MUST NOT select or publish across app IDs.
- **FR-LIVE-005:** Channel names, message bytes, publish rate, connections per
  viewer/app, subscriptions per connection, and idle lifetime MUST be limited.
- **FR-LIVE-006:** App suspension, policy revocation, and session revocation MUST
  close affected active connections promptly on the same node.
- **FR-LIVE-007:** The hub MUST be in-memory and single-node in V1 with no event
  history, replay, ordering across publishers, or delivery guarantee.
- **FR-LIVE-008:** Clients MUST recover from disconnect by reading current KV
  state and resubscribing; events are hints, not durable state.
- **FR-LIVE-009:** Unknown/reserved channels and malformed messages MUST fail
  closed without terminating unrelated app connections.
- **FR-LIVE-010:** Origin, session, policy, and channel tests MUST cover the
  upgrade and ongoing message lifecycle.

### 8.12 Lightweight blob capability

- **FR-BLOB-001:** The SDK MUST expose bounded `upload`, `get`, `list`, and
  `delete` operations. V1 has no replace, folders, public/signed URLs,
  resumable or multi-request chunked upload, transformations, thumbnails,
  search, or per-viewer ACL model.
- **FR-BLOB-002:** The app ID, viewer identity, and storage key MUST be derived
  by the server. Callers supply display metadata and bytes, never an app ID,
  filesystem path, bucket, endpoint, or storage credential.
- **FR-BLOB-003:** A blob belongs to one app-wide shared namespace, matching V1
  KV semantics. Every currently authorized viewer of an app with the enabled
  blob capability may upload, read, list, and delete blobs in that app.
- **FR-BLOB-004:** Blob IDs MUST be opaque and server-issued. Original
  filenames are bounded display metadata only and MUST NOT participate in
  filesystem path construction or authorization.
- **FR-BLOB-005:** Uploads MUST stream through private bounded staging and
  become readable only after byte storage and SQLite metadata reach a committed
  `ready` state. Partial, staging, deleting, orphaned, or inconsistent records
  MUST never be served.
- **FR-BLOB-006:** Downloads MUST pass through the authenticated gateway and
  use attachment-oriented, no-sniff response handling. V1 exposes no raw
  filesystem, mount, bucket, or independently usable object URL.
- **FR-BLOB-007:** Per-file bytes, total app bytes, object count, list page,
  metadata, upload rate, and request duration MUST be bounded. Disk write-stop
  and storage-source failure deny upload without blocking safe reads or
  revocation.
- **FR-BLOB-008:** Every metadata lookup and storage key MUST begin from the
  server-derived app ID. Cross-app reads, lists, deletes, guessed IDs, and
  client-selected keys MUST fail at gateway, service, repository, and storage
  layers.
- **FR-BLOB-009:** Cleanup and crash recovery MUST reconcile SQLite and storage
  from server-derived IDs, never a client path. An unavailable backend or
  disagreement fails closed and remains diagnosable without revealing private
  paths or provider details.
- **FR-BLOB-010:** V1 uses a private local-filesystem storage adapter behind a
  narrow internal blob-store interface. FUSE mounts, a second storage server,
  remote object stores, provider credentials, and cloud durability claims are
  excluded from V1. Blob support MUST preserve the production shape of one
  `tinyhost` process, one SQLite database, one private data directory, and one
  systemd service.

### 8.13 Admin and deployer web UI

- **FR-UI-001:** UI MUST use embedded server-rendered HTML with minimal local
  JavaScript.
- **FR-UI-002:** Login UI MUST exist on platform and app origins without sharing
  ambient credentials.
- **FR-UI-003:** Operator UI MUST support deployers, apps, policies, audit,
  limits, email/TLS diagnostics, disk health, version/update state, and a
  bounded recent host CPU/RAM/data-volume resource chart.
- **FR-UI-004:** Deployer UI MUST support their apps, deployments, policies,
  tokens, usage, suspension/deletion, and actionable failures.
- **FR-UI-005:** Sensitive values MUST be write-only or display-once.
- **FR-UI-006:** Destructive or access-broadening actions MUST show the exact
  target and require confirmation.

### 8.14 Agent skills

- **FR-SKILL-001:** The repository MUST first create one generic Tiny platform
  skill containing the canonical shared principles, SDK/capability model,
  authentication, deployment, and verification workflow.
- **FR-SKILL-002:** The repository MUST ship one self-contained deployer skill
  spanning app development through verified deployment and one self-contained
  operator skill by copying the relevant shared sections from the generic
  skill. Runtime inheritance between installed skills is forbidden.
- **FR-SKILL-003:** Skills MUST route to versioned SDK, manifest, capability,
  security, and troubleshooting references.
- **FR-SKILL-004:** Skills MUST include deterministic verification scripts.
- **FR-SKILL-005:** Skills MUST prohibit embedded secrets, selectable app IDs,
  auth reimplementation, and success after failed denial probes.
- **FR-SKILL-006:** CI MUST detect drift between the generic skill, copied
  specialized sections, exported SDK, manifest, CLI, and HTTP contracts.
- **FR-SKILL-007:** Skills MUST use generic agent-readable Markdown and
  executable scripts; a Codex-compatible `SKILL.md` packaging is the first
  distribution format but not the only usable documentation.

## 9. Architecture requirements

### 9.1 Deployment topology

```text
Internet
  ↓ ports 80/443
tinyhost modular Go monolith
  ├── control-plane router
  ├── app-plane gateway
  ├── auth/policy/session services
  ├── deployment/static runtime
  ├── app capability services
  ├── in-memory app-scoped realtime hub
  ├── embedded web UI
  ├── SQLite
  ├── private release filesystem
  └── outbound Resend + ACME only
```

There are no internal HTTP microservices in V1.

### 9.2 Dependency direction

```text
entry points and provider adapters
        ↓
application command/query services
        ↓
domain policies and state machines
        ↓
repository/filesystem/provider interfaces
```

Domain code must not import HTTP, SQL-driver, Resend, ACME, CLI, or web-template
packages.

### 9.3 Future repository structure

```text
tiny/
├── AGENTS.md
├── PRINCIPLES.md
├── PRD.md
├── go.mod
├── cmd/
│   ├── tinyhost/
│   └── tiny/
├── internal/
│   ├── gateway/
│   ├── appauth/
│   ├── identity/
│   ├── otp/
│   ├── sessions/
│   ├── policies/
│   ├── apps/
│   ├── uploads/
│   ├── archive/
│   ├── releases/
│   ├── verification/
│   ├── staticruntime/
│   ├── capabilities/
│   ├── kv/
│   ├── live/
│   ├── control/
│   ├── tokens/
│   ├── audit/
│   ├── config/
│   ├── certificates/
│   ├── update/
│   ├── jobs/
│   └── persistence/
├── migrations/
├── web/
├── sdk/
│   └── typescript/
├── skills/
│   ├── tiny-platform/
│   ├── tiny-deployer/
│   └── tiny-operator/
├── specs/
├── docs/
└── test/
```

Packages may be combined when early behavior is small. Boundaries and dependency
direction matter more than directory count.

## 10. Request and data flow

### 10.1 Protected app request

```text
request
→ canonicalize host
→ classify platform/app/unknown
→ resolve active app
→ classify pre-auth/reserved/protected endpoint
→ validate app-scoped session
→ load current policy
→ evaluate identity
→ construct AuthorizationContext
→ dispatch to static/KV/capability handler
```

Errors before authorization cannot dispatch protected content.

### 10.2 Pre-authentication request

Login/OTP routes require an active resolved app but no existing session. They
receive no release or KV repository dependency. Rate limits and generic
responses occur before provider work.

### 10.2a Global viewer identity handoff

```text
app document request without app session
→ derive active app from hostname and create state-bound handoff
→ redirect only to platform-host identity route
→ validate platform-host global viewer identity or complete generic OTP
→ re-evaluate app policy and authorize one-time app-bound handoff
→ exact app callback atomically consumes handoff and rechecks policy
→ create host-only app session and redirect to safe relative path
```

The platform-host identity cookie is never sent to the app host. The app
callback accepts no client-selected app or absolute return URL. Rejected,
pre-auth, and callback paths serve no app bytes.

### 10.3 Control-plane mutation

```text
platform/CLI request
→ authenticate operator/deployer/token
→ authorize role + target resource + scope
→ validate idempotency and input
→ application service
→ transactional metadata + audit event
→ optional durable filesystem state machine
→ stable response/read model
```

### 10.4 Realtime connection and event flow

```text
HTTP upgrade request
→ normal host/session/current-policy authorization
→ validate WebSocket origin and limits
→ create LiveConnection(AppID, IdentityID, SessionID)
→ subscribe/publish inside AppID namespace
→ committed KV mutation emits best-effort app-local event
→ revocation signal closes matching connections
```

Connection state lives only in the running process. SQLite remains the source of
truth for KV. Reconnect begins with a fresh authorization evaluation and state
read.

### 10.5 Filesystem/database coordination

SQLite and the filesystem cannot share a transaction. Deployments and updates
therefore use:

- explicit durable intermediate states;
- deterministic IDs and release paths;
- content manifests and hashes;
- idempotent retry/recovery;
- cleanup that treats database state as authority;
- interruption tests before and after every durable transition.

## 11. State model

### 11.1 Application

```text
creating → active ↔ suspended → deleting → permanently removed
    └────→ failed
```

Only `active` resolves on the app plane.

### 11.2 Deployment

```text
uploading → uploaded → validating → staged → verified → active → superseded
    └────────────── rejected/failed ───────────────────────────────┘
```

Rejected means invalid user input. Failed means an operational stage could not
complete. Both are terminal; retry creates a related attempt.

### 11.3 OTP

```text
pending → consumed | expired | locked | invalidated
```

Concurrent verification can create at most one session.

### 11.4 Session and token

```text
active → expired | revoked | rotated
```

Authentication state does not cache continuing authorization.

### 11.5 Server update

```text
checking → downloaded → verified → rollback-ready → activating → healthy
                                                    └→ failed → rolled-back
```

Migrations must declare whether automatic rollback is safe.

### 11.6 Live connection

```text
connecting → active → reconnecting | closed
                 └── policy/session/app revoked → closed
```

Subscription membership and outbound queues live in the in-memory hub. They are
never restored after process restart.

## 12. Logical data model

Required V1 entities:

```text
users
identities
api_tokens
applications
access_policies
access_rules
otp_challenges
sessions
identity_sessions
identity_handoffs
deployments
deployment_files
app_kv
app_quota_usage
audit_events
outbox
jobs
schema_migrations
```

Required constraints:

- normalized deployer email unique for active users;
- app slug unique under the configured app suffix;
- every active app references a valid policy and deployment;
- session type/app/user/identity combinations are valid;
- global identity sessions have one identity and family, no app/control user;
- app sessions have one app and identity and can be linked to a global identity
  family; handoffs bind exactly one server-derived app and are consumed once;
- identity-link migrations are additive: runtime validation quarantines a
  parentless app session created before the recorded migration cutoff without
  mutating its `revoked_at`; post-cutoff explicitly brokerless sessions retain
  their app-local semantics and malformed cutoff/session timestamps deny;
- deployment belongs to exactly one app;
- KV primary key begins with app ID;
- token secret hash is unique and plaintext is never persisted;
- audit metadata follows action-specific redacted schemas.

Transaction boundaries include:

- OTP consumption plus global identity/session-family creation or app-session
  creation, as applicable;
- handoff consumption plus policy recheck plus app-session creation;
- policy revision plus current revision swap plus audit;
- deployment activation plus current deployment swap plus audit;
- token create/revoke plus audit;
- KV mutation plus version/quota accounting, followed after commit by a
  best-effort in-memory change event.

## 13. UI requirements and states

### 13.1 General interaction rules

- Server truth owns auth, policies, deployments, quotas, and update state.
- Forms render existing safe read models and submit explicit commands.
- Busy mutations disable duplicate submission and show the target operation.
- Errors preserve safe entered values and provide retry/diagnostic action.
- Lists distinguish loading, empty, content, stale/refreshing, and unavailable.
- Access-broadening changes visibly summarize the resulting policy.
- No security-relevant action occurs as a template/render side effect.

### 13.2 Login states

```text
email entry
→ requesting (disabled form)
→ generic code-sent state
→ code verification
→ success redirect
   or invalid/expired/limited/unavailable with safe retry
```

The UI never reveals whether the email is allowlisted.

### 13.3 App and deployment states

- Empty: explain `tiny deploy` and show no misleading health state.
- Uploading/validating: show current stage and cancellation limitations.
- Failed/rejected: stable reason code, safe message, retry as new attempt.
- Active: URL, policy summary, release, TLS, protection-probe status.
- Suspended: no content access; explain operator/deployer action.
- Rolling back: keep prior state visible until verified result.
- Deleting: disable deployment/policy mutations and state recoverability.
- Live connected/reconnecting/offline: preserve rendered KV state, show a
  non-blocking connection indicator, and resubscribe/read current state after
  reconnect.

### 13.4 Operator health states

- Healthy.
- Degraded but fail-closed: email or ACME/provider problem.
- Disk warning: writes near limit.
- Write-disabled: critical disk watermark.
- Update available.
- Update verifying/applying/rolled back.
- Database/migration failure requiring local root action.
- Resource history unavailable, which is displayed as unavailable rather than
  as zero use or a healthy measurement.

V1 UI uses restrained transitions and progressive status disclosure; visual
animation is secondary to explicit durable state.

## 14. Configuration and secrets

Non-secret typed configuration:

```yaml
server:
  platform_domain: tiny.example.com
  app_domain: apps.example.com
  listen_http: 0.0.0.0:80
  listen_https: 0.0.0.0:443

data:
  directory: /var/lib/tinyhost

email:
  provider: resend
  api_key_secret: resend_api_key
  from: TinyHost <access@example.com>

auth:
  otp_expiry: 10m
  session_expiry: 24h
  max_attempts: 5
```

TinyHost’s own secrets:

- Resend API key;
- session/challenge hashing keys;
- future capability encryption root;
- release/update trust configuration where applicable.

Secrets live in root-owned credential files or systemd credentials, not SQLite
as plaintext and not the normal YAML configuration.

## 15. Default limits

Defaults must be operator-configurable within safe bounds. Initial recommended
values require benchmark validation:

| Limit | Recommended V1 default |
|---|---:|
| Apps per deployer | 20 |
| Archive upload | 100 MB |
| Expanded release | 250 MB |
| Files per release | 10,000 |
| Single file | 50 MB |
| Deployment attempts | 20/hour/deployer |
| Viewer OTP expiry | 10 minutes |
| OTP attempts | 5 |
| Viewer session | 24 hours |
| Global viewer identity | 30 days absolute; rotate every 24 hours |
| Global identity prior-token overlap | 60 seconds maximum |
| App handoff | 5 minutes maximum |
| Active CLI token default | 30 days |
| KV key | 256 bytes |
| KV JSON value | 64 KiB |
| KV keys per app | 10,000 |
| KV total per app | 100 MB |
| Blob file | 25 MB |
| Blob objects per app | 1,000 |
| Blob total per app | 250 MB |
| Blob list page | 100 |
| WebSocket connections per app | 100 |
| WebSocket connections per viewer/app | 5 |
| Subscriptions per connection | 32 |
| WebSocket message | 32 KiB |
| WebSocket publish rate | 20/second/connection |
| WebSocket outbound queue | 100 messages/connection |
| JSON request body | 1 MiB unless narrower |
| Audit retention | 30 days or bounded rows |
| App releases retained | 10 plus active |
| Disk warning watermark | 80% |
| Disk write-stop watermark | 90% |

At the critical disk watermark, new deployments and data writes fail safely;
existing authorized static reads continue when safe.

## 16. Security requirements

### 16.1 Mandatory invariants

1. One public gateway.
2. Authorization before static reads, API invocation, WebSocket upgrade,
   blob access, and proxying.
3. No public storage path or alternate port.
4. Server-derived app identity.
5. Server-derived viewer identity.
6. Immediate policy and session revocation.
7. Viewer/deployer/operator separation.
8. Hostile archive extraction.
9. App-origin isolation and host-only cookies.
10. No provider secret in browser, SDK, logs, audit, or deployment bundle.

### 16.2 Threat classes that require explicit tests

- host/suffix/forwarded-host confusion;
- route registration without authorization;
- cross-app IDs, sessions, deployments, data, and tokens;
- cross-app WebSocket connection, channel, publish, and KV-event delivery;
- OTP enumeration, brute force, replay, and concurrency;
- CSRF, open redirects, stored XSS, and cookie scope;
- traversal, symlink, collision, and archive bombs;
- stale policy or cache invalidation failure;
- disk, memory, goroutine, database-write, and email exhaustion;
- connection, subscription, message, and slow-consumer exhaustion;
- interruption across activation/update durable transitions;
- secret leakage through logs/errors/audit;
- dependency and signed-update compromise.

### 16.3 Logging and privacy

- No request bodies by default.
- Cookies, authorization headers, OTPs, API tokens, provider secrets, and raw
  credentials are forbidden in logs/audit.
- IP addresses are truncated or keyed-hashed where practical.
- Viewer email is visible only where authorization/audit needs it.
- No outbound product analytics or telemetry by default in the recommended
  configuration.

## 17. API and compatibility

### 17.1 HTTP namespaces

Platform host:

```text
/api/v1/auth/*
/api/v1/apps/*
/api/v1/deployments/*
/api/v1/access/*
/api/v1/tokens/*
/api/v1/operator/*
```

App host:

```text
/_tiny/auth/*
/_tiny/api/v1/me
/_tiny/api/v1/app
/_tiny/api/v1/capabilities
/_tiny/api/v1/kv/*
/_tiny/ws/v1
```

Reserved namespaces cannot be shadowed by app files.

### 17.2 Error envelope

```json
{
  "error": {
    "code": "not_authorized",
    "message": "This request is not authorized.",
    "request_id": "req_..."
  }
}
```

Safe stable codes are part of the contract. Internal reasons belong in redacted
logs/audit.

### 17.3 Versioning

- Server HTTP API is versioned.
- Manifest is versioned.
- SDK package uses semantic versions and declares supported API/capabilities.
- Capability modules are independently versionable.
- CLI discovers server compatibility before mutation.
- Additive fields are allowed within V1; authorization meaning is not changed
  without a versioned contract/ADR.

## 18. Distribution and technology stack

### 18.1 Server and CLI

- Go modular monolith.
- Standard `net/http`, `html/template`, `embed`, `crypto`, and context patterns.
- `modernc.org/sqlite` preferred for a CGO-free spike; final choice requires
  concurrency, migration, backup-free update recovery, and security evidence.
- Direct Resend HTTP adapter.
- ACME implementation selected through the M0 spike.
- CLI uses the standard library unless a small framework materially improves
  stable command/help/JSON output.

### 18.2 SDK

- TypeScript source.
- ESM output with type declarations.
- Framework-neutral browser core.
- Standard `fetch` and `AbortSignal`.
- Standards-compliant browser WebSocket client with bounded reconnect behavior.
- Minimal dependencies and bundle-size gate.
- Working package label `@tinyhost/sdk`; branding may replace it without changing
  module boundaries or behavior.

### 18.3 Web UI

- Go server-rendered templates.
- Embedded local CSS and minimal vanilla JavaScript.
- No remote fonts, CDN scripts, frontend build dependency, or separate web
  server required for the first server release.

### 18.4 Release artifacts

- `tinyhost` for supported Linux architectures.
- `tiny` for supported developer/CI platforms.
- Checksums, signatures, provenance, and reproducible-build evidence.
- Thin installation scripts that verify before installing.

## 19. Agent implementation and verification loop

Implementation agents work one vertical milestone slice at a time:

```text
frame outcome and non-goals
→ update contract and threat cases
→ write deny-path charter
→ implement thinnest vertical slice
→ unit/contract/integration/negative tests
→ fuzz/race/failure injection as relevant
→ principle and ADR review
→ update skill examples
→ independent verification evidence
```

Separate roles should be used sequentially or in parallel where available:

- planner;
- implementer;
- adversary;
- reviewer;
- verifier.

No agent may mark a security or deployment feature complete solely because the
happy path works.

## 20. Delivery milestones

### M0 — Architecture proving ground

Spikes:

- canonical host parser and route registry;
- SQLite driver/concurrency/migration/update rollback;
- hostile archive corpus and safe Linux open-beneath behavior;
- app-scoped cookie/browser behavior;
- authenticated WebSocket upgrade, revocation, slow-consumer, and in-memory hub;
- per-host ACME lifecycle;
- real-listener integration harness.

Exit: ADRs accepted/replaced and no blocker to the secure static path.

### M1 — Protected static read path

Deliver:

- config/migrations/apps/identity/OTP/sessions/policy;
- host gateway and typed authorization context;
- protected immutable static runtime;
- minimal operator-seeded state and viewer login.

Exit: anonymous, wrong-app, revoked, suspended, malformed, and dependency-error
requests expose zero release bytes.

### M2 — Deployer control plane

Deliver:

- platform auth and RBAC;
- deployer management;
- app/policy commands;
- scoped tokens;
- dashboard/CLI read models and audit.

Exit: cross-deployer isolation, immediate token revocation, and transactionally
audited mutations.

### M3 — Immutable deployment loop

Deliver:

- manifest/client validation;
- streaming upload and hostile extraction;
- release state machine;
- activation, TLS readiness, public probes, failure recovery, and cleanup.

Exit: `tiny deploy` returns a working protected URL; attack corpus and
interruption recovery pass.

### M4 — SDK, KV, lightweight blobs, and realtime

Deliver:

- current viewer/app;
- capability discovery;
- rudimentary versioned JSON KV;
- bounded app-scoped blob upload, download, list, metadata, and delete backed by
  the private local data directory;
- in-memory app-scoped WebSocket channels and KV change events;
- TypeScript SDK and examples;
- generic platform skill followed by self-contained specialized skill copies.

Exit: two-app KV/blob/live isolation, quotas/conflicts, partial-write and
storage-disagreement recovery, reconnect/current-state recovery, revocation
disconnects, browser security, SDK compatibility, and skill tasks pass.

### M5 — Operable Hetzner-first release

Deliver:

- installer/init/status/doctor/root recovery;
- systemd packaging;
- signed self-update/rollback;
- limits, cleanup, diagnostics, admin UI, and a bounded operator-only host
  CPU/RAM/data-volume chart;
- release signing/provenance and all skills.

Exit: clean Hetzner install, failed-update rollback, socket inventory, complete
security matrix, and release checklist pass.

## 21. Post-V1 direction

### 21.1 Blob backend extensions

V1 deliberately ships only the private local-filesystem blob adapter. A later
usage-driven extension may add a direct S3-compatible adapter behind the same
internal blob-store interface. It must use object operations rather than
presenting remote object storage as a mounted POSIX filesystem, and it must not
make a provider bucket, object URL, endpoint, or credential browser-visible.

FUSE mounts and standalone object-store servers remain outside the supported
TinyHost topology. They add a service/mount lifecycle and weaker filesystem
semantics without changing the gateway authorization, SQLite metadata, quota,
or reconciliation work TinyHost must perform itself.

### 21.2 Later candidates

Rank after usage evidence:

1. Direct S3-compatible blob-store adapter if local-disk limits become real.
2. Durable realtime history/replay or multi-node fan-out if usage demands it.
3. Operator-governed LLM capability.
4. Central SSO exchange.
5. Wildcard DNS-provider adapters.
6. Temporary invitations and groups.
7. Internal/Jira/data-warehouse capability adapters.
8. Operator backups and disaster recovery.
9. Deployer-selected rollback to a prior immutable application release.
10. Backend runtime only through a separate security concept.

Future provider access follows:

```text
SDK invocation
→ current app/viewer authorization
→ effective operator-approved grant
→ operation/resource/budget policy
→ server-held encrypted credential
→ narrow allowlisted adapter
→ bounded redacted result
```

Raw secret delivery and unrestricted authenticated HTTP proxying remain
non-goals.

## 22. Locked product decisions

No blocking product questions remain for the manual implementation handoff.

### D1 — V1 delivery scope

Implement milestones M0–M5. V1 includes the secure static platform, rudimentary
KV, lightweight local blob storage, and realtime WebSocket channels. It excludes
remote blob backends, external provider capabilities, backups, and backend
runtimes.

### D2 — V1 public data APIs

Expose deliberately small KV and blob APIs:

```ts
await tiny.kv.set("poll/options", options);
const current = await tiny.kv.get("poll/options");
const page = await tiny.kv.list({ prefix: "poll/", limit: 100 });
await tiny.kv.delete("poll/options", { expectedVersion: current.version });

const stored = await tiny.blobs.upload(file);
const downloaded = await tiny.blobs.get(stored.id);
const blobs = await tiny.blobs.list({ limit: 100 });
await tiny.blobs.delete(stored.id);
```

Values are JSON, mutations are versioned, and list is prefix/cursor/limit
bounded. There are no collections, schemas, joins, filters, or arbitrary
queries in V1. Blob IDs are opaque and server-issued; names are display metadata
only. Blobs are app-shared, immutable after upload, attachment-oriented on
download, local-disk backed, and cursor/size/count/quota bounded.

### D3 — Public apps

V1 is private-only. No CLI, manifest, UI, or API surface exposes public mode.
The architecture may leave a future policy seam, but missing/unknown policy
always denies.

### D4 — Deployer authentication and owner access

The operator reconciles one exact active-deployer email list. A deployer runs
`tiny login` directly or reaches the same bounded login inside
`tiny deploy .`, completes email OTP when no valid bearer exists, and receives
a server-bound scoped CLI token stored in the protected per-user Tiny
credential file. Every request rechecks deployer status, token scope, expiry,
and target ownership.

An active app owner is an implicit viewer. Deploying without viewer rules
creates a private owner-only app.

### D5 — License

The implementation repository uses Apache-2.0.

### D6 — Supported platforms

- Server: Hetzner Ubuntu 24.04 LTS and 26.04 LTS x86-64.
- Client: macOS and Linux, x86-64 and ARM64.
- Windows client and Linux ARM64 server follow after V1.

Supported server releases are an explicit allowlist, never a numeric version
range. Interim and end-of-life Ubuntu releases remain unsupported even when
their version number falls between supported LTS releases. The exact Hetzner
image identifiers are pinned during repository bootstrap.

### D7 — Agent skills

Create one generic `tiny-platform` agent skill first. It is the canonical shared
workflow and vocabulary. Then create self-contained `tiny-deployer` and
`tiny-operator` skills by copying the relevant shared content into each
role-facing skill. `tiny-deployer` owns app understanding, SDK use, build,
access-policy review, deployment, and independent protection verification.

Create separate maintainer-facing `distribute-tiny-cli` and
`distribute-tiny-sdk` skills. They reuse the signed release and compatibility
contracts but never inherit deployer or operator authority. Inspection and
local preparation are the default; publication requires explicit authorization
for finalized identities, version, destinations, and channel.

The specialized skills do not require the generic skill at runtime. Copy markers
or generation/check scripts make CI fail when shared sections drift. Content is
generic Markdown and executable scripts; Codex-compatible `SKILL.md` packaging
is the first distribution.

### D8 — Working names

“TinyHost”, `tinyhost`, `tiny`, and `@tinyhost/sdk` are working labels only. The
product may be renamed without changing architecture, authorization semantics,
protocol behavior, package boundaries, or the PRD’s actor/capability model.

Avoid scattering literal branding through domain packages. Keep names at
composition roots, distribution metadata, user-facing copy, and SDK package
configuration.

### D9 — Minimum-necessary guided flows

Human commands begin with the intended outcome (`tinyhost setup`,
`tiny deploy`, `tiny login`, `tiny logout`) and ask only for information that
is required, not already verified, and not safely derivable. They persist
verified reusable state in the correct boundary. A generated config or
`tiny.yaml` is a transparent receipt and automation surface; it is not a
prerequisite document the human must create.

The assistant may show derived defaults and an explicit edit path without
turning every default into a question. It must never infer an authorization
broadening, accept a secret in argv, follow an unverified server redirect, run
an application build, or introduce prompts into JSON/non-interactive use.
Missing required external state such as DNS or a verified sending domain
produces one exact action and a resumable continuation rather than a wall of
flags.

### D10 — Pre-rename beta distribution

Before the final public rename, one manually approved GitHub prerelease channel
MAY distribute the complete signed release directly from an exact
`vMAJOR.MINOR.PATCH` tag reachable from `main`. The workflow MUST test before
signing, use a protected beta-only signing environment, verify local and
uploaded draft bytes, and publish only a prerelease. CLI, host, and SDK tarball
consumers use the same exact versioned GitHub release.

This beta path MUST NOT publish npm, JSR, Homebrew, stable/latest channels,
reserve final identities, change DNS, or enable silent updates. The committed
beta authority MUST be replaced across every embedded trust anchor by a new
operator-controlled production authority before the first stable release.

## 23. Additional accepted defaults

- Email OTP only in V1; no password and no required magic-link flow.
- Resend only in V1 behind an adapter interface.
- Per-host HTTP-challenge ACME first.
- Server-rendered admin/deployer UI.
- No automatic build command inside `tiny deploy`.
- Human setup/deploy commands are inference-first, resumable wizards; generated
  configuration and manifests remain explicit, reviewable automation receipts.
- No Docker initially.
- No backup feature initially.
- No outbound telemetry by default.
- Manual signed `tinyhost update`; optional scheduled updates later.
- Stable app hostnames reused across releases.
- Global viewer identity with app-scoped local sessions and app-bound handoffs;
  never a parent-domain app cookie.
- Current policy evaluated on every protected HTTP request initially.
- WebSocket connections are closed on matching app/session/policy revocation.
- Realtime is best-effort, in-memory, and single-node.

## 24. Final V1 acceptance checklist

### Installation

- [ ] Supported clean Hetzner VPS initializes with documented prerequisites.
- [ ] Human setup asks only for non-discoverable required values and can resume
      after an external DNS or email prerequisite is fixed.
- [ ] Only TinyHost owns expected public sockets.
- [ ] DNS, TLS, Resend, SQLite, permissions, disk, and version diagnostics work.
- [ ] Root operator recovery works with email unavailable.
- [ ] Failed update returns to the prior healthy version.

### Deployer

- [ ] Operator can atomically reconcile the exact active-deployer email list.
- [ ] Deployer can authenticate without VPS access.
- [ ] `tiny deploy <dir>` returns a protected stable HTTPS URL.
- [ ] A first human deploy can generate its manifest and platform selection
      without requiring hand-authored config or repeated known values.
- [ ] Deployer can manage access, releases, tokens, and deletion only
      for owned apps.
- [ ] Agent/JSON output is deterministic and contains no secret.

### Viewer

- [ ] Allowed viewer can authenticate by OTP and access the app.
- [ ] Anonymous, unrelated, revoked, expired, and wrong-app sessions receive no
      app content.
- [ ] App suspension and policy removal deny the next request.

### Deployment and isolation

- [ ] Attack archive corpus is rejected safely.
- [ ] Failed upload/validation/activation keeps the previous release.
- [ ] Unknown host and reserved paths never expose files.
- [ ] Cross-app HTTP/service/repository matrix passes.
- [ ] Restart/interruption recovery is deterministic for every durable state.

### SDK, KV, lightweight blobs, and realtime

- [ ] App can read viewer/app/capability information through the SDK.
- [ ] KV supports app-scoped get/set/delete/prefix-list, version conflicts, and
      enforced limits.
- [ ] Blobs support app-scoped upload/get/list/delete with opaque IDs, bounded
      local storage, attachment downloads, and partial-write recovery.
- [ ] Cross-app, revoked, disk-stop, and metadata/storage disagreement blob
      tests expose zero unauthorized or uncertain bytes.
- [ ] Authenticated sockets support app channels and KV change events.
- [ ] Revocation closes affected live connections; reconnect recovers through
      a KV read, never replay.
- [ ] SDK cannot select app identity or receive secrets.
- [ ] Typed errors and compatibility failures are actionable.
- [ ] Examples compile and run as contracts.

### Security and release

- [ ] Route registry proves every surface is classified.
- [ ] Anonymous-denial matrix covers HTML, assets, fallback, KV/blob API,
      WebSocket, and reserved routes.
- [ ] Host/path/manifest/archive fuzz targets pass.
- [ ] Race detector and relevant failure injection pass.
- [ ] Logs/audit pass secret scanning.
- [ ] Artifacts are checksummed, signed, and carry provenance.
- [ ] Coding-agent skills reproduce build/deploy/denial verification.

## 25. Implementation authorization

The product decisions in §22 are locked. The implementation agent may:

- create the repository structure in §9.3;
- execute M0–M5 in order;
- make local implementation choices that do not change principles, external
  contracts, V1 scope, trust boundaries, or accepted ADRs;
- add ADRs when a spike replaces a recommended technology.

The agent must stop for user approval before:

- expanding V1 scope;
- adding a public listener or service;
- changing the authorization/session/secret boundary;
- adding a production dependency or provider;
- making KV data less isolated or durability claims stronger;
- exposing a public-app mode in V1;
- executing user-controlled backend code;
- changing the Apache-2.0 license. Working product/package names may be replaced
  consistently without changing architecture or contracts.
