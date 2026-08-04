# Operator-governed LLM Chat Capability Contract

## Authority and ownership

`llm.chat` is a protected Tinkercloud capability. Every invocation receives the
sealed gateway `AuthorizationContext`; the app ID and viewer identity come only
from that context. Request bodies, URLs, headers, SDK options, and provider
responses cannot select an app, viewer, provider, connection, model, endpoint,
header, or credential.

An operator owns provider credentials, profiles, grants, limits, and usage
evidence. A deployer owns app code and may request only the logical capability.
A viewer sends content through an already-authorized app and can spend only that
app's active operator-approved grant. App content is sent to the selected
external provider; Tinkercloud does not promise that content remains local.

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
and safe connection metadata; a root-owned Tinkercloud credential supplies the
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

## Operator API-key control surface

The admin-host dashboard has one operator-only **API keys** section, separate
from **LLM chat** profiles, grants, limits, and usage. The initial key types are
the fixed Anthropic and Gemini providers only. The create form accepts exactly a
provider selection and one bounded write-only password API-key field. It has no
connection ID, display-name, arbitrary secret name, provider URL, or generic
secret field. The trusted service validates the fixed provider, generates the
opaque connection ID, and derives the display label exactly as `Anthropic API
key` or `Gemini API key` before persistence. Existing connection names remain
unchanged and multiple connections for one provider remain valid.

The operator-only LLM chat section may offer a live model catalog as an
advisory profile-creation aid. A catalog option exists only when its connection
is active, its encrypted key decrypts through the root-owned credential
boundary, and the fixed provider's official model-list endpoint succeeds with
a bounded valid response. Gemini entries are limited to models that advertise
`generateContent`. The server omits a connection's entries on decrypt,
transport, timeout, provider, or response-validation failure; one failure does
not make the dashboard unavailable and no stale or inferred model entry is
substituted.

Catalog entries contain only the safe connection ID and label, fixed provider,
and provider model identifier needed by the operator form. They never enter
app-facing capability discovery, the SDK, a deployed app, or a viewer response.
The catalog is not an authorization decision: profile persistence still
requires a currently active connection, and invocation still re-verifies the
active profile, grant, and connection.

Every profile form also provides an explicit custom model-identifier path.
This path accepts the same bounded single-line string as persisted profiles and
lets an operator select a newly released provider model without upgrading
Tinkercloud. It cannot select a provider URL, add request options, bypass the
fixed adapter, or use an inactive or missing connection. Tinkercloud never
changes an existing profile's model automatically when a provider catalog
changes.

The server alone derives whether key management is ready. If its LLM repository,
envelope root, or credential validator is unavailable, the dashboard shows an
unavailable state with the root-only `tinkercloud llm enable` and restart next
step. It renders no create, rotate, or disable form in that state and never
reveals the root, its environment reference, envelope, or reason-specific
configuration detail. Rotation derives the fixed provider from the stored
connection; a browser-supplied provider, display name, or ID is denied.

These controls are operator-only and require the existing host-only control
session, same-origin POST, CSRF, bounded form parsing, and typed service role
check. Safe configured metadata may show the pre-existing label, fixed provider,
opaque server-generated ID, and durable status. API keys, envelopes, values,
provider responses, and raw validation failures never render in HTML, notices,
audit entries, or errors. Failed validation or persistence leaves any existing
connection unchanged.

## Deny-path evidence

Tests must prove anonymous/malformed authorization, cross-app scope, disabled
or revoked grant/connection, invalid input, exhausted quota/rate/concurrency,
provider failure, cancellation, audit failure, malformed provider response,
redirects, and secret/prompt leakage all fail closed before a usable response.
Catalog tests must additionally prove that missing, disabled, undecryptable, or
provider-rejected keys contribute no options; malformed and oversized provider
catalogs contribute no options; deployers, viewers, apps, and SDK discovery see
no catalog metadata; malformed catalog selections are denied; and a bounded
custom identifier still requires an active connection.
