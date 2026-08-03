---
name: tinkercloud-deployer
description: Build, adapt, configure, deploy, and independently verify private Tinkercloud static apps. Use when a deployer or deployment agent wants to create an app, use @tinkercloud/sdk, choose a viewer allowlist, write tinker.yaml, authenticate with the Tinker CLI, deploy an existing project, or troubleshoot a Tinkercloud deployment.
---

# Tinkercloud deployer

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

For a fresh deployment, `<SERVER>` is the exact normalized HTTPS admin URL
supplied once by the operator. Never invent it, derive it from an app hostname,
follow a redirect, or accept insecure TLS. The one deploy command verifies and
saves that exact URL directly without redirects. It then reuses a valid
server-bound bearer or performs exactly one CLI email-and-OTP login only when
the bearer is absent, unauthorized, or expired. Do not run standalone `tinker whoami` or `tinker login` before this fresh first deployment.

Confirm the deployer-only `tinker` CLI is installed with `tinker version`. If it
is missing, first probe the prepared exact `v0.1.6` client asset, then install
and confirm the exact version:

```sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 --head --location --fail --silent --show-error --max-time 15 -o /dev/null https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v0.1.6
curl --proto '=https' --proto-redir '=https' --tlsv1.2 --location --fail --silent --show-error --max-time 15 --max-filesize 32768 -o /dev/null https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-client.sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-client.sh | sh
tinker version
```

The immutable installer verifies its release evidence. Do not substitute
`latest`, a package-manager tag, or a caller-supplied release origin.
`tinker version` must print exactly `tinker 0.1.6`; if it prints anything else, stop; do not
deploy with it or substitute another version. If the exact `v0.1.6` release or
the required installer asset for this role is unavailable, stop: do not install,
deploy, or substitute another version. Minimal HTTPS release readiness is the
exact tag page plus this role's exact installer asset URL returning HTTPS
success; the installer remains the checksum/signature authority.
Never install the privileged `tinkercloud` server binary on a deployer machine,
download an unsigned executable, or treat an unauthenticated installer URL as
its own trust root. Ask the operator for the signed client source/release
location when it cannot be discovered safely.

Do not ask the deployer for a bearer or OTP. During fresh deployment, let the
one deploy command manage its internal server verification and authentication;
when login is needed, the only authentication prompts are exactly `Email: ` and
`Code: `. Enter the code only in the CLI. For later troubleshooting after this
fresh single-command deployment, `tinker whoami --server <remembered-server>`
and `tinker login --server <remembered-server>` may diagnose or refresh a
credential; they are explicitly not part of the fresh first-deploy path.
This bounded beta path permits one human CLI OTP only when that saved bearer is
absent, unauthorized, or expired. Prepare the first deploy **owner-only**, but
do not invoke it until the final reviewed deployment step. Completion needs
anonymous HTML, asset, and reserved API denial evidence with no app bytes.

Terminal CLI authentication rule: immediately after a CLI authentication or OTP
attempt fails, is malformed, times out, or is denied, stop that deploy attempt.
Do not run standalone `tinker whoami` or `tinker login` before this fresh first
deployment; those later troubleshooting commands are not a retry path.
Do not retry `tinker login`, use `--force`, switch account or identity, log out,
delete or clear saved credentials, or create an alternate OTP path. A valid
exact server-scoped saved identity uses zero OTP. Enter an eligible OTP directly
in the CLI, never chat.

One-invocation deployment rule: after the final review, invoke `tinker --server
<SERVER> deploy .` (or the equivalent explicit public-confirmation form)
exactly once for that deploy attempt.
Any CLI deploy outcome—validation, upload, activation, verification,
transport/TLS/redirect/timeout/error, or success—ends the agent invocation; do
not rerun deploy, upload another release, or retry from chat. The CLI may use
its already-bounded transient readiness retries inside that one invocation.
`active_but_unverified` permits only the existing independent exact-URL recheck:
fresh anonymous `curl --include --silent --show-error --no-location --cookie '' --max-time 15 --max-filesize 32768 -H 'Accept: application/json' <returned-url>` with no authentication, requiring `401`, `Cache-Control: no-store`, JSON `not_authorized`, no `Set-Cookie` or `Location`, and no app bytes. It is evidence only, never a second deployment. A later deploy attempt requires an explicit new human
request after the cause is addressed, not an automatic retry.

Success records the immutable deployment ID, protected exact app origin,
authenticated platform-health success, and anonymous HTML, asset, and reserved
API denial with no app bytes.
Human success prints `Deployment: <id>`, `State: active`, and `URL:
<exact-origin>`; JSON returns `valid:true` with a bounded deployment object.
Platform-health and anonymous probes are internal success preconditions, not
extra credentials or follow-up actions.

Use the matching browser SDK for a new project:

```sh
npm install @tinkercloud/sdk@0.1.6
```

Across the operator dashboard and deployer CLI: One browser OTP maximum for the
operator and one CLI OTP maximum for the deployer; the two-role total is at
most two and never permits two OTPs for either role. Before issuing, relaying,
requesting, or suggesting a third human OTP, stop immediately; never switch
accounts, clear credentials, create a viewer session, or use another mailbox to
evade the budget.

A fully fresh successful human onboarding with neither a reusable browser
identity nor a saved CLI bearer requests exactly two codes total: exactly one
operator browser code and exactly one deployer CLI code. A reusable identity
reduces the relevant lane to zero.

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

Use `@tinkercloud/sdk` for app identity and Tinkercloud capabilities. Install
the prepared exact public-beta `0.1.6` package with the project's existing
package manager only after that prerelease is published, then bundle it into
the static build. Do not write `latest`, guess a version, vendor an improvised
transport client, or load the SDK from a CDN.

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

- `name` is a valid stable lowercase app slug; do not silently rewrite it.
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

Before this review, lowercase the project basename only when it is a valid
nonreserved DNS label. Otherwise the CLI suggests `my-app`; ask one blocking
stable-slug question rather than silently accepting that generic fallback.
Inspect the actual filesystem to choose output using the existing safe rule.

For a first owner-only deploy, the wizard asks exactly these six prompts in this
order: `App slug (<suggested>, Enter to accept):`, `Description (optional):`,
`Build output (<default>, Enter to accept):`, `Allowed emails or domains,
comma-separated (optional):`, `Features (kv,blobs,realtime; optional):`, and
`SPA fallback (optional):`. Empty optional answers keep their defaults: no
description, owner-only access, no features, and no fallback; they do not
create follow-up questions. Accept the suggested valid slug or provide one
stable lowercase ASCII DNS label (1–63 characters; letters, digits, internal
hyphens; not a reserved label). Enter empty values for description, allowlist,
features, and SPA fallback unless separately reviewed; an empty allowlist is
owner-only. For the output, root `index.html` means `.`, exactly one safe
conventional output directory containing `index.html` means that directory, and
multiple or no such directories require one blocking output question. An
invalid or ambiguous slug requires one blocking stable lowercase slug question,
never a generic question. Do not invent an output, slug, feature, access grant,
or fallback.

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
already explicit. Review endpoint, slug, description, output, owner-only
access, no features, and SPA fallback; even owner-only requires affirmative
go-ahead. Then run exactly once from the project root with the reviewed wizard
answers:

```sh
tinker --server <SERVER> deploy .
```

Ask exactly: `Deploy this owner-only app to <server> now? [y/N]`. Only explicit
`yes` continues; this is the agent's pre-invocation approval, not a CLI prompt.

For an accepted post-V1 capability-free public-static release, show the
internet-access consequence plainly and require an explicit deployer decision.
Set `access.mode: public` and any deliberate `access.indexing` choice in the v2
manifest. For a new or currently private app, use `tinker --server <remembered-server>
--confirm-public deploy .`;
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
