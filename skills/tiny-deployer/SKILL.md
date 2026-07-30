---
name: tiny-deployer
description: Build, adapt, configure, deploy, and independently verify private TinyHost static apps. Use when a deployer or deployment agent wants to create an app, use @tinyhost/sdk, choose a viewer allowlist, write tiny.yaml, authenticate with the tiny CLI, deploy an existing project, or troubleshoot a TinyHost deployment.
---

# TinyHost deployer

<!-- shared:role-common:start -->
## Platform boundary

Use `operator` for the person who hosts TinyHost, `deployer` for a person
authorized to create and manage their own apps, and `viewer` for a person who
accesses an app. A deployment agent is automation acting through deployer
authority. Never transfer credentials or authority between these roles.

TinyHost V1 hosts private static apps. The gateway owns TLS, app routing,
viewer authentication, access policy, static files, SDK capabilities, and
deployment activation. It derives the app from the hostname and the viewer from
an opaque app-host session. App code must never select either identity.

Fail closed. Missing, stale, malformed, redirected, incompatible, or ambiguous
security state is a denial, not a value to guess. Never expose or request a
deployer token, OTP, provider secret, database credential, app ID, or viewer ID
in chat, argv, source code, `tiny.yaml`, browser storage, logs, or output.

V1 is private-only. The app owner is always an implicit viewer. There is no
public mode. Each app has its own private SQLite data file for KV and bounded
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

Do not report success from a happy path alone. A deployment is complete only
after a fresh anonymous request through the real HTTPS gateway proves that no
app content is exposed.
<!-- shared:role-common:end -->

<!-- shared:deployer:start -->
## Deployer workflow

Own the whole path from app idea or existing project to a verified protected
URL. Do not make the deployer combine a separate development skill.

### 1. Establish the outcome and inspect first

Inspect the project, framework, existing `tiny.yaml`, package scripts, built
output, and current Tiny CLI state before asking questions. Decide whether the
task is:

- build and deploy;
- adapt an existing app to TinyHost and deploy; or
- deploy an already-built static app.

Continue safe local inspection and implementation while waiting for a
non-secret answer. Stop before an unresolved access broadening or live
deployment.

Discover the TinyHost platform URL from an explicit request, an already
verified Tiny default, or current CLI state. If it remains missing, ask for the
operator-provided platform URL, for example `https://tiny.example.com`. Accept
only normalized HTTPS. Never invent it, derive it from an app hostname, follow
a redirect, or accept insecure TLS.

Confirm the deployer-only `tiny` CLI is installed with `tiny version`. If it is
missing, use the operator-provided signed client release and reviewed installer,
or build `./cmd/tiny` from a trusted checkout of the matching TinyHost release.
An operator-approved npm/pnpm/Yarn/Bun package or Homebrew formula is acceptable
only when it was derived from that same verified release; a package-manager
name or `latest` tag alone is not compatibility or provenance evidence.
Never install the privileged `tinyhost` server binary on a deployer machine,
download an unsigned executable, or treat an unauthenticated installer URL as
its own trust root. Ask the operator for the signed client source/release
location when it cannot be discovered safely.

Do not ask the deployer for a bearer or OTP. Use the interactive CLI credential
flow when needed:

```sh
tiny login --server https://tiny.example.com
tiny whoami
```

`tiny login` reuses a valid server-bound credential. Only a definite
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
`tiny.yaml` policy. When editing an existing app, show the complete resulting
email/domain set; do not describe a partial list as merely additive.

### 3. Build on the client SDK

Use `@tinyhost/sdk` for app identity and TinyHost capabilities. Install it with
the project's existing package manager and bundle it into the static build.
Reuse the compatible version already in the lockfile. For a new app, use the
operator's version-matched published package or reviewed local SDK artifact;
never write `latest`, guess a version, vendor an improvised transport client, or
load the SDK from a CDN. If the compatible package cannot be discovered, ask
for its signed release/source location and continue with non-SDK work.

App code imports the configuration-free, same-origin client:

```ts
import {
  TinyCapabilityUnavailableError,
  TinyVersionConflictError,
  tiny,
} from "@tinyhost/sdk";

const controller = new AbortController();
const [current, app, available] = await Promise.all([
  tiny.user.current({ signal: controller.signal }),
  tiny.app.info({ signal: controller.signal }),
  tiny.capabilities.list({ signal: controller.signal }),
]);

console.log(current.identity.id, current.identity.email);
console.log(current.app.slug, app.slug);
console.log(available.capabilities);
```

There is no API URL, app ID, token, cookie, or secret to configure. Do not
reimplement login, authorization, storage HTTP calls, or WebSocket framing.

`tiny.user.current()` returns `{ identity: { id, email }, app: { slug } }`.
`tiny.app.info()` returns `{ slug }`.
`tiny.capabilities.list()` returns `{ capabilities: [{ name, version, limits?
}] }`. Use a capability only after confirming the matching `name` entry and
deliberately enabling its manifest feature. Code inspection may suggest a
feature but must not grant it automatically.

#### JSON KV

KV is app-shared current state. Values are JSON; keys are bounded strings.
Paginate lists and use optimistic versions for updates and deletes:

```ts
const current = await tiny.kv.get("poll/options", {
  signal: controller.signal,
});
const saved = await tiny.kv.set(
  "poll/options",
  { choices: ["A", "B"] },
  { expectedVersion: current?.version, signal: controller.signal },
);
const page = await tiny.kv.list({
  prefix: "poll/",
  limit: 100,
  signal: controller.signal,
});
await tiny.kv.delete(saved.key, {
  expectedVersion: saved.version,
  signal: controller.signal,
});
```

Handle `TinyVersionConflictError` by rereading current state and reconciling;
do not blindly overwrite another viewer's change.

#### Reactive document collections

Use a collection for small app-owned JSON records rather than inventing a
browser database, raw SQL endpoint, or a per-viewer backend. The platform
assigns opaque `doc_...` IDs. Collection names are bounded lowercase names;
documents must be JSON objects; mutations use optimistic versions.

```ts
const tasks = tiny.db.collection<{ title: string; done: boolean }>("tasks");
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
collection listeners from one Tiny client share one socket; cleanup removes
only that listener, while explicit `tiny.live.channel(name)` remains a separate
application channel. Do not treat a socket event as a database row, expect
replay/history/ordering, or use the collection API as arbitrary SQL, joins,
server functions, or a high-volume event log.

#### Operator-governed LLM chat

An app can request chat only when its reviewed manifest enables
`capabilities.llm.chat: true` **and** the operator grants a fixed approved
profile to that app. The app sends only its bounded chat request:

```ts
const answer = await tiny.llm.chat.complete({
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
tiny dev
```

It serves the manifest build output (or the supplied directory), stores local
state under `.tiny/local/`, and prints a conspicuous local development
identity. It emulates current viewer/app information, KV, document collections,
and their live freshness hints on `localhost` only. It deliberately does **not**
emulate login, deployment, access policy, blobs, operator LLM connections, or
`llm.chat`; never use it as production authorization evidence. Stop it with
Ctrl-C. A real deploy still needs the normal HTTPS anonymous-denial proof.

#### Blobs

Blobs are immutable, app-shared attachments with opaque server-issued IDs:

```ts
const stored = await tiny.blobs.upload(file, {
  signal: controller.signal,
});
const bytes = await tiny.blobs.get(stored.id, {
  signal: controller.signal,
});
const blobs = await tiny.blobs.list({ limit: 100 });
await tiny.blobs.delete(stored.id);
```

Names are display metadata, never paths. There are no public/signed blob URLs,
folders, buckets, provider endpoints, browser credentials, or per-viewer blob
ACLs in V1.

#### Realtime

Use custom channels for ephemeral hints:

```ts
const channel = tiny.live.channel("updates");
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
const stop = tiny.live.onKvChange(
  { prefix: "poll/" },
  () => void readCurrentState(),
);
```

Read KV after initial connect, every reconnect, each live hint, and tab
visibility recovery. Never make a live event the source of truth.

#### Failures and cancellation

Pass `AbortSignal` to network methods and make retry/cancel states visible.
Handle the typed categories relevant to the UI:

- `TinyNotAuthenticatedError`;
- `TinyNotAuthorizedError`;
- `TinyCapabilityUnavailableError`;
- `TinyValidationError`;
- `TinyVersionConflictError`;
- `TinyQuotaExceededError`;
- `TinyRateLimitedError`;
- `TinyTemporarilyUnavailableError`; and
- `TinyVersionIncompatibleError`.

Show safe request IDs when present. Do not reveal internal paths, credentials,
policy membership, or raw server responses.

### 4. Create the deployment receipt

`tiny.yaml` is a strict, reviewable receipt, not prerequisite paperwork. Reuse
an existing valid manifest. Otherwise infer one valid directory-based slug and
one unambiguous conventional output directory. If output is missing, identify
the exact project-owned build action; do not deploy source arbitrarily or make
`tiny deploy` run a guessed build.

Use this V1 shape and omit unused optional sections:

```yaml
version: 1
name: team-pulse
description: Lightweight team check-ins.

build:
  output: dist

access:
  mode: private
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
- `access.mode` is always `private`; empty allowlists mean owner-only.
- Enable only capabilities the app uses and the deployer deliberately accepts.
- `spa.fallback` names a normal file inside the built output.
- Unknown keys are errors.
- Never place a URL, app ID, token, email provider secret, or database setting
  in the manifest.

The human shortcut `tiny init [DIR]` creates but never overwrites this receipt.
`tiny deploy [DIR]` can run the same bounded setup when it is absent. JSON mode
never prompts, logs in, generates a manifest, or stores inferred state.

Validate and build with the project's own declared commands:

```sh
tiny inspect-manifest tiny.yaml
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
tiny deploy .
```

Do not upload only `dist` when `tiny.yaml` is in the project root. The CLI
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
