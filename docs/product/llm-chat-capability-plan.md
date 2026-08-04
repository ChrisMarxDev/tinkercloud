# Reactive LLM chat capability plan

**Status:** Post-V1 L1/L2 implemented; L3 streaming deferred

**Date:** 2026-08-04
**Roadmap position:** Post-V1 extension; live provider smoke remains pending.

## Observable outcome

An authenticated viewer can use the existing provider-neutral SDK operation in
any active app:

```ts
const reply = await tinker.llm.chat.complete({
  messages: [{ role: "user", content: "Summarize this review." }],
});
```

The call works when the host has a usable default profile and the app is not
disabled. Otherwise discovery omits the capability and calls return the stable
`capability_unavailable` classification. A deployer does not request access in
the manifest and an operator does not approve a grant.

## Included slice

- Write-only operator-managed Anthropic and Gemini connections.
- Operator-only live model catalog plus bounded custom model identifiers.
- Mandatory profile request, output, timeout, rate, and concurrency limits.
- First active profile automatically selected as the host default.
- Atomic “use as default” selection with no fallback profile.
- Host-wide immediate enable/disable plus an optional monthly per-app allowance;
  enabled and unlimited by default.
- Per-app inherit, unlimited, or specific allowance plus emergency disable.
- Always-on UTC-month usage accounting without prompt/completion persistence.
- Reactive discovery and invocation through sealed gateway authorization.
- Existing non-streaming SDK syntax, stable errors, cancellation, and audit.

Streaming, tools, images, embeddings, files, raw provider options, generic
proxying, provider retries, and server-owned conversation history remain out of
scope.

## Trust and ownership

| Actor | Owns / may do | Must not do |
|---|---|---|
| Operator | Keys, profiles, default, host quota, app overrides and disable | Expose keys or provider internals |
| Deployer | App code and its own app data | Select provider/model/key or request a grant |
| Viewer | Submit bounded content through an authenticated app | Supply app/viewer identity or cross app budgets |
| Gateway | Derive app, viewer, session, and typed authorization | Trust browser tenancy input |
| LLM service | Resolve current policy, reserve usage, invoke fixed adapter | Log content or accept arbitrary destinations |

The external provider receives submitted message content. Tinkercloud stores
only encrypted keys, non-secret configuration, usage counters, reservations,
and safe audit metadata.

## Reactive flow

```text
authenticated SDK request
→ gateway derives active app and viewer
→ LLM repository resolves current default profile and active connection
→ app disable and effective quota are checked
→ technical rate, size, timeout, and concurrency bounds are enforced
→ worst-case usage is atomically reserved
→ fixed Anthropic or Gemini adapter runs
→ actual usage and safe audit evidence are reconciled
→ common response or stable error returns
```

Deployment and activation do not consult LLM state. Changing the default,
quota, connection, or app policy affects the next discovery or completion.

## Persistence

- `provider_connections`: encrypted provider credentials and safe metadata.
- `llm_chat_profiles`: fixed model and mandatory technical limits, with one
  current default selected by the operator.
- `llm_host_policy`: optional monthly allowance inherited by each app.
- `app_llm_policies`: per-app disable and inherit/unlimited/specific override.
- `llm_usage`: per-app UTC-month used/reserved/in-flight counters.
- `llm_reservations`: per-call profile and safe accounting state.

This is a clean pre-live schema replacement. The former manifest request,
deployment gate, app-grant table, and profile-owned financial quota are removed;
no migration or compatibility path is retained.

## Failure and deny evidence

Tests cover missing/default changes, inactive connections, disabled apps,
exhausted optional quota, unlimited metering, stale policy writes, cross-app
scope, malformed authorization/input, rate/concurrency exhaustion, cancellation,
provider ambiguity, audit failure, redirects, and secret/content leakage. An
unavailable default denies and never falls back to another profile.
