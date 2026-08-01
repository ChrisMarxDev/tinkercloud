---
name: tinkercloud-platform
description: Canonical Tinkercloud agent guidance for building, deploying, verifying, and operating private Tinkercloud apps. Use when maintaining Tinkercloud role skills or when a task genuinely spans both deployer and operator responsibilities.
---

# Tinkercloud platform

This is the canonical authoring source. For a human workflow, prefer the
self-contained `tinkercloud-deployer` or `tinkercloud-operator` role skill.

An opt-in local deployer-workstation test wrapper may drive the normal CLI OTP
flow through the controlled local Resend reader, but it is not production
CI/noninteractive deployment-agent authentication. Production deployment
agents require separately provisioned app-scoped deployer tokens and never
receive an implicit credential write or prompt.

<!-- shared:role-common:start -->
## Platform boundary

Use `operator` for the person who hosts Tinkercloud, `deployer` for a person
authorized to create and manage their own apps, and `viewer` for a person who
accesses an app. A deployment agent is automation acting through deployer
authority. Never transfer credentials or authority between these roles.

Tinkercloud V1 hosts private static apps. The accepted post-V1 public-static
extension remains gateway-only: it permits only reviewed, capability-free,
immutable static files after both a current app policy and a default-off,
revisioned operator gate succeed. The gateway owns TLS, app routing,
viewer authentication, access policy, static files, SDK capabilities, and
deployment activation. It derives the app from the hostname and the viewer from
an opaque app-host session. App code must never select either identity.

Fail closed. Missing, stale, malformed, redirected, incompatible, or ambiguous
security state is a denial, not a value to guess. Never expose or request a
deployer token, OTP, provider secret, database credential, app ID, or viewer ID
in chat, argv, source code, `tinker.yaml`, browser storage, logs, or output.

V1's historical release boundary is private-only, and the app owner is always
an implicit viewer. In the accepted post-V1 extension, public is never a
default or a capability grant: a public release needs `access.mode: public`,
no browser capabilities, explicit deployer acknowledgement, and the current
operator gate. Reserved `/_tinker/*` routes remain private-viewer-only; a
public release cannot expose identity, SDK, KV, collections, blobs, realtime,
or LLM access. Each app has its own private SQLite data file for KV and bounded
JSON document collections; blobs and that data are utility-grade state on one
VPS, so losing the VPS can lose them. Realtime is app-scoped, in-memory,
best-effort notification with no history, replay, ordering, or delivery
guarantee. Live collection events are freshness hints: recover current state
with a snapshot after first connect, reconnect, visibility recovery, and every
hint.

`llm.chat`, when an operator grants it, is a narrow server-side capability.
Capability discovery is absent until the app requests it and the operator grant
is active. If present, treat its disclosure as a notice that prompt content is
sent to an operator-selected external AI provider; discovery limits are safe
current bounds, not a promise that a later request will be admitted.
Provider credentials, connection IDs, model names, and upstream URLs never
enter app code, browser storage, the deployer manifest, or the SDK request.

Start from the requested outcome. Inspect trusted local and server state, reuse
verified values, and choose a secure default before asking anything. Ask only
for a required value or decision that is still unknown, cannot be discovered,
and cannot be defaulted safely. Group unresolved optional choices into one
review. Never infer broader authority.

Do not report success from a happy path alone. A private deployment is complete
only after a fresh anonymous HTTPS request proves no app content is exposed. A
public-static deployment additionally needs the exact anonymous document/asset
and indexing proof plus denial of every reserved route through that gateway.
<!-- shared:role-common:end -->

<!-- shared:deployer:start -->
## Deployer workflow

Own the whole path from app idea or existing project to a verified protected
URL. Do not make the deployer combine a separate development skill.

### 1. Establish the outcome and inspect first

Inspect the project, framework, existing `tinker.yaml`, package scripts, built
output, and current Tinker CLI state before asking questions. Decide whether the
task is:

- build and deploy;
- adapt an existing app to Tinkercloud and deploy; or
- deploy an already-built static app.

Continue safe local inspection and implementation while waiting for a
non-secret answer. Stop before an unresolved access broadening or live
deployment.

Discover the Tinkercloud platform URL from an explicit request, an already
verified Tinker default, or current CLI state. If it remains missing, ask for the
operator-provided admin URL, for example `https://admin.example.com`. Accept
only normalized HTTPS. Never invent it, derive it from an app hostname, follow
a redirect, or accept insecure TLS.

Remember the exact normalized `{server URL, deployer email}` pair after
`tinker whoami` verifies both. Reuse this non-secret context in the current task
and later follow-ups instead of asking again. Treat conversation context only
as a candidate and reverify before mutation. Keep the pair only in conversation
context and Tinker's protected per-server records, never in app or project files.

For a known server, run `tinker whoami --server <remembered-server>` before
concluding login is missing; a bare `tinker whoami` may inspect another default.
If valid, continue without login or an email question. If definitely expired,
run `tinker login --server <remembered-server>`, reuse the remembered email at
the CLI prompt, and let the deployer enter only the OTP there. Replace the pair
with the exact server and email returned by the successful `whoami`.

Change the pair only after an explicit switch, logout, or contradictory verified
state. Never transfer an email between servers. If `whoami` returns another
email, stop before mutation and ask whether to adopt it or use
`tinker login --force` for the intended account.

Confirm the deployer-only `tinker` CLI is installed with `tinker version`. If it is
missing, use the operator-provided signed client release and reviewed installer,
or build `./cmd/tinker` from a trusted checkout of the matching Tinkercloud release.
An operator-approved npm/pnpm/Yarn/Bun package or Homebrew formula is acceptable
only when it was derived from that same verified release; a package-manager
name or `latest` tag alone is not compatibility or provenance evidence.
Never install the privileged `tinkercloud` server binary on a deployer machine,
download an unsigned executable, or treat an unauthenticated installer URL as
its own trust root. Ask the operator for the signed client source/release
location when it cannot be discovered safely.

Do not ask the deployer for a bearer or OTP. Use the interactive CLI credential
flow when needed:

```sh
tinker login --server https://admin.example.com
tinker whoami
```

`tinker login` reuses a valid server-bound credential. Only a definite
unauthorized/expired credential may fall back to OTP. Let the deployer enter
the OTP into the CLI prompt, not the conversation. An explicit `--server`
applies only to that command; successful login stores the verified default.

### 2. Propose the access policy

Always include an access proposal in the pre-deploy summary. If the deployer
already named the audience, translate it exactly. Otherwise propose
**owner-only** and offer one edit:

- owner-only: safest for personal work or a first preview;
- exact email addresses: best for a few named viewers or mixed domains;
- exact email domains: grants every matching address and is intentionally
  broader; use only when the deployer explicitly wants the whole domain; or
- a deliberate combination of exact emails and domains.

Make the proposal concrete. For example: “Keep this owner-only for the first
deploy; add `alice@example.com` for review; use `example.com` only if every
address at that domain should be allowed.”

Ask for the unresolved audience decision once, together with optional
description, capabilities, and SPA fallback. Do not ask a series of questions
whose answers are already inferable or optional. Never infer a company domain,
group membership, public access, or an email list.

An activation replaces the app's exact allowlist with the candidate
`tinker.yaml` policy. When editing an existing app, show the complete resulting
email/domain set; do not describe a partial list as merely additive.

### 3. Build on the client SDK

Use `@tinkercloud/sdk` for app identity and Tinkercloud capabilities. Install it with
the project's existing package manager and bundle it into the static build.
Reuse the compatible version already in the lockfile. For a new app, use the
operator's version-matched published package or reviewed local SDK artifact;
never write `latest`, guess a version, vendor an improvised transport client, or
load the SDK from a CDN. If the compatible package cannot be discovered, ask
for its signed release/source location and continue with non-SDK work.

App code imports the configuration-free, same-origin client:

```ts
import {
  TinkerCapabilityUnavailableError,
  TinkerVersionConflictError,
  tinker,
} from "@tinkercloud/sdk";

const controller = new AbortController();
const [current, app, available] = await Promise.all([
  tinker.user.current({ signal: controller.signal }),
  tinker.app.info({ signal: controller.signal }),
  tinker.capabilities.list({ signal: controller.signal }),
]);

console.log(current.identity.id, current.identity.email);
console.log(current.app.slug, app.slug);
console.log(available.capabilities);
```

There is no API URL, app ID, token, cookie, or secret to configure. Do not
reimplement login, authorization, storage HTTP calls, or WebSocket framing.

`tinker.user.current()` returns `{ identity: { id, email }, app: { slug } }`.
`tinker.app.info()` returns `{ slug }`.
`tinker.capabilities.list()` returns `{ capabilities: [{ name, version, limits?
}] }`. Use a capability only after confirming the matching `name` entry and
deliberately enabling its manifest feature. Code inspection may suggest a
feature but must not grant it automatically.

#### JSON KV

KV is app-shared current state. Values are JSON; keys are bounded strings.
Paginate lists and use optimistic versions for updates and deletes:

```ts
const current = await tinker.kv.get("poll/options", {
  signal: controller.signal,
});
const saved = await tinker.kv.set(
  "poll/options",
  { choices: ["A", "B"] },
  { expectedVersion: current?.version, signal: controller.signal },
);
const page = await tinker.kv.list({
  prefix: "poll/",
  limit: 100,
  signal: controller.signal,
});
await tinker.kv.delete(saved.key, {
  expectedVersion: saved.version,
  signal: controller.signal,
});
```

Handle `TinkerVersionConflictError` by rereading current state and reconciling;
do not blindly overwrite another viewer's change.

#### Reactive document collections

Use a collection for small app-owned JSON records rather than inventing a
browser database, raw SQL endpoint, or a per-viewer backend. The platform
assigns opaque `doc_...` IDs. Collection names are bounded lowercase names;
documents must be JSON objects; mutations use optimistic versions.

```ts
const tasks = tinker.db.collection<{ title: string; done: boolean }>("tasks");
const task = await tasks.create({ title: "Ship", done: false });
await tasks.update(task.id, { ...task.data, done: true }, {
  expectedVersion: task.version,
});

const stop = tasks.subscribe({
  onSnapshot: ({ documents }) => render(documents),
  onUpdate: () => void refreshUI(),
  onStatus: (status) => showConnectionState(status),
});
// Cleanup: stop()
```

Subscriptions deliberately reconcile through snapshots. Managed KV and
collection listeners from one Tinker client share one socket; cleanup removes
only that listener, while explicit `tinker.live.channel(name)` remains a separate
application channel. Do not treat a socket event as a database row, expect
replay/history/ordering, or use the collection API as arbitrary SQL, joins,
server functions, or a high-volume event log.

#### Operator-governed LLM chat

An app can request chat only when its reviewed manifest enables
`capabilities.llm.chat: true` **and** the operator grants a fixed approved
profile to that app. The app sends only its bounded chat request:

```ts
const answer = await tinker.llm.chat.complete({
  messages: [{ role: "user", content: "Summarize this checklist." }],
});
```

There is no API key, provider choice, model selector, endpoint, system-secret
field, streaming socket, or direct provider SDK. Show capability, quota, rate,
and temporary-unavailability failures safely; do not retry blindly or expose
provider error bodies.

#### Local development

Use the built-in loopback preview for normal app work:

```sh
tinker dev
```

It serves the manifest build output (or the supplied directory), stores local
state under `.tinker/local/`, and prints a conspicuous local development
identity. It emulates current viewer/app information, KV, document collections,
and their live freshness hints on `localhost` only. It deliberately does **not**
emulate login, deployment, access policy, blobs, operator LLM connections, or
`llm.chat`; never use it as production authorization evidence. Stop it with
Ctrl-C. A real deploy still needs the normal HTTPS anonymous-denial proof.

#### Blobs

Blobs are immutable, app-shared attachments with opaque server-issued IDs:

```ts
const stored = await tinker.blobs.upload(file, {
  signal: controller.signal,
});
const bytes = await tinker.blobs.get(stored.id, {
  signal: controller.signal,
});
const blobs = await tinker.blobs.list({ limit: 100 });
await tinker.blobs.delete(stored.id);
```

Names are display metadata, never paths. There are no public/signed blob URLs,
folders, buckets, provider endpoints, browser credentials, or per-viewer blob
ACLs in V1.

#### Deployer data access

Use `tinker data` only to inspect or deliberately repair the managed KV and JSON
documents of an app you own. It reuses the saved CLI login and the server
derives the private app database from the owned slug:

```sh
tinker data kv list team-pulse --prefix poll/
tinker data kv get team-pulse poll/options
tinker data documents list team-pulse tasks --limit 50
tinker data documents get team-pulse tasks doc_abcdefghijklmnopqrstuv
```

`data:read` is bounded inspection. `data:write` permits one mutation at a time;
re-read first and supply the returned version for an update or delete. A KV set
may use an optional expected version, while destructive deletes require the
exact target-bound `delete:...` phrase, either through the interactive prompt
or `--confirm delete:...`. Use `--json` only for deterministic automation: it
never prompts, requires `--confirm`, and must not mix progress text with the
JSON result.

CLI bearers created before this capability do not silently gain data scopes.
If an exact owner receives `not_authorized` after a server upgrade, run
`tinker login --force` once to issue a fresh interactive credential.

Never use or ask for a database URL, app ID, database credential, SQLite file,
SQL shell, schema/migration command, export/import, bulk delete, or viewer
impersonation. A deployer cannot access another deployer's app, a
deployment-agent token has no data scope by default, and suspended/deleting
app lifecycle state may deny a mutation or all access. Do not place app data in
chat, logs, or source control. See `specs/api/deployer-data-contract.md` for
the exact scoped commands and denial behavior.

#### Realtime

Use custom channels for ephemeral hints:

```ts
const channel = tinker.live.channel("updates");
channel.subscribe();
const off = channel.on("changed", () => void readCurrentState());
await channel.connect();
channel.publish("changed", { refresh: true });

// On cleanup:
off();
channel.unsubscribe();
channel.close();
```

For KV notifications:

```ts
const stop = tinker.live.onKvChange(
  { prefix: "poll/" },
  () => void readCurrentState(),
);
```

Read KV after initial connect, every reconnect, each live hint, and tab
visibility recovery. Never make a live event the source of truth.

#### Failures and cancellation

Pass `AbortSignal` to network methods and make retry/cancel states visible.
Handle the typed categories relevant to the UI:

- `TinkerNotAuthenticatedError`;
- `TinkerNotAuthorizedError`;
- `TinkerCapabilityUnavailableError`;
- `TinkerValidationError`;
- `TinkerVersionConflictError`;
- `TinkerQuotaExceededError`;
- `TinkerRateLimitedError`;
- `TinkerTemporarilyUnavailableError`; and
- `TinkerVersionIncompatibleError`.

Show safe request IDs when present. Do not reveal internal paths, credentials,
policy membership, or raw server responses.

### 4. Create the deployment receipt

`tinker.yaml` is a strict, reviewable receipt, not prerequisite paperwork. Reuse
an existing valid manifest. Otherwise infer one valid directory-based slug and
one unambiguous conventional output directory. If output is missing, identify
the exact project-owned build action; do not deploy source arbitrarily or make
`tinker deploy` run a guessed build.

Use this V1 shape and omit unused optional sections:

```yaml
version: 2
name: team-pulse
description: Lightweight team check-ins.
tags:
  - team

build:
  output: dist

access:
  mode: private
  indexing: false
  allow:
    emails:
      - alice@example.com
    domains:
      - example.org

features:
  kv: true
  blobs: false
  realtime: true

capabilities:
  llm:
    chat: false

spa:
  fallback: index.html
```

Rules:

- `name` is a valid stable app slug; do not silently rewrite it.
- `build.output` and optional fallback stay beneath the project with no
  symlinks or traversal.
- `access.mode` is private by default. The accepted post-V1 `public` mode can
  activate only when the operator's current public gate is enabled, the release
  is capability-free, and the deployer gives the explicit CLI acknowledgement;
  otherwise it fails closed and preserves the earlier active release. Empty
  private allowlists mean owner-only.
- Tags are optional, lowercase ASCII, 1–24 characters, internally hyphenated
  at most, unique, and limited to eight. `access.indexing` is valid only for a
  public release and defaults false; successful documents are `noindex,
  nofollow` unless both the effective public policy and immutable indexing opt-in
  are current.
- Enable only capabilities the app uses and the deployer deliberately accepts.
- `spa.fallback` names a normal file inside the built output.
- Unknown keys are errors.
- Never place a URL, app ID, token, email provider secret, or database setting
  in the manifest.

The human shortcut `tinker init [DIR]` creates but never overwrites this receipt.
`tinker deploy [DIR]` can run the same bounded setup when it is absent. JSON mode
never prompts, logs in, generates a manifest, or stores inferred state.

Validate and build with the project's own declared commands:

```sh
tinker inspect-manifest tinker.yaml
# Run the existing project build command, then its tests/typecheck if present.
```

Do not invent a package manager or arbitrary build script. Confirm the declared
output exists and contains the expected static entry point.

### 5. Deploy and verify

Show one final summary containing:

- verified platform URL;
- project and build output;
- app slug and optional description;
- complete resulting viewer emails/domains, explicitly labeling owner-only;
- enabled capabilities and their utility-grade limits;
- SPA fallback; and
- the fact that deployment will create an immutable release and replace the
  current access policy atomically.

Name any access broadening and obtain the deployer's decision if it was not
already explicit. Then run from the project root:

```sh
tinker deploy .
```

For an accepted post-V1 capability-free public-static release, show the
internet-access consequence plainly and require an explicit deployer decision.
Set `access.mode: public` and any deliberate `access.indexing` choice in the v2
manifest. For a new or currently private app, use `tinker deploy --confirm-public .`;
the CLI reads the authenticated deployer's current policy before asking or
sending that acknowledgement. A public-to-public redeploy needs neither a
prompt nor a fresh acknowledgement. Never add the flag, change a private app to
public, or enable indexing by inference. If the operator gate is disabled or
unavailable, stop: a retry or a client flag cannot broaden access. Verify
anonymous root HTML and one immutable asset against the receipt's
lowercase SHA-256 and exact byte size (sizes can exceed the denial-body cap),
the expected `X-Robots-Tag`, and exact safe denials for all representative
`/_tinker/*` routes without cookies.

For an existing app policy, first read the current policy with `tinker access get
APP`. `tinker access set APP --file policy.json` accepts only writable policy
members (`mode`, optional `expected_revision`, `confirm_broadening`, and
`allow`); never paste the read-only `revision` member from a GET response. In
an interactive terminal, Tinker names a newly added email/domain and asks once
before writing. In JSON/non-interactive automation, set
`confirm_broadening: true` in the file explicitly; otherwise Tinker returns
`confirmation_required` without sending the write. A stale expected revision
is a conflict, not permission to overwrite a newer policy.

Do not upload only `dist` when `tinker.yaml` is in the project root. The CLI
validates the project, streams an archive, activates only after policy and TLS
readiness, preserves the last known-good release on failure, and performs its
own fresh anonymous HTTPS denial proof.

Treat any deploy error, redirect, TLS failure, timeout, `2xx` anonymous app
response, malformed denial envelope, or missing denial evidence as failure.
Never send the deployer bearer or browser cookies to the app origin to test
anonymous access. Do not weaken TLS or substitute a local preview.

The CLI retries only bounded transient first-host DNS, TLS, transport, and
gateway-readiness failures. If it returns `active_but_unverified`, do not claim
success and do not immediately upload another identical release: report the
returned deployment ID, protected URL, and safe reason, then independently
recheck the exact URL. This outcome means server activation committed but the
CLI's anonymous public proof did not complete. Redirects, public `2xx`,
wrong-origin URLs, malformed denials, and unsafe headers remain terminal and
are never retryable.

After CLI success, verify the authenticated viewer path only if the environment
allows it without exposing credentials. Do not claim a viewer check that was
not run.

### 6. Report the result

Report only observed facts:

- protected app URL;
- deployment/release identifier when returned;
- exact access summary;
- enabled capabilities;
- build/tests actually run;
- successful anonymous-denial evidence; and
- any unverified viewer behavior or utility-grade persistence limitation.

If blocked, give one exact next action and preserve completed safe work. Do not
repeat already verified questions on resume.
<!-- shared:deployer:end -->

<!-- shared:operator:start -->
## Operator workflow

Operate Tinkercloud as one small security boundary: one `tinkercloud` server, one
SQLite database, one private data directory, and one systemd service on a
dedicated supported Hetzner VPS. Only Tinkercloud listens publicly on TCP 80/443.
Do not add Docker as the primary install, a reverse proxy, another file server,
a storage mount, a public object URL, a backend runtime, or a second recovery
authority in V1.

Use this repository's `PRINCIPLES.md`, `PRD.md`, `specs/operations/`, and
`docs/operations/` for implementation detail when available. Before host
mutation, confirm the exact target is a clean dedicated x86-64 Hetzner VPS
running explicitly supported Ubuntu 24.04 LTS or 26.04 LTS. Ambiguous,
interim, end-of-life, other-distribution, or future unverified releases deny.

For a new server, start with the signed installer and resumable
`tinkercloud setup`. Inspect first, then ask only for the controlled root domain,
initial operator email, and Resend credential source that cannot be derived.
Derive `admin.<domain>` and `<slug>.<domain>`, and persist the normalized
operator email as the internal ACME contact; never ask for, accept, or override
it through a separate ACME-contact flag, question, or environment input.
Reserve `admin`, `api`, `auth`, `status`, `www`, `docs`, and `install`, and ask
for one wildcard DNS record.
Do not reserve `tinker` or `tinkercloud`. Pause with one exact DNS or Resend action
when external state is incomplete; resume without re-asking verified answers.
Before the ACME-capable service starts, require both `admin.<domain>` and a
synthetic one-label app hostname to resolve. This proves the exact platform and
wildcard DNS setup without guessing or comparing a public VPS IP; a failure
must stop before ACME and report the affected non-secret hostname or wildcard
action.

The workstation `tinker host install|status|doctor|update` commands may perform
the same root-local flow over an explicit `root@HOST` using normal OpenSSH
host-key verification. They are not a remote control API: never add SSH
options, arbitrary remote commands, automatic host-key acceptance, or a
deployer bearer to that path.

Keep provider and Tinkercloud secrets in root-owned mode-0600 credential files or
systemd credentials. Never accept them in argv, ordinary YAML, chat, browser
state, shell history, logs, or audit. Run the gateway as the unprivileged
`tinkercloud` service identity with only the narrow bind capability for 80/443.

If offering `llm.chat`, configure it as an operator-owned capability adapter:
use the dashboard **API keys** section to choose Anthropic or Gemini and enter
one write-only provider key. Tinkercloud derives its safe label and opaque ID;
never ask for a name, ID, URL, or arbitrary secret. If the dashboard says key
management is unavailable, run the root-only `tinkercloud llm enable` and restart
before entering a key; never expose or copy the encryption root. Define a fixed
approved model/profile and bounded quotas in **LLM chat**, then grant that
profile explicitly to selected apps. The browser receives only Tinkercloud's
same-origin response; it never receives the provider key, connection ID,
provider endpoint, raw provider error, arbitrary model choice, or an outbound
proxy. Removing a grant or connection must deny the next request. The local
`tinker dev` emulator deliberately excludes LLM capability calls, so verify the
real adapter through its normal protected gateway tests.

Use `tinkercloud status` for offline state and `sudo tinkercloud doctor` for checks
that may read root credentials. Initialization is complete only after the exact
platform HTTPS version proof, route classification, socket confinement, safe
unknown-app denial, database, permissions, DNS, TLS, and email checks succeed.
Do not replace public evidence with localhost, a browser page, a redirect, or
insecure TLS.

Manage deployer authority as one revision-protected exact normalized email
allowlist. Show the complete resulting list. Require explicit confirmation only
when adding deployment authority. Removal immediately revokes that deployer's
CLI tokens and pending CLI OTPs while removing dashboard authority on the next
request. It preserves their global browser identity and independently
authorized app access.

For root recovery with `tinkercloud deployers authorize|suspend|revoke`, the
database mutation runs in a fixed child that drops permanently to `tinkercloud`.
After that child has committed and closed, Tinkercloud refreshes only an already
active service and verifies it remains active; it never starts an inactive
service. `deployer_applied_service_refresh_failed` means the authority mutation
is durable but the platform must be checked with `sudo tinkercloud doctor` before
the changed deployer is asked to log in.

Install only artifacts verified against signed release metadata. A checksum
alone is not a trust root. An update verifies both the server artifact and the
signed compatibility manifest before snapshot; air-gapped updates require both
triplets and never mix local with remote inputs. `tinkercloud update` must retain rollback state until
the restarted candidate passes doctor, listener readiness, platform health,
and anonymous protected-app denial. Any ambiguous or failed gate restores the
prior healthy state.

The updater derives its probe from installed server state and selects a locally
verified active app itself. Never ask for or pass an app ID, slug, probe host,
path, or URL. If a failed update did not complete automatic restoration, use
the installed root-local recovery path:

```sh
sudo tinkercloud update --rollback
tinkercloud status
sudo tinkercloud doctor
```

Do not invent another rollback command or delete rollback state by hand.

Local app insights default enabled. To stop or resume new local tracking, use
the root-local `tinkercloud insights disable` or `tinkercloud insights enable`.
This command delegates the durable SQLite mutation to the `tinkercloud` service
identity, then refreshes only an already active service after that child closes;
it never starts an inactive service or creates a browser/remote mutation path.
Dashboard aggregates remain owner/operator-only, and a disabled or unavailable
read must be shown as unavailable rather than zero.

Root access to the dedicated VPS is the recovery authority. If email is
unavailable, use root-only operator recovery and revoke affected global
identity families and CLI credentials; never create a remote HTTP recovery
bypass.

The accepted post-V1 public-static gate remains disabled unless an operator
explicitly changes it with the root-local `tinkercloud public enable` command.
That command must be revisioned and audited; it changes no app policy. Before
enabling it, verify the intended scope and explain that only independently
acknowledged, capability-free public releases can become anonymous. Use
`tinkercloud public disable` to revoke anonymous static access; the next
anonymous request must deny while normal private owner/viewer access remains.
The child closes its durable SQLite mutation before success and does not query,
restart, or start systemd: the gateway reads the gate on every request, so a
service refresh would add downtime without improving revocation.
Never add a public listener, proxy, file server, public capability endpoint, or
remote gate mutation.

At disk warning, diagnose and clean only through database-led Tinkercloud
operations. At the write-stop watermark, new deployments, KV mutations, and
blob uploads deny while safe reads, deletions, and revocations continue.
Never expose raw storage paths.

V1 has no operator backup or disaster-recovery feature. State plainly that
loss of the VPS can lose releases, KV, and blobs. Do not describe utility-grade
local state or realtime as durable.

For every consequential operation, report the exact target, command or
dashboard action, observed result, and remaining verification. Never claim
setup, update, recovery, or security completion from the happy path alone.
<!-- shared:operator:end -->
