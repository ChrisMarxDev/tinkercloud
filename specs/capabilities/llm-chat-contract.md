# Operator-governed LLM Chat Capability Contract

## Authority and ownership

`llm.chat` is a protected TinyHost capability. Every invocation receives the
sealed gateway `AuthorizationContext`; the app ID and viewer identity come only
from that context. Request bodies, URLs, headers, SDK options, and provider
responses cannot select an app, viewer, provider, connection, model, endpoint,
header, or credential.

An operator owns provider credentials, profiles, grants, limits, and usage
evidence. A deployer owns app code and may request only the logical capability.
A viewer sends content through an already-authorized app and can spend only that
app's active operator-approved grant. App content is sent to the selected
external provider; TinyHost does not promise that content remains local.

## Version 1 operation

The first operation is non-streaming completion:

```ts
type ChatMessage = { role: "user" | "assistant"; content: string };
type ChatRequest = { messages: ChatMessage[]; maxOutputTokens?: number };
type ChatResponse = {
  message: { role: "assistant"; content: string };
  usage: { inputTokens: number; outputTokens: number };
  finishReason: "stop" | "length";
  requestId: string;
};
```

Messages are non-empty valid UTF-8, alternate roles, and end in `user`.
`maxOutputTokens` can only reduce the active profile limit. All history and
size limits are operator-selected, server-enforced bounds. Streaming, tools,
images, embeddings, files, raw provider options, and conversation persistence
are outside this version.

## Secret and destination boundary

Connections are write-only. SQLite holds an authenticated encrypted envelope
and safe connection metadata; a root-owned TinyHost credential supplies the
envelope root and never enters SQLite, APIs, logs, audit, diagnostics, the SDK,
or deployed files. The only compiled destinations are Anthropic Messages and
Gemini generateContent. Production clients reject redirects. Test adapters may
receive an injected local endpoint and transport solely for conformance tests.

No public interface returns a provider credential, encrypted envelope,
connection identifier, provider URL, or provider error body. Capability
discovery is absent unless both the manifest requests `llm.chat` and a current
approved grant is active. When present, it may disclose only the logical
version, safe effective numeric limits, and a fixed content-disclosure text.

## Admission, quota, and completion

Before durable quota reservation, the service reads the current non-secret
effective limits and applies bounded in-memory app/viewer rate admission. The
repository then atomically re-verifies the active app, approved grant, active
profile and connection before reserving a conservative token maximum and a
profile concurrency slot. A changed or revoked binding fails closed rather
than using the pre-admission snapshot. A revoked grant is terminal: no normal
control update or stale writer can reactivate it. Disabled, revoked, suspended,
exhausted, malformed, or unavailable state denies before decrypting a key or
contacting a provider.

Successful responses reconcile provider token metadata and append safe audit
metadata in the same durable transaction. A known pre-provider failure releases
the reservation. Cancellation or malformed/ambiguous provider outcome releases
the concurrency slot but retains the reservation conservatively. An audit or
reconciliation failure is a failed request, not a successful completion.

Audit records may include server IDs, profile ID, outcome, and token counts.
They never include prompt text, completion text, headers, URLs, provider bodies,
or credentials.

## Stable errors

The service exposes only these classifications: `unauthorized`,
`capability_unavailable`, `invalid_request`, `rate_limited`, `quota_exhausted`,
`temporarily_unavailable`, and `cancelled`. Implementations must not map a
provider body, TLS failure, decrypt failure, SQL detail, or grant state to a
browser-visible message.

## Deny-path evidence

Tests must prove anonymous/malformed authorization, cross-app scope, disabled
or revoked grant/connection, invalid input, exhausted quota/rate/concurrency,
provider failure, cancellation, audit failure, malformed provider response,
redirects, and secret/prompt leakage all fail closed before a usable response.
