# ADR 0047: Reactive operator-supplied encrypted LLM chat

Status: Accepted for post-V1 L1/L2 implementation

## Context

Tinkercloud needs a narrow way for an authorized private app to use an external
LLM without giving a deployer or viewer an organization API key. This changes
the secret, persistence, spending, and external-network trust boundaries.

## Decision

Tinkercloud stores each provider key only as an authenticated encrypted envelope in
the control SQLite database. The encryption root is injected from the existing
root-owned service credential boundary, is not persisted in SQLite, and has no
public read endpoint. Connection entry and rotation are write-only. The LLM
repository never exposes a credential through a list, discovery, audit, or
diagnostic model; plaintext exists only while an internal adapter makes an
approved request.

Every active app receives LLM chat reactively when the host has a usable default
profile. There is no manifest request, deployment gate, or per-app approval.
Profiles fix the provider and model and carry mandatory message, timeout, rate,
and concurrency bounds. The first active profile becomes the host default; an
operator can atomically select another active profile, and the next invocation
uses it without fallback. The service receives the sealed app authorization
context, derives tenancy from it, atomically reserves usage and a concurrency
slot, invokes only a compiled-in Anthropic or Gemini adapter, then reconciles
safe token metadata and audit evidence transactionally.

Financial limiting is optional. The host has an immediate global enable/disable
switch and may define a monthly per-app token
allowance, while each app can inherit it, be unlimited, or receive a specific
allowance. Absence of quota means unlimited use, not absent metering. Usage is
always counted in UTC calendar months. A separate per-app emergency disable
takes effect on the next invocation and does not depend on quota state.

The operator dashboard separates the API-key lifecycle from LLM chat
configuration. Its fixed-provider create control asks only for Anthropic or
Gemini and a write-only key; the trusted service derives the opaque connection
ID and deterministic safe label. Existing labels remain historical metadata.
Profiles, default selection, optional quota, app policy, limits, and usage stay in the LLM chat section. If the
root-owned credential boundary is unavailable, the API-key section shows only
the root-only enable-and-restart next step and exposes neither a mutation form
nor root configuration detail.

The LLM chat section may build an operator-only live model catalog by
decrypting each active connection key, calling only that adapter's fixed
official model-list endpoint, and clearing the plaintext after the lookup. A
connection contributes options only when that authenticated lookup returns a
bounded valid response; failures are omitted without exposing detail or making
the whole dashboard unavailable. Catalog metadata never enters app capability
discovery. It is advisory rather than an authorization source: profile writes
and invocations still re-check active persisted state.

The operator may instead enter a bounded custom model identifier against an
active connection. This avoids coupling support for a newly released model to
a Tinkercloud binary update while preserving the fixed provider adapter,
destination, limits, app policy, and quota boundaries. Tinkercloud does not infer or
automatically migrate existing profile models when provider catalogs change.

The initial host setup generates the capability root in the root-owned service
credential file. An existing host enables the same boundary with root-only
`tinkercloud llm enable`; it generates the root locally, records only its
environment reference in config, prints no value, and requires a service
restart before LLM controls appear. LLM configuration never blocks deployment
or activation. Calls simply remain unavailable until the current default
profile and connection can supply them.

Production adapters have fixed official HTTPS destinations and reject
redirects. They are not a generic HTTP proxy. Local test endpoints and
transports are injection seams used only by adapter conformance tests.

## Consequences

The operator can disable a connection, change the default, or disable an app
and the next request reflects that state. Browser
discovery exposes only active safe limits and a fixed external-content notice,
never the selected provider or model. A provider or persistence ambiguity can conservatively retain reserved
tokens, reducing availability instead of undercounting spend. Prompts and
completions are intentionally absent from persistence and audit, so Tinkercloud
does not offer provider replay or server-owned conversation history.

This clean pre-live replacement removes the earlier manifest/grant model; no
compatibility or data migration is required. It preserves the operator-only
provider connections, live model catalog, custom model identifiers, and fixed
adapters introduced with the model-catalog work.

This is deliberately post-V1: it adds an external provider dependency and
operator secret lifecycle without adding a listener, backend runtime, remote
credential authority, arbitrary URL, or browser-visible secret.
