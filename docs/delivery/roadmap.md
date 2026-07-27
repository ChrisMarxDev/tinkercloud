# Delivery Roadmap

The roadmap uses vertical slices. Each milestone includes a user-visible result,
a security claim, and an exit gate. Calendar estimates should be added only by
the implementing team after spikes.

## Next implementation slice — M2 admin-board deployer allowlist

Outcome: an authenticated operator can use the admin board to see and manage
the complete set of normalized email identities allowed to deploy apps,
including authorizing an email that has never signed in. This is the next
implementation priority. It is followed by the M5 host resource overview and
then the planned M5 guided operator setup.

Trust boundary and ownership:

- the active operator is derived from the control browser session and rechecked
  by the server for every mutation;
- the TinyHost `users` records remain the only deployer-authorization source of
  truth; the browser form does not introduce a second allowlist;
- email, status, route, CSRF, and confirmation values are untrusted input; and
- suspend or revoke still invalidates affected control credentials in the same
  audited transaction and applies to the next relevant request.

Contract and deny charter, before the happy path:

- extend [`specs/api/control-auth-contract.md`](../../specs/api/control-auth-contract.md)
  with the add-new-deployer form, normalized-email behavior, bounded complete
  listing/pagination behavior, and generic failure response;
- prove anonymous, deployer-role, inactive-operator, missing/wrong-CSRF,
  cross-origin, malformed-email, operator-email collision, replay-conflict,
  database-failure, and audit-failure attempts do not create or change a
  deployer;
- prove an unavailable or truncated deployer read model is never presented as
  the complete allowlist; and
- retain the root-local `tinyhost deployers` command as recovery/bootstrap, not
  as a prerequisite for routine allowlist management.

Vertical path:

- add an operator-only normalized-email form to the Deployers section;
- authorize a new deployer through the existing typed, audited browser action;
- show the new active deployer immediately in a bounded list from which the
  operator can suspend, re-authorize, or revoke them; and
- make every deployer reachable through pagination or an equivalent explicit
  bounded navigation instead of silently stopping at the current first 100.

Exit evidence:

- a fresh email can be authorized entirely from the admin board and can then
  complete deployer login;
- every authorized, suspended, and revoked deployer is discoverable from the
  board without VPS access;
- a non-operator cannot read the deployer list or invoke any deployer mutation;
- malformed and failed mutations leave authorization and audit state
  unchanged; and
- suspension or revocation denies the deployer's next control request and does
  not resurrect prior credentials when re-authorized.

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
- host router, authorization context, protected static runtime;
- minimal auth/operator pages and Resend adapter;
- route registry and negative security matrix.

Exit gate:

- anonymous HTML and asset requests expose zero release bytes;
- wrong-app and revoked sessions deny;
- policy/database failure denies;
- path and host fuzz suites pass.

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

Evidence implemented: TinyHost-owned platform login, app-origin login,
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

Control browser sessions are persisted separately from CLI bearer tokens, and
each control OTP is bound before delivery to its browser or CLI completion
channel. Cross-presenting a dashboard cookie as an API bearer, a CLI bearer as
a dashboard cookie, or an app viewer session on either surface denies without
successful-use metadata; the upgrade path fails legacy unbound challenges
closed and revokes every pre-separation bearer token, requiring one fresh CLI
login.

The server-rendered dashboard displays each owned app's current canonical
private allowlist and policy revision, with editable prepopulated email/domain
fields. It labels an empty list as owner-only and explains that an immutable
deployment's `tiny.yaml` policy replaces the current revision at activation;
the read model fails unavailable rather than silently showing absent or stale
policy state as empty.

## M3 — Immutable deployment loop

Outcome: `tiny deploy` returns a verified protected URL, with rollback.

Components:

- manifest parser, upload stream, archive inspector/extractor;
- deployment state machine, release manifest, activation and recovery;
- certificate readiness and public gateway probes;
- rollback and cleanup.

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
activation and rollback. Failure injection proves either operation preserves
the prior pointer and policy together.

Evidence implemented: an optional validated `tiny.yaml` description is stored
only in each immutable deployment manifest. Dashboard history shows the
matching release description while the app summary follows `current_deployment_id`,
so rollback restores it without a mutable app field. Malformed final manifest
metadata makes the read model unavailable. The active current app alone has an
accessible new-tab stable-gateway launch icon; it never links raw release
storage and remains behind normal app authentication.

Before the CLI returns the protected URL, it separately reaches that exact
server-derived `https://{slug}.{app_suffix}/` host anonymously through the
real HTTP/TLS transport. It accepts only the composed gateway's bounded 401
denial envelope; this client-side gate never sends the deployer token or
cookies to the app host and rejects 404, redirect, public content, malformed
evidence, and transport/TLS failure. The suffix is activation evidence rather
than an inference from the control-plane host, so `tiny.example.com` and
`*.apps.example.com` remain supported.

The human first-app path now includes the dependency-free, owner-only Tiny
Ritual sample and a short Markdown setup guide. A regression parses its real
manifest, proves its allowlist stays owner-only with capabilities disabled, and
archives exactly the four intended deployment files. Local preview is explicitly
presentation evidence only; it does not stand in for TinyHost authentication,
TLS, activation, or anonymous-denial evidence.

Exit gate:

- failed upload/validation/activation retains prior release;
- attack archive corpus passes;
- restart at each transition recovers deterministically;
- CLI refuses success when an anonymous probe retrieves content.

## M4 — SDK, KV, and realtime

Outcome: a static app can identify the viewer, persist scoped JSON values, and
react to app events immediately.

Components:

- current-user API;
- bounded V1 JSON KV model with versions, prefix listing, and quotas;
- single-node in-memory realtime hub with custom channels and KV change events;
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
`TinyVersionIncompatibleError`. Checked-in SDK examples compile and the
uncompressed ESM output has a 12 KiB regression gate.
Distribution preparation now keeps the compiled npm artifact, TypeScript-native
JSR artifact, and exported SDK version synchronized. The npm tarball has an
exact file allowlist and must install and import in an offline clean consumer;
registry publication remains blocked pending namespace ownership.

Evidence implemented: the public SDK gallery contains three deployable private
apps covering current viewer/app information, capability discovery, every V1
KV operation, bounded cursors, optimistic concurrency, cancellation, typed
error presentation, KV change subscriptions, and custom live channel
connect/on/publish/status/close. Their local build copies the checked SDK ESM
into each release; a temporary-release contract rejects unresolved package
imports, remote executable assets, client-selected app identity, and
credential-like browser configuration. Every live flow rereads current KV
after events, reconnects, and visible-tab recovery rather than claiming replay
or delivery history.

Exit gate:

- two-app isolation matrix passes at HTTP and repository layers;
- quota and concurrency conflicts are deterministic;
- unauthorized upgrades fail and revocation closes affected connections;
- reconnecting clients can recover by rereading current KV state;
- SDK contains no long-lived secret or selectable app ID.
- SDK examples and `tiny-app-development` skill pass contract tests.

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

Evidence implemented: `tinyhost status` remains an offline diagnostic and does
not load provider credentials. `sudo tinyhost doctor` independently validates
the root-only systemd credential file before making its bounded read-only
Resend request, so it does not depend on systemd's inherited environment.
Absent, symlinked, non-root-owned, permissive, malformed, duplicate, and
unexpected credential assignments fail the Resend check closed without a
provider request or secret/path/reference disclosure. Focused, race, full Go,
skill-drift, and offline security-gate evidence pass.

The root-only deployer authorization command now validates its local root
caller, narrowly hands off any legacy root-owned SQLite DB/WAL/SHM artifacts,
then drops permanently to `tinyhost` before opening SQLite. New and repaired
artifacts stay owned by the gateway identity without permissive mode changes;
ownership/drop/open failures deny without a success message.

### Planned M5 slice — lightweight host resource overview

Outcome: the authenticated operator can see a small recent CPU, RAM, and
TinyHost data-volume storage chart in the existing admin board and can recognize
resource pressure without SSHing into the VPS.

This remains part of the single `tinyhost` process. It uses a low-cadence,
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
TinyHost setup through one understandable guided flow without composing the
full `tinyhost init --non-interactive` command by hand.

The working CLI shape is a root-local `tinyhost setup` assistant. Before
implementation, its final command contract and deny charter must be added to
the M5 operations contract. It must call the existing resumable init
application service rather than create a second installation pipeline.

The assistant:

- checks supported OS/architecture, root authority, NTP, disk, and ports before
  asking for configuration;
- asks only for values it cannot discover or safely default: platform/app
  domains, operator email, verified sending address, and credential-file paths;
- defaults the public ACME contact to the operator email unless explicitly
  overridden;
- accepts secrets only through explicit root-readable file paths, never secret
  values in argv, echoed prompts, generated shell commands, logs, or summaries;
- previews the resulting non-secret configuration and affected paths before
  confirmation;
- renders durable step progress for preflight, paths, database, operator,
  service, and public verification, and resumes safely after interruption;
- turns DNS, Resend, TLS, permission, port, and public-health failures into one
  actionable next step without weakening fail-closed behavior;
- retains `tinyhost init --non-interactive` for agents, CI, and reproducible
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
The packaged and generated gateway units are also locally validated without a
systemd PID 1: they retain the `tinyhost` user, strict sandbox, exactly the
configured writable paths, and only `CAP_NET_BIND_SERVICE` in their ambient
and bounding capability sets for ports 80/443. Their cgroup bind policy denies
all other TCP and UDP binds, allows only TCP 80/443, and restricts address
families to Unix, IPv4, and IPv6 sockets. Production config rejects other
listener ports. The VPS inventory rejects any additional non-loopback listener
owned by TinyHost while leaving firewall, SSH, and operator-owned listeners
outside TinyHost's mutation scope.
Host preflight uses an exact Ubuntu 24.04 LTS/amd64 and Ubuntu 26.04 LTS/amd64
allowlist. Its denial matrix rejects non-Linux/non-amd64 hosts, other
distributions, duplicated or malformed OS metadata, EOL interim releases
between those LTS versions, and future unverified versions before mutation.
A pinned dedicated Ubuntu 26.04 LTS/amd64 VPS exercised the compiled production
preflight with an injected unavailable NTP adapter: it advanced past host
support to the stable `clock_unsynchronized` denial, created no service user,
config, init state, database, systemd unit, or public listener, and left only
SSH listening after temporary evidence cleanup.
Init remains incomplete unless the configured platform host proves the public
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
VPN membership must not replace TinyHost app identity, session, or policy
checks. Users retain TinyHost email OTP login and per-app authorization in the
first VPN-only mode; central SSO remains a separate later candidate. This work
is not authorized to weaken the M5 public proof and is not scheduled ahead of
the committed M6 blob slice.

## M6 — App-scoped local blob storage

Outcome: immediately after the V1 M0–M5 exit gate, a static app can upload,
download, list, inspect, and delete bounded files through `@tinyhost/sdk`
without provisioning object storage or another server.

Components:

- a technology-neutral blob capability contract and deny charter;
- an ADR fixing the local-disk persistence, atomic commit, cleanup, and
  metadata/filesystem recovery model;
- streaming gateway handlers requiring the typed authorization context;
- app-scoped metadata and quota state in SQLite;
- private local blob and staging namespaces under the TinyHost data directory;
- typed SDK operations with cancellation, bounded metadata, and actionable
  quota/disk-pressure errors; and
- cleanup and disagreement recovery that never accepts a client path or serves
  a partial file.

Constraints:

- local VPS disk is the only blob byte store for this slice; high throughput,
  multi-node distribution, object storage, and business-critical durability are
  explicit non-goals;
- blob/app/viewer identity is server-derived on every operation;
- no second file server, raw filesystem URL, public listener, selectable app
  ID, or browser-visible storage credential is introduced;
- uploads stream into private staging, validate size/quota/metadata, and become
  readable only through an atomic committed metadata-and-file state; and
- disk warning/write-stop policy, app suspension, policy/session revocation,
  quotas, audit, and cleanup apply immediately and fail closed.

Exit gate:

- anonymous, wrong-app, revoked, malformed, oversized, quota-exceeded, and
  disk-stop operations expose no blob bytes and create no committed blob;
- interruption or injected filesystem/SQLite failure never makes a partial
  upload readable and preserves prior valid metadata/file state;
- two-app repository, HTTP, and SDK isolation matrices pass;
- deleting or cleaning a blob cannot escape its server-derived app namespace;
  and
- the SDK examples and TinyHost agent skills cover upload, download, list,
  delete, limits, denial verification, and the local-disk durability disclaimer.

## Later post-V1 candidates

Rank only after usage evidence:

1. Durable realtime history/replay and multi-node fan-out.
2. Central SSO exchange.
3. Wildcard DNS provider adapters.
4. Temporary invitations/groups.
5. Backend runtime (separate security concept).
6. Operator backup and disaster recovery.
7. Operator-governed LLM and internal-service capability broker.
