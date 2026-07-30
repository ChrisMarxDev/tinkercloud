# Operator-governed LLM chat capability plan

**Status:** Post-V1 L1/L2 implemented; L3 streaming deferred
**Date:** 2026-07-29
**Roadmap position:** Implemented post-V1 extension. This does not expand the
locked V1 scope. Live VPS and dedicated real-provider smoke evidence remain
pending.

## Outcome

An operator can add an Anthropic or Gemini API key without exposing it to a
deployer, viewer, deployed bundle, SDK response, log, or audit event. After the
operator approves a binding for one app, that app can build a basic chat
interface with a small provider-neutral SDK:

```ts
const reply = await tiny.llm.chat.complete({
  messages: [
    { role: "user", content: "Summarize this review." },
  ],
}, { signal });

reply.message.content;
```

The implemented public operation is non-streaming completion. A compatible
streaming method remains the deferred L3 slice.

## Why this belongs in TinyHost

This lets deployers build useful private chat and summarization apps without
running a backend or receiving an organization credential. It extends the SDK
as the app platform while preserving the gateway and TinyHost as the only
security, policy, quota, and credential boundary.

The design is governed most directly by Principles 1, 3, 4, 8, 10, 12, 13,
and 14. It follows ADR 0008: apps invoke a narrow server-side capability and
never receive a secret or generic authenticated proxy.

## Scope

### Included in the first capability release

- Operator-managed, write-only Anthropic and Gemini connections.
- A provider-neutral `llm.chat` capability and request/response contract.
- One operator-selected model profile per app binding.
- Explicit operator approval, disable, rotation, and revocation per app.
- Non-streaming chat completion through the same-origin app API.
- Bounded message history, message sizes, output tokens, timeout, concurrency,
  per-viewer rate, per-app rate, and monthly token budget.
- Usage accounting and audit metadata without prompts or completions.
- Typed SDK errors, cancellation, capability discovery, compiled examples, and
  two-provider conformance tests.
- Clear disclosure that app content is sent to the selected external provider.

### Deferred L3 slice

- `tiny.llm.chat.stream(...)` as an async iterable over bounded text deltas.
- Cancellation and partial-response handling using the same authorization,
  reservation, usage, and audit model as non-streaming completion.
- A deployable minimal chat example with continuous sending/streaming/error
  states.

### Non-goals

- Raw API key delivery, environment-variable injection, or key export.
- A generic authenticated HTTP proxy or caller-selected provider URL/headers.
- Caller-selected connection, provider, or arbitrary model identifier.
- Tool/function calling, embeddings, images, files, web search, agents,
  conversation storage, or provider-specific request passthrough.
- Prompt/completion logging or a provider-response replay system.
- Custom/operator-written adapters or third-party code loaded into TinyHost.
- Claims that prompts remain inside TinyHost: approved content leaves the VPS
  for the selected external provider.

## Actors, trust boundary, and ownership

| Actor or component | Owns / may do | Must not do |
|---|---|---|
| Operator | Adds and rotates a provider key; defines model/limit profiles; approves or revokes an app binding | Expose key material through a read path |
| Deployer | Requests `llm.chat` in `tiny.yaml`; builds the app UI; sees the approved profile's safe limits and disclosure | Select a secret, provider URL, connection ID, or unrestricted model |
| Viewer | Sends bounded chat content from an authorized app and receives the bounded result | Use an App A session or request to consume App B's grant or budget |
| Gateway | Derives app, viewer, session, current policy, and typed authorization context | Trust browser-supplied app/viewer identity |
| LLM service | Resolves the effective app grant, reserves quota, invokes one adapter, reconciles usage, audits safe metadata | Receive an app ID from SDK input or log content |
| Provider adapter | Maps the common chat contract to one fixed allowlisted provider API | Accept arbitrary hosts, headers, paths, or credentials from the caller |

The operator owns provider credentials. The app owns any conversation state it
chooses to persist through existing KV. TinyHost owns grants, quota state,
usage records, and safe audit evidence. The external provider receives the
message content for each approved invocation.

## Implemented operator and deployer flow

1. The operator opens **API keys**, chooses Anthropic or Gemini, enters one API
   key, and submits it over the protected operator session. The key field is
   write-only; TinyHost derives the connection label and opaque identifier.
2. TinyHost validates the key with a bounded provider check, encrypts it with
   the host-local capability root, and stores only ciphertext plus safe
   metadata.
3. The operator creates a chat profile: fixed provider model, input/output
   limits, timeout, per-viewer/app rate, concurrency, and monthly token budget.
4. A deployer requests the logical capability in `tiny.yaml`:

   ```yaml
   capabilities:
     llm:
       chat: true
   ```

5. Deployment remains staged until an active operator-approved app/profile
   binding exists. An old active release remains active if approval or
   verification is missing.
6. Capability discovery returns `llm.chat` version and safe limits, never the
   provider key or internal connection identifier.
7. A currently authorized viewer calls the SDK. TinyHost rechecks the current
   grant and connection on every invocation.
8. Disabling the connection, revoking the app binding, suspending the app, or
   revoking the viewer denies the next request.

Recommended default: the app cannot select the model. The operator changes the
profile without requiring a new deployment.

## Client contract

### First slice

```ts
type ChatMessage = {
  role: "user" | "assistant";
  content: string;
};

type ChatRequest = {
  messages: ChatMessage[];
  maxOutputTokens?: number; // may only tighten the operator limit
};

type ChatResponse = {
  message: { role: "assistant"; content: string };
  usage: {
    inputTokens: number;
    outputTokens: number;
  };
  finishReason: "stop" | "length";
  requestId: string;
};

tiny.llm.chat.complete(
  request: ChatRequest,
  options?: { signal?: AbortSignal },
): Promise<ChatResponse>;
```

`messages` must alternate coherently and end in `user`. The server rejects
empty, malformed, excessive, or unsupported content before provider work.
There is no SDK field for app ID, viewer ID, connection, provider, API key,
base URL, headers, or raw provider options.

### Streaming follow-on

```ts
for await (const event of tiny.llm.chat.stream(request, { signal })) {
  if (event.type === "text") append(event.delta);
  if (event.type === "done") showUsage(event.usage);
}
```

Streaming should use one same-origin authenticated response and a versioned
event schema. Partial output is display state, not durable history; the app
persists a completed conversation explicitly if it wants recovery.

## Logic and data flow

```text
browser SDK request
→ gateway resolves active app from host
→ validates app-scoped viewer session and current policy
→ constructs sealed AuthorizationContext
→ app API validates same origin, SDK version, route, and body bounds
→ LLM service resolves active app grant from AuthorizationContext.AppID
→ verifies active operator connection and fixed model profile
→ atomically reserves worst-case token budget and concurrency slot
→ provider adapter calls one compiled-in HTTPS destination with decrypted key
→ adapter validates and normalizes the bounded provider response
→ service reconciles actual usage, releases concurrency, and appends safe audit
→ SDK receives common response or stable typed error
```

No automatic provider retry occurs in the first slice. This avoids ambiguous
duplicate spend. Cancellation stops local work and the outbound request where
possible; an ambiguous provider outcome conservatively retains its reserved
usage until reconciliation policy says otherwise.

## State model

### Server truth

- **Connection:** `validating → active ↔ disabled → revoked`, with
  `validation_failed` as a non-active result. Rotation creates a new encrypted
  credential version and atomically activates it after validation.
- **Chat profile:** active configuration containing the fixed model and safe
  limits; profile edits are revisioned and audited.
- **App grant:** `requested → approved ↔ disabled → revoked`. Only `approved`
  plus an active connection/profile enables discovery and invocation.
- **Invocation:** `admitted → reserved → calling → succeeded | failed |
  cancelled | ambiguous`. Payload content is never persisted.
- **Budget:** durable app/profile/period counters with an atomic reservation
  before outbound work and reconciliation from provider usage metadata.

### Client/UI state

The app owns a chat controller or state object outside rendering:

```text
checking-capability
→ ready-empty | ready-with-history
→ sending
→ complete | validation-error | rate-limited | quota-exhausted
           | provider-unavailable | cancelled
```

During `sending`, keep prior messages visible, append the pending viewer
message, disable duplicate send, expose cancel, and reserve a stable response
area. Transition to the assistant response without replacing the whole chat
surface. On retry, resend only after an explicit viewer action. Capability
revocation transitions to a durable unavailable state and preserves local
conversation display.

TinyHost does not own chat history. Apps may persist completed messages in KV,
subject to the existing utility-grade durability disclaimer.

## Backend and persistence changes

The accepted schema is implemented by
`migrations/0006_llm_chat_capability.sql` plus its embedded mirror:

- `provider_connections`: safe name, provider kind, encrypted credential
  envelope, key version, status, timestamps, and validation metadata.
- `llm_chat_profiles`: connection ID, fixed provider model, request/output
  bounds, timeout, rate/concurrency limits, budget period and token limit.
- `app_capability_grants`: app ID, capability/version, profile ID, status,
  revision, approving operator, and timestamps.
- `llm_usage`: app/profile/period counters and reservations; no prompt,
  completion, raw provider body, or credential data.

The encryption root is a TinyHost-owned secret supplied through the existing
root-owned/systemd credential boundary. Ciphertext may live in SQLite; the root
key may not. Plaintext exists only in bounded process memory for validation and
provider invocation.

ADR 0047 records this persistence and secret-boundary change. Migration and
update handling must preserve encrypted credentials and must not downgrade to
plaintext or silently reactivate a revoked grant.

## Implemented files and modules

| Area | Implemented change |
|---|---|
| Contract | Add `specs/capabilities/llm-chat-contract.md`; update HTTP, manifest, security, and event/audit contracts |
| Decision | Add `docs/decisions/0047-operator-governed-llm-chat.md` and index it |
| Manifest | Extend `internal/releases/manifest.go` and `specs/manifest/tiny-yaml.md` with a logical chat request, not a connection/model |
| Trust seam | Extend `apps.App` and sealed `appauth.AuthorizationContext` with an effective chat grant reference derived from active server state |
| Domain/service | Add `internal/llm/` common types, validation, admission, reservation, service, and stable errors |
| Adapters | Add compiled-in `internal/llm/anthropic` and `internal/llm/gemini` adapters with fixed destinations and conformance tests |
| Persistence | Add migration 0006 and `internal/persistence/llm.go` for connections, grants, revisions, and usage reservations |
| Gateway/API | Add a protected `LLMChat` endpoint classification and `POST /_tiny/api/v1/llm/chat`; extend capability discovery |
| Composition | Inject the service and outbound HTTP client through `internal/compose/`; add no listener or internal HTTP service |
| Operator control | Add write-only create/rotate/disable connection and profile/grant controls in `internal/control*` and embedded native UI |
| SDK | Provider-neutral types and `tiny.llm.chat.complete` in `sdk/typescript/src/index.ts`; `stream` remains L3 |
| Examples/skills | Add a private minimal chat example; update `skills/tiny-platform` first, refresh marked role-skill copies, and run drift checks |

No file or raw URL serves provider data. No backend runtime, worker process, or
second public listener is added.

## Deny-path test charter

These deny paths govern the implemented happy path:

- Anonymous, missing, malformed, wrong-app, expired, revoked, and suspended
  authorization never reaches grant storage or an adapter.
- A disabled/missing/revoked grant or connection denies before decrypting a
  credential or calling the provider.
- App A cannot discover, invoke, charge, or infer App B's connection, profile,
  grant, usage, model, or request.
- Client-supplied app/viewer/connection/provider/model/base URL/header fields
  are rejected as unknown input.
- Missing/duplicate roles, invalid UTF-8, empty messages, oversized histories,
  excessive output limits, duplicate JSON fields, and trailing bodies deny
  before quota reservation or outbound work.
- Quota, rate, and concurrency exhaustion deny before provider work and cannot
  be bypassed through parallel requests or cancellation races.
- Provider timeout, TLS failure, malformed JSON, oversized response, missing
  usage, partial stream, cancellation, and audit/persistence failure return a
  stable safe error without leaking provider bodies or credentials.
- Connection rotation/revocation affects the next request; no stale cache
  extends authority.
- Logs, errors, metrics, audit, diagnostics, fixtures, bundles, and SDK output
  pass secret and prompt/completion leakage scans.
- Fixed adapter clients reject redirects and never send credentials to a host
  outside the compiled provider allowlist.

An invocation is not complete solely because the provider returned a response:
quota reconciliation and safe audit must also succeed. If their outcome is
ambiguous, return failure and retain conservative usage evidence.

## Verification and release gates

1. Unit tests for request validation, state transitions, redaction, admission,
   reservation/reconciliation, provider mappings, and cancellation.
2. Contract tests for strict JSON, typed errors, capability discovery, SDK/API
   versions, manifest semantics, audit fields, and both provider adapters.
3. Integration tests through a real gateway, SQLite database, app session,
   current policy, fake outbound provider servers, and two-app isolation.
4. Negative security matrix and race tests covering revoke/rotate/budget/
   cancellation concurrency.
5. Failure injection for SQLite busy, encryption-root unavailable, provider
   timeout/malformed response, audit append failure, and process interruption
   around budget reservation.
6. SDK compile, bundle, example, dependency, and package-content checks.
7. Native operator UI accessibility, write-only secret, busy/error/stale state,
   CSRF, exact-target confirmation, and no-secret-render tests.
8. Manual provider smoke tests use dedicated low-value test credentials and
   prove no credential or payload appears in logs.

### Current evidence status

Implemented local evidence includes unit and persistence tests for encrypted
connections, current grants, reservation/reconciliation, audit failure,
concurrency, quota, missing-root denial, and safe control views; Anthropic and
Gemini conformance tests with fixed destinations and redirect denial; a
composed real-listener/two-app gateway test; strict SDK tests; and the
deployable LLM Chat example.

This evidence uses local fake provider transports where provider behavior must
be deterministic. A root-run deployment of this exact build on the disposable
VPS and dedicated low-value Anthropic/Gemini smoke calls have not been recorded.
They remain release evidence, not implied by passing local tests.

## Delivery slices

### L1 — Contract and secret/grant foundation

**Implemented.** The technology-neutral contract, ADR, encrypted connection metadata,
write-only operator flow, app grants, quotas, deny-path tests, and capability
discovery are present.

### L2 — Non-streaming chat completion

**Implemented.** The common service, Anthropic and Gemini adapters, protected route,
`tiny.llm.chat.complete`, example, conformance tests, and targeted failure
injection provide the first deployer/viewer-observable outcome.

### L3 — Streaming chat

**Deferred.** Add bounded streaming events, cancellation, partial-output UX, reservation
reconciliation, and disconnect/failure tests without changing credential or
grant semantics.

## Risks and seams to watch

- **Authorized-viewer spend:** private app access is still a billing surface.
  Use per-viewer and app limits plus atomic worst-case reservation.
- **Provider drift:** provider payloads and usage fields change. Keep mappings
  behind conformance-tested adapters and return `TemporarilyUnavailable` on
  unknown shapes.
- **Prompt injection:** content can manipulate model output but must never alter
  TinyHost authorization, destinations, headers, credentials, or grants.
- **Streaming ambiguity:** disconnects may consume provider tokens without a
  final usage frame. Reserve conservatively and do not promise exact spend.
- **Secret lifecycle:** preserve ADR 0047's implemented write-only rotation,
  root-owned encryption boundary, and no-backup expectation.
- **Single-binary pressure:** keep adapters compiled-in and narrow; do not
  smuggle in a plugin/runtime system.

## Accepted defaults and deferred decisions

1. **Provider support:** Anthropic and Gemini adapters share the L2 conformance
   boundary.
2. **Model selection:** operator-fixed profile; the app may only reduce
   `maxOutputTokens`.
3. **Secret entry:** protected write-only operator UI for normal add/rotate;
   root-only replacement is a recovery path.
4. **Budget unit:** input/output tokens, not currency. Provider pricing is
   mutable and currency caps would imply false precision.
5. **System instructions:** omit from the client contract initially. Add a
   revisioned deployer-visible profile field later if real apps need it.
6. **Streaming:** remains in L3; the implemented L2 types leave it additive.

## Current recommendation

Keep implemented L1/L2 narrow. The exact signed build has passed the
disposable-VPS no-provider matrix; collect only the remaining low-value
real-provider smoke evidence, and defer L3 until streaming usage justifies its
cancellation and accounting complexity. Do not broaden the extension into
tools, generic HTTP, custom adapters, or browser-visible provider choices.
