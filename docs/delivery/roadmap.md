# Delivery Roadmap

The roadmap uses vertical slices. Each milestone includes a user-visible result,
a security claim, and an exit gate. Calendar estimates should be added only by
the implementing team after spikes.

## Active post-V1 extension — public reach and local insights

Outcome: a verified viewer can discover current apps they may open, an owner
can see bounded local usage, and an operator may permit deliberate
capability-free public static publishing without adding a listener, runtime,
provider, or anonymous platform capability.

Delivery order:

1. **L0 governance/contracts:** PRD D3/§21.3, ADR 0054, manifest v2,
   public-static/catalog/insights contracts, threat model, and deny charter.
2. **L1 local insights:** successful document outcome seam, bounded async local
   recorder, 30-day SQLite aggregates/digests, hard-delete/retention behavior,
   and owner/operator native UI summaries.
3. **L2 team catalog:** manifest-v2 tags, global-identity-only catalog auth,
   policy-filtered server query, and local search/tag filtering over authorized
   cards.
4. **L3 public static:** durable default-off operator gate, sealed static
   context, capability-free public activation, exact broadening confirmation,
   posture-aware probes, noindex default, and next-request transitions.
5. **L4 release evidence:** first-party public examples plus the complete
   two-owner/two-app private/public/catalog/insights clean-VPS matrix.

L4 implementation evidence includes the capability-free, deliberately indexed
[`public-static-product-story`](../../examples/public-static-product-story/)
example. It does not make the starter or SDK gallery public: those remain
private owner-only examples. Release evidence is complete only when the local
real-listener and opt-in clean-VPS matrices prove the gate/default-denial,
explicit acknowledgement, exact static/indexing behavior, reserved-route
denials, catalog filtering, approximate-visitor aggregation, next-request
transitions, two-owner isolation, restart persistence, and malformed/failure
paths listed in the VPS contract and denial charter.

Exit evidence is the acceptance list in
[`concept/features/public-reach-and-local-insights.html`](../../concept/features/public-reach-and-local-insights.html)
and the executable obligations in
[`public-reach-local-insights-denial-charter.md`](../../test/security/public-reach-local-insights-denial-charter.md).

## Next implementation slice — M5/M3 minimum-necessary guided flows

Outcome: a human operator can run `tinkercloud setup`, and a human deployer can run
`tinker deploy .`, without preparing configuration paperwork or repeatedly
entering information Tinkercloud can discover, verify, reuse, or safely default.
The exhaustive targets are the
[operator flow](../../concept/flows/operator.html) and
[deployer flow](../../concept/flows/deployer.html).

Already implemented foundations:

- one revision-protected exact active-deployer allowlist with atomic
  credential revocation and audit;
- persistent server-bound CLI bearer, verified default server, forced account
  switch, and exact-bearer logout;
- deploy-first missing-manifest wizard, immutable descriptions, dashboard
  search/status filtering, stable launch, hard deletion, and no deployer
  rollback; and
- one global browser identity with dashboard-role checks and app-bound
  handoffs.

Remaining vertical path:

- add the resumable human `tinkercloud setup` assistant over the strict
  non-interactive initialization contract;
- derive conventional platform/app/sender values from one base domain and
  pause with exact DNS/Resend actions, including a fail-fast exact-admin and
  wildcard-resolution check before the ACME-capable service starts;
- make project/output/capability discovery explicit and ask only when safe
  evidence is ambiguous;
- replace field-by-field optional deploy questions with one review/edit
  checkpoint; and
- remove the normal update `--app-slug` ceremony by deterministically selecting
  a locally verified active app (or proving the exact no-app state); and
- instrument prompt tests against
  [`specs/ux/minimum-necessary-input-contract.md`](../../specs/ux/minimum-necessary-input-contract.md).

Exit evidence:

- a fresh supported VPS reaches a verified dashboard without hand-authored
  config or repeated answers;
- interrupted external DNS/email work resumes at the exact blocked step;
- a useful built static project reaches a protected URL from
  `tinker deploy .` without hand-authored YAML;
- every human prompt proves its input is required, unknown, and unsafe to
  default; and
- JSON/non-interactive behavior remains deterministic and non-prompting.

## Cross-cutting delivery evidence — trusted GitHub issue loop

Outcome: written GitHub issues can be triaged, planned, and—after explicit
trusted maintainer approval—implemented by a recurring agent into a draft pull
request without granting merge or release authority.

Evidence implemented:

- issue forms place written reports in `inbox`; there is no in-app feedback
  source or special feedback label in this slice;
- deterministic prefetch exits without a model call for an empty queue,
  `open` alone, untrusted `implement`, or any `pending` issue;
- approval is derived from the current `implement` label event actor and a
  configured maintainer allowlist;
- one approved issue maps to one `feature/issue-*` branch and draft PR, never a
  direct implementation push to `main`; and
- fake-GitHub deny tests cover empty, intake, open-only, trusted approval,
  untrusted approval, event lookup failure, and pending precedence.

Contract:
[`specs/delivery/github-issue-loop-contract.md`](../../specs/delivery/github-issue-loop-contract.md).
Decision:
[`docs/decisions/0031-trusted-issue-loop-draft-prs.md`](../decisions/0031-trusted-issue-loop-draft-prs.md).

## M0 — Architecture proving ground

Outcome: risky decisions have evidence before product code expands.

Work:

- host canonicalization/routing spike;
- SQLite driver, migration, update-rollback, and interruption spike;
- safe archive extraction prototype and attack corpus;
- app-scoped cookie browser proof;
- authenticated WebSocket upgrade and revocation proof;
- per-host ACME lifecycle spike;
- define test harness that sends requests through a real listener.

Exit gate:

- ADR 0004 accepted or replaced;
- no unresolved blocker to gateway-only routing, safe extraction, or update
  recovery;
- spikes are discarded or isolated, not treated as production foundations.

## M1 — Protected static read path

Outcome: operator-created app content is visible only to an allowed viewer.

Components:

- configuration, database/migrations, apps, identity, OTP, sessions, policy;
- one admin-host global browser identity, current dashboard-role checks,
  one-time app-bound handoff, and
  local app-session child revocation;
- host router, authorization context, protected static runtime;
- minimal auth/operator pages and Resend adapter;
- route registry and negative security matrix.

Exit gate:

- anonymous HTML and asset requests expose zero release bytes;
- wrong-app and revoked sessions deny;
- policy/database failure denies;
- path and host fuzz suites pass.
- a viewer completes OTP once per browser profile, receives no second OTP for
  an allowed app, and still receives no app bytes for denied/replayed/wrong-app
  handoff paths.
- migration validation quarantines pre-cutoff parentless app sessions without
  changing `revoked_at`; all newly issued app sessions are handoff-linked and
  no brokerless browser-session issuance path exists.

## M2 — Deployer control plane

Outcome: operator authorizes a deployer; deployer authenticates and creates an
app policy without server access.

Components:

- platform sessions and scoped tokens;
- operator/deployer RBAC;
- app and policy commands;
- deterministic CLI output;
- audit events.

Exit gate:

- deployer cannot see or mutate another deployer’s app;
- token revocation applies on next request;
- security-sensitive mutations are transactionally audited.

Evidence implemented: successful bearer authentication atomically rechecks the
active user, revocation, expiry, app binding, and exact scope while recording
only a nullable UTC `last_used_at` timestamp. Denied, revoked, expired,
wrong-app, and wrong-scope attempts leave it untouched; a metadata write
failure denies and rolls back. Bounded token list and dashboard models expose
only this safe timestamp (never a raw token/hash, IP, or user agent), and
cross-owner token reads are denied.

Evidence implemented: Tinkercloud-owned platform login, app-origin login,
one-time-code verification, display-once token, dashboard, and operations
templates now consume one embedded dependency-free native web system. It
preserves generic non-enumerating auth responses, server-rendered forms,
same-origin/CSRF/ownership checks, current policy revision and broadening
confirmation, exact destructive targets, visible durable state labels, and the
VPS recovery disclaimer. The canonical tokens, mark, primitives, accessibility
rules, denial charter, complete loading/stale/unavailable/error/progress/
pagination/validation/busy state vocabulary, and realistic showcase live under
`web/` and `specs/ui/`. Styled safe platform and app browser error paths retain
their status/JSON boundaries. Exact SHA-256 CSP sources admit only the embedded
style and interaction bytes—never `unsafe-inline` or an asset route. Focused,
race, and security regressions reject remote or executable design assets and
prove user values remain escaped; this is not VPS deployment evidence or
reduced-motion emulation evidence.

The dashboard authenticates only from the admin-host global browser identity,
then rechecks the current operator/deployer role on every request. It has no
control-session credential or dashboard-specific browser OTP channel. A CLI
bearer remains a separate credential: cross-presenting a browser identity or
app viewer session as a bearer, or a bearer as a browser cookie, denies without
successful-use metadata. Global browser logout revokes its derived app sessions
but never CLI/agent bearers.

The server-rendered dashboard displays each owned app's current canonical
private allowlist and policy revision, with editable prepopulated email/domain
fields. It labels an empty list as owner-only and explains that an immutable
deployment's `tinker.yaml` policy replaces the current revision at activation;
the read model fails unavailable rather than silently showing absent or stale
policy state as empty.

## M3 — Immutable deployment loop

Outcome: `tinker deploy` returns a verified protected URL with failed-activation
preservation.

Components:

- manifest parser, upload stream, archive inspector/extractor;
- deployment state machine, release manifest, activation and recovery;
- certificate readiness and public gateway probes;
- failure recovery and cleanup.

Evidence implemented: the CLI streams a reproducible archive through a private
temporary file rather than RAM, obtains a fresh random idempotency key for each
deploy invocation, and cleans that file after the request. The control adapter
enforces the configured archive boundary independently of the 1 MiB JSON
control-body boundary, including unknown-length streams.

The manifest-named app is created automatically only when the authenticated
deployer's ownership-scoped app list proves it is missing; a foreign slug
conflict remains a denial. The server stores the canonical private manifest
allowlist with each immutable deployment, validates that candidate (not an old
ambient policy), and atomically swaps release pointer plus policy revision on
activation. Failure injection proves a failed activation preserves the prior
pointer and policy together.

Evidence implemented: an optional validated `tinker.yaml` description is stored
only in each immutable deployment manifest. Dashboard history shows the
matching release description while the app summary follows `current_deployment_id`,
so failed activation preserves the active app without a mutable app field.
Malformed final manifest
metadata makes the read model unavailable. The active current app alone has an
accessible new-tab stable-gateway launch icon; it never links raw release
storage and remains behind normal app authentication.

Before the CLI returns the protected URL, it separately reaches that exact
server-derived `https://{slug}.{domain}/` host anonymously through the
real HTTP/TLS transport. It accepts only the composed gateway's bounded 401
denial envelope; this client-side gate never sends the deployer token or
cookies to the app host. Transient first-host DNS, TLS, transport, 404, and
gateway-readiness outcomes receive a finite retry; redirects, public content,
wrong origins, malformed denials, and unsafe headers fail immediately. The
activation request budget exceeds the server's bounded 45-second certificate
gate while caller cancellation still wins. If independent evidence remains
incomplete after activation, the CLI returns non-success
`active_but_unverified` with only the safe deployment ID, URL, state, and
reason, rather than hiding a committed release behind `deploy_failed`. The
exact activation host is derived as `<slug>.<domain>` from the single
configured root domain; `admin.<domain>` is reserved for the dashboard and
browser identity broker.

The human first-app path now includes the dependency-free, owner-only Tinker
Ritual sample and a short Markdown setup guide. A regression parses its real
manifest, proves its allowlist stays owner-only with capabilities disabled, and
archives exactly the four intended deployment files. Local preview is explicitly
presentation evidence only; it does not stand in for Tinkercloud authentication,
TLS, activation, or anonymous-denial evidence.

Evidence implemented: restart recovery is database-led and evaluates stable
durable deployment records one at a time. Incomplete states fail rather than
resume, and verified/active/superseded records survive only when their
server-derived immutable release hash and complete file evidence match. A
malformed historical record is isolated so it cannot block a healthy app; a
corrupt current record atomically clears that app's pointer and makes it
unavailable. Recovery never discovers authority by scanning release or staging
directories, and persistence tests prove repeat recovery is idempotent.

Exit gate:

- failed upload/validation/activation retains prior release;
- attack archive corpus passes;
- restart at each transition recovers deterministically;
- CLI refuses success when an anonymous probe retrieves content.

## M4 — SDK, per-app data, lightweight blobs, and realtime

Outcome: a static app can identify the viewer, persist scoped JSON values and
bounded documents/files, and react to app events immediately.

Components:

- current-user API;
- bounded V1 JSON KV model with versions, prefix listing, and quotas;
- bounded JSON document collections with optimistic versions and snapshots;
- bounded app-scoped blob upload, download, list, metadata, and delete backed
  by the private local data directory;
- single-node in-memory realtime hub with custom channels plus KV and collection
  change hints;
- first-class TypeScript/browser SDK and capability discovery;
- CSP/CORS/CSRF browser tests.

Evidence implemented: authenticated live sockets have validated V1 idle, ping,
pong, write-deadline, and outbound-queue bounds. Real-socket tests prove a
silent peer is disconnected, a pong-responsive peer remains usable, a slow
consumer fails closed, and revocation closes its matching socket directly.
The built TypeScript SDK runs against a real composed gateway listener with an
authorized host-only session, proving identity/app/capability discovery and
app-scoped KV get/set/list/delete. The contract also proves that no SDK input
selects another app, raw clients without a version header remain compatible,
and an unsupported supplied SDK major maps to the actionable typed
`TinkerVersionIncompatibleError`. Checked-in SDK examples compile and the
uncompressed ESM output has a 12 KiB regression gate.
Distribution preparation now keeps the compiled npm artifact, TypeScript-native
JSR artifact, and exported SDK version synchronized. The npm tarball has an
exact file allowlist and must install and import in an offline clean consumer;
registry publication remains blocked pending namespace ownership.

Evidence implemented: the public SDK gallery contains six deployable private
apps. The V1 examples cover current viewer/app information, capability
discovery, every V1 KV operation, bounded cursors, optimistic concurrency,
cancellation, typed error presentation, KV change subscriptions, and custom
live channel connect/on/publish/status/close; Reactive Collections and LLM Chat
exercise the implemented collection slice and post-V1 L1/L2 extension. Their
local build copies the checked SDK ESM into each release; a temporary-release
contract rejects unresolved package imports, remote executable assets,
client-selected app identity, and credential-like browser configuration.
Realtime examples reconcile current durable state after events, reconnects,
and visible-tab recovery rather than claiming replay or delivery history.

### Implemented M4 slice — per-app SQLite and reactive collections

Outcome: KV and bounded JSON documents use normal embedded SQLite without a
database daemon, shared app-data writer, remote database authority, or browser
SQL surface.

Decision and contract:

- ADR [`0048`](../decisions/0048-per-app-sqlite-collections.md) keeps the
  existing `modernc.org/sqlite` engine and separates `tinkercloud.db` control state
  from `apps/{immutable-app-id}/data.db` app state;
- [`specs/capabilities/collections-contract.md`](../../specs/capabilities/collections-contract.md)
  defines server-issued document IDs, optimistic versions, bounded lists and
  snapshots, quotas, and post-commit freshness hints; and
- the pre-release migration removes the legacy control-database `app_kv` table.
  Production composition has no shared-control-database KV fallback.

Evidence implemented:

- the bounded in-process app database manager validates server-derived immutable
  IDs before path construction, lazily migrates app files, limits open handles,
  closes idle handles, and removes only the selected app database during hard
  deletion;
- KV and collection repositories use the selected app-local database, and
  isolation, traversal, stale-write, quota, concurrent-limit,
  failed-transaction, and deletion tests fail closed;
- collection create/get/update/delete/list/snapshot routes, SDK
  `tinker.db.collection(...)`, optimistic conflicts, and app-scoped
  `collection.changed` hints are implemented; reconnect and tab visibility
  recover from authoritative snapshots rather than event replay;
- the Reactive Collections example and SDK tests cover the typed surface; and
- `tinker dev` uses project-local normal SQLite and supports the local subset of
  viewer/app information, KV, collections, and live freshness hints while
  refusing non-loopback listeners. It explicitly excludes production auth,
  deployment, blobs, LLM/provider calls, and deployment protection evidence.

Live release evidence:

- the signed `0.1.0` build passed the guarded clean-host VPS acceptance run on
  2026-07-31 at `testing.tinkercloud.fun`;
- real gateway requests proved two-app KV/collection isolation, restart
  persistence, anonymous denial with no app bytes, and fixture-scoped cleanup;
  and
- the same pass proved one dashboard OTP identity across allowed app hosts,
  distinct host-only app sessions, replay/wrong-app denial, app-local logout,
  global child-session revocation, account switching, blob isolation, and
  omitted ungranted LLM capability state. The redacted owner-only result is
  `.tinker/vps/unattended-reports/vps-e2e-20260731T145714Z-74715.status`.

### Implemented M4 slice — lightweight app-scoped blobs

Outcome: an authenticated static app can store and retrieve small attachments
through `@tinkercloud/sdk` without operating a bucket, mount, storage server, or
backend process.

Contract and decision:

- [`specs/capabilities/blob-contract.md`](../../specs/capabilities/blob-contract.md)
  and ADR [`0030`](../decisions/0030-v1-lightweight-local-blob-storage.md)
  define the public and persistence boundary;
- `tinker.yaml`, capability discovery, the sealed authorization context, gateway
  route registry, HTTP contract, SDK, deployable Attachment Shelf example, and
  Tinkercloud skills move together;
- the implementation uses a narrow internal streaming blob-store interface and
  a private standard-library local adapter; and
- the deployment shape remains one server process, one embedded SQLite engine,
  one control database plus isolated app-local database files, one private data
  directory, and one systemd service—no Go CDK, FUSE, rclone, s3fs, Mountpoint,
  MinIO, remote driver, extra package, or listener.

Evidence implemented:

- `features.blobs` is opt-in and disabled by default; the gateway constructs
  the app and viewer authority before every blob operation;
- SQLite owns `staging`, `ready`, and `deleting` metadata and exact app-scoped
  quota accounting; private local bytes are addressed only by server-derived
  `(app, blob ID)`;
- uploads stream through bounded standard-library I/O into a same-filesystem
  private temporary object and become readable only after close/sync, atomic
  finalization, and the `ready` catalog commit;
- the SDK exposes app-shared `upload`, `get`, bounded `list`, and `delete`
  without app ID, path, key, bucket, URL, or credential input;
- downloads remain behind the authenticated gateway and are attachment-only,
  `nosniff`, and `private, no-store`; and
- startup/bounded reconciliation removes interrupted staging/deleting rows,
  unreachable orphans, and missing/corrupt ready metadata without serving
  uncertain bytes.

Default product bounds are 25 MB per blob, 1,000 blobs and 250 MB total per app,
and 100 records per list page. Upload concurrency/rate, metadata, filename,
content type, and duration are also finite.

Local and real-listener evidence implemented:

- anonymous, wrong-app, revoked, suspended, capability-disabled, malformed,
  oversized, quota-exceeded, disk-stop, and cross-origin mutation paths are
  covered with no ready blob or disclosed bytes;
- injected stream, close/sync/rename, SQLite, disk-source, and
  metadata/storage-disagreement failures leave no readable partial/foreign blob
  and reconcile catalog/accounting state;
- every repository lookup and storage key begins with server-derived app
  identity, while filenames remain display metadata only;
- real-gateway and built-SDK coverage proves two-app upload/get/list/delete
  isolation, typed errors, cancellation, bounded pagination, and no inline
  uploaded-document execution; and
- manifest verification, SDK examples, and skills include the blob capability
  while preserving the single-process/local-storage boundary.

Remaining release evidence:

- The updated blob-enabled production binary must still be installed through
  the signed update path on the disposable VPS and pass the unattended SSH/OTP
  suite. That gate is pending until release-key approval permits installation
  of this exact build; prior VPS success is not evidence for it.
- The final V1 run must record the two-app KV/blob matrix at HTTP, repository,
  storage, SDK, and updated-VPS layers; deterministic quota/concurrency;
  partial/disagreement recovery; revocation; reconnect KV recovery; SDK secret/
  app-selector absence; and example/skill drift checks.

## M5 — Operable Hetzner-first release

Outcome: an operator can initialize a clean Hetzner VPS, diagnose it, and apply
a signed self-update with automatic rollback.

Components:

- one-command install, init/status/doctor commands, and systemd packaging;
- guided root-local operator setup over the same resumable init state machine,
  while retaining deterministic non-interactive automation;
- disk and rate limits, cleanup jobs, retention;
- signed update, compatibility gate, local rollback state, and health rollback;
- structured logs, admin views, security diagnostics;
- signed, reproducible release pipeline.

Evidence implemented: `tinkercloud status` remains an offline diagnostic and does
not load provider credentials. `sudo tinkercloud doctor` independently validates
the root-only systemd credential file before making its bounded read-only
Resend request, so it does not depend on systemd's inherited environment.
Absent, symlinked, non-root-owned, permissive, malformed, duplicate, and
unexpected credential assignments fail the Resend check closed without a
provider request or secret/path/reference disclosure. Focused, race, full Go,
skill-drift, and offline security-gate evidence pass.

The root-only deployer authorization command now validates its local root
caller, narrowly hands off any legacy root-owned SQLite DB/WAL/SHM artifacts,
then mutates SQLite in a permanently dropped `tinkercloud` child. New and repaired
artifacts stay owned by the gateway identity without permissive mode changes.
After its durable close, the root parent refreshes an already-running service
and verifies it remains active; it never starts an inactive service.

### Planned M5 slice — lightweight host resource overview

Outcome: the authenticated operator can see a small recent CPU, RAM, and
Tinkercloud data-volume storage chart in the existing admin board and can recognize
resource pressure without SSHing into the VPS.

This remains part of the single `tinkercloud` process. It uses a low-cadence,
fixed-size in-memory sample window and server-rendered output; it adds no
Prometheus/Grafana stack, database time-series table, public metrics endpoint,
client polling loop, or second listener. History resets on server restart.

Trust boundary and data ownership:

- only a current server-derived operator session receives the host resource
  read model; deployers, viewers, and anonymous callers receive none of it;
- samples contain aggregate CPU/RAM utilization and configured data-volume
  usage only, never processes, command lines, paths, hostnames, addresses,
  emails, app IDs, or credentials;
- storage usage comes from the same configured-volume semantics as the
  independent disk watermark gate; and
- collection or rendering failure shows unavailable and never weakens write
  denial at the critical disk watermark.

Contract and deny charter, before the happy path:

- extend [`specs/operations/m5-contract.md`](../../specs/operations/m5-contract.md)
  and the safe dashboard read model with bounded sample count, value ranges,
  staleness, reset, role, and unavailable-state behavior;
- reject or mark unavailable malformed, out-of-range, stale, partially
  collected, and source-error samples rather than graphing invented zeroes;
- prove anonymous and deployer-role dashboard responses contain no host samples
  or chart markup; and
- prove repeated collection and rendering stay within fixed memory/work bounds
  and do not mutate operational or authorization state.

Exit evidence:

- an operator sees labeled recent CPU, RAM, and storage utilization plus exact
  current used/total storage bytes and warning/write-stop thresholds;
- the chart remains readable without JavaScript and exposes an accessible text
  summary;
- restart begins a fresh bounded history without an error or misleading
  continuity; and
- source failure renders a clear unavailable state while the independent disk
  write gate continues to fail closed.

### Planned M5 slice — guided operator setup

Outcome: after SSHing into a clean supported VPS, an operator can complete
Tinkercloud setup through one understandable guided flow without composing the
full `tinkercloud init --non-interactive` command by hand.

The working CLI shape is a root-local `tinkercloud setup` assistant. Before
implementation, its final command contract and deny charter must be added to
the M5 operations contract. It must call the existing resumable init
application service rather than create a second installation pipeline.

The assistant:

- checks supported OS/architecture, root authority, NTP, disk, and ports before
  asking for configuration;
- asks for one controlled base domain and the operator email, derives
  conventional platform/app/sender values and the internal ACME contact from
  the normalized operator email, and puts non-standard choices
  behind one edit step;
- pauses with exact external DNS and Resend actions, persists validated
  non-secret progress, and resumes without re-asking prior valid answers;
- accepts secrets through explicit root-readable file paths, or through a
  no-echo prompt only after supported-shell handling passes audit; never secret
  values in argv, ordinary config, echoed output, generated shell commands,
  logs, or summaries;
- previews the resulting non-secret configuration and affected paths before
  confirmation;
- renders durable step progress for preflight, paths, database, operator,
  service, and public verification, and resumes safely after interruption;
- turns DNS, Resend, TLS, permission, port, and public-health failures into one
  actionable next step without weakening fail-closed behavior;
- retains `tinkercloud init --non-interactive` for agents, CI, and reproducible
  automation; and
- opens no temporary web setup listener, issues no bootstrap URL/token, and
  adds no remote recovery or authorization bypass.

Exit evidence:

- a first-time operator completes a clean supported Hetzner host setup without
  manually assembling the init argv;
- cancellation and restart resume from the first incomplete durable step
  without repeating completed mutations;
- malformed input, unreadable or over-permissive secret files, unsupported
  hosts, occupied ports, and dependency failures stop before unsafe mutation;
- captured terminal output, process argv, config, logs, and generated summaries
  contain no Resend key, HMAC key, session secret, or other credential; and
- the guided and non-interactive paths produce the same validated config,
  hardened systemd unit, init-state transitions, public gateway proof, and
  anonymous-denial evidence.

Release evidence is produced by `scripts/release-build.sh`, verified by
`scripts/release-verify.sh`, and normatively described in
[`specs/operations/release-artifact-contract.md`](../../specs/operations/release-artifact-contract.md).

Evidence implemented: the release builder produces the supported Linux/amd64
server and all four V1 macOS/Linux client targets. Every distributable payload
is individually signed and listed in checksums and per-target provenance; a
signed release manifest also binds the full evidence set, and the verifier
rejects an incomplete platform matrix. `packaging/install-client.sh`
is a non-root verifier/installer with denial tests for insecure origin,
unsupported platform, checksum mismatch, and bad signature. CI installs the
pinned SDK dependency lockfile and runs Go vet, normal/race tests, SDK tests,
skill drift, and release/installer tamper tests without VPS credentials.
The continuous gate also runs a checked-in-source secret-pattern scanner with
deny/allow self-tests, every parser fuzz target for a fixed time budget, a
pinned Go vulnerability scan, and a lockfile-bound production npm audit. It
does not require a VPS, runtime credential, or public listener.
The signed complete-release manifest now binds a strict release version and
the server/CLI/SDK/control-API/app-API/schema compatibility matrix without
changing the existing per-artifact signature bytes. A publish-free preparation
task derives a lifecycle-script-free, dependency-free npm CLI candidate and a
Homebrew formula from a fully verified release. It performs no registry, tap,
hosted-release, namespace, DNS, or channel mutation. ADR 0052 locks the
Tinkercloud product, command, package, formula, tap, and GitHub identities;
registry availability and any future official domain remain separate
publication decisions.
The standalone `distribute-tinker-cli` and `distribute-tinkercloud-sdk` maintainer
skills turn those gates into explicit inspect, prepare, and publish workflows.
Preparation remains local and non-mutating; publishing is blocked until the
operator has verified ownership of the public destinations and explicitly
authorized the named release. Repository drift checks validate both skill
contracts and their invocation metadata.
An explicitly dispatched `beta-release` GitHub Actions workflow provides the
pre-stable distribution channel. It accepts only an existing
`vMAJOR.MINOR.PATCH` tag reachable from `main`, runs the normal gates before
entering the protected signing environment, builds and verifies one complete
release, verifies the uploaded draft bytes, and publishes only a GitHub
prerelease. Direct CLI, host, and SDK-tarball installation use that exact
versioned release. npm, JSR, Homebrew, stable/latest promotion, and silent
updates remain disabled. The committed authority is beta-only and must rotate
across every embedded trust anchor before stable distribution.
Stable distribution scaffolding is executable but intentionally inactive:
`stable-release.yml` can publish and remotely verify an immutable GitHub
release only when the committed key policy identifies a rotated production
authority, while `npm-cli-publish.yml` derives `@tinkercloud/cli` from that
exact release and uses npm trusted-publisher OIDC. A denial charter and static
self-tests reject automatic triggers, beta-key promotion, private repositories,
existing versions, long-lived npm tokens, unverified downloads, and mixed
GitHub/npm mutation. The npm candidate has an exact content allowlist and
canonical provenance metadata. Activation still requires the public GitHub
repository, protected Environments, production key rotation, npm scope
ownership, and one interactive 2FA bootstrap because npm cannot configure a
trusted publisher for a package that does not yet exist.
The packaged and generated gateway units are also locally validated without a
systemd PID 1: they retain the `tinkercloud` user, strict sandbox, exactly the
configured writable paths, and only `CAP_NET_BIND_SERVICE` in their ambient
and bounding capability sets for ports 80/443. Their cgroup bind policy denies
all other TCP and UDP binds, allows only TCP 80/443, and restricts address
families to Unix, IPv4, and IPv6 sockets. Production config rejects other
listener ports. The VPS inventory rejects any additional non-loopback listener
owned by Tinkercloud while leaving firewall, SSH, and operator-owned listeners
outside Tinkercloud's mutation scope.
Host preflight uses an exact Ubuntu 24.04 LTS/amd64 and Ubuntu 26.04 LTS/amd64
allowlist. Its denial matrix rejects non-Linux/non-amd64 hosts, other
distributions, duplicated or malformed OS metadata, EOL interim releases
between those LTS versions, and future unverified versions before mutation.
A pinned dedicated Ubuntu 26.04 LTS/amd64 VPS exercised the compiled production
preflight with an injected unavailable NTP adapter: it advanced past host
support to the stable `clock_unsynchronized` denial, created no service user,
config, init state, database, systemd unit, or public listener, and left only
SSH listening after temporary evidence cleanup.
Init remains incomplete unless the derived admin host proves the public
gateway's exact HTTPS version endpoint: no redirect, a verified final host,
`200` bounded `{"api_version":1}` JSON, and `no-store`/`nosniff` headers.
Focused denial tests reject 401/404/5xx, arbitrary 2xx/HTML, malformed or
oversized JSON, wrong host/version, redirects, TLS, transport, and timeout
failures; a real locally trusted TLS harness verifies the positive path.

Exit gate:

- clean Hetzner VPS install succeeds without Docker;
- failed update returns to the prior healthy binary and schema state;
- update evidence rejects a missing app route, public content, malformed denial,
  timeout, or transport failure and accepts only the composed gateway's
  protected-route denial;
- only expected public sockets exist;
- dependency scan, fuzz targets, and failure-injection suite pass.

### Open deployment topology — VPN-only ingress

The canonical V1 topology remains a public dedicated VPS. Strict VPN-only
ingress is not yet supported because per-host ACME uses public HTTP-01 and init
requires public HTTPS gateway evidence. ADR
[0028](../decisions/0028-operator-supplied-tls-for-vpn-only.md) now fixes the
first VPN-only certificate model: the operator provides and renews one
certificate/key pair covering the platform and wildcard app hostnames. Secure
support still needs private or split DNS rules, protected certificate
installation and atomic replacement, trusted-network health and
anonymous-denial probes, expiry diagnostics, and a real VPN acceptance suite.
VPN membership must not replace Tinkercloud app identity, session, or policy
checks. Users retain Tinkercloud email OTP login and per-app authorization in the
first VPN-only mode; central SSO remains a separate later candidate. This work
is not authorized to weaken the M5 public proof and is not scheduled ahead of
the committed V1 blob slice.

## Implemented M4 extension — deployer data access

Outcome: an authenticated deployer can use the saved `tinker` CLI login to
inspect and deliberately repair the bounded KV/document state of an app they
own, without receiving SQLite access or a new database service.

Implemented L1–L3 boundary and required evidence:

- ADR [`0049`](../decisions/0049-typed-deployer-app-data-access.md), the
  [`deployer-data contract`](../../specs/api/deployer-data-contract.md), and
  its [deny charter](../../test/security/deployer-data-denial-charter.md) define
  a typed server-derived deployer-data authorization context, scoped API/CLI
  operations, lifecycle semantics, bounded payload/page rules, and no-raw-SQL
  boundary;
- control bearer authentication rechecks active deployer status, expiry,
  revocation, exact `data:read`/`data:write` scope, optional app binding, and
  ownership before resolving an internal immutable app ID or opening data;
- the control API and `tinker data` CLI reuse existing KV/collection response,
  pagination, validation, quota, optimistic-version, cancellation, and
  idempotency semantics. They expose bounded list/get plus individual create,
  update, and delete operations—not SQLite paths/files, SQL, arbitrary filters,
  schemas, bulk actions, export/import, or backup;
- before a write, the control database records redacted metadata-only audit
  intent and request digest. A successful app commit marks that intent
  succeeded for exact replay; an interrupted outcome reconciles only when
  current versioned state proves the result. The surface does not claim a
  cross-database transaction or rollback. New or safely reconciled mutations
  emit the existing app-scoped best-effort freshness hint, while failed and
  completed replays do not. Suspended owned apps are readable for diagnosis
  but not writable; deleting/deleted/unavailable apps deny all access; and
- focused repository/service/control/CLI tests must prove anonymous,
  credential-crossing, scope, app-binding, cross-owner, malformed-selector,
  stale-version, audit-failure, cancellation, unavailable-DB, and two-deployer/
  two-app denial behavior before this is reported as release evidence.

This extension deliberately does not grant an operator access to an unowned app
or add a dashboard data browser, database export/import, or backup feature. An
operator who exactly owns an app uses the same owner-scoped CLI path. It retains
one private app-local SQLite authority per app and the same loss-of-VPS
utility-data disclaimer as M4.

## Implemented post-V1 extension — operator-governed LLM chat

Outcome: an operator can keep an Anthropic or Gemini key behind Tinkercloud,
approve one bounded profile for an app, and let an authorized viewer use
provider-neutral non-streaming chat without exposing a secret, provider URL,
connection, model selector, or app identifier.

Implemented L1/L2 evidence:

- ADR [`0047`](../decisions/0047-operator-governed-llm-chat.md) and the
  [`llm.chat` contract](../../specs/capabilities/llm-chat-contract.md) define the
  post-V1 secret, grant, quota, destination, audit, and denial boundaries;
- the control database stores authenticated encrypted connection envelopes,
  profiles, grants, conservative token reservations, usage, and redacted audit
  evidence while the encryption root remains in the root-owned service
  credential boundary;
- root-local enablement, write-only operator connection/rotation controls,
  profile/grant controls, activation gating, safe discovery, fixed Anthropic
  and Gemini adapters, the protected app route, SDK
  `tinker.llm.chat.complete`, and a deployable example are implemented;
- local unit, persistence, provider-conformance, UI, SDK, and composed
  real-listener tests cover two-app isolation, revocation, strict input,
  rate/quota/concurrency admission, conservative ambiguous outcomes, redirects,
  cancellation, and secret/prompt/completion leakage; and
- `tinker dev` deliberately reports no LLM capability and performs no provider
  call, because emulating grants, spend, and secret handling would misrepresent
  production authorization.

This implementation does not expand the locked V1 release. L3 streaming,
tools, embeddings, files, arbitrary provider options, generic authenticated
HTTP, and provider-owned conversation history remain deferred. The exact signed
build passed the root-run disposable-VPS suite on 2026-07-29, including
root-local LLM enablement, restart/doctor checks, capability omission, anonymous
401, and authenticated ungranted 403 behavior without contacting a provider.
Dedicated low-value real-provider smoke tests remain pending; local
fake-provider, real-listener, and no-provider VPS evidence must not be reported
as real-provider evidence.

## Later post-V1 candidates

Rank only after usage evidence:

1. Direct S3-compatible blob-store adapter if local-disk limits become real.
2. Durable realtime history/replay and multi-node fan-out.
3. Central SSO exchange.
4. Wildcard DNS provider adapters.
5. Temporary invitations/groups.
6. Backend runtime (separate security concept).
7. Operator backup and disaster recovery.
8. Internal/Jira/data-warehouse capability adapters behind separately approved
   narrow grants.
