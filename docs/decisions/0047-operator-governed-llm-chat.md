# ADR 0047: Operator-governed encrypted LLM chat capability

Status: Accepted for post-V1 L1/L2 implementation

## Context

TinyHost needs a narrow way for an authorized private app to use an external
LLM without giving a deployer or viewer an organization API key. This changes
the secret, persistence, spending, and external-network trust boundaries.

## Decision

TinyHost stores each provider key only as an authenticated encrypted envelope in
the control SQLite database. The encryption root is injected from the existing
root-owned service credential boundary, is not persisted in SQLite, and has no
public read endpoint. Connection entry and rotation are write-only. The LLM
repository never exposes a credential through a list, discovery, audit, or
diagnostic model; plaintext exists only while an internal adapter makes an
approved request.

An app receives one operator-selected profile through an explicit `llm.chat`
grant. Profiles fix the provider and model and carry all message, timeout,
rate, concurrency, and monthly-token bounds. The service receives the sealed
app authorization context, derives tenancy from it, atomically reserves usage
and a concurrency slot, invokes only a compiled-in Anthropic or Gemini adapter,
then reconciles safe token metadata and audit evidence transactionally.

The operator dashboard separates the API-key lifecycle from LLM chat
configuration. Its fixed-provider create control asks only for Anthropic or
Gemini and a write-only key; the trusted service derives the opaque connection
ID and deterministic safe label. Existing labels remain historical metadata.
Profiles, grants, limits, and usage stay in the LLM chat section. If the
root-owned credential boundary is unavailable, the API-key section shows only
the root-only enable-and-restart next step and exposes neither a mutation form
nor root configuration detail.

The initial host setup generates the capability root in the root-owned service
credential file. An existing host enables the same boundary with root-only
`tinyhost llm enable`; it generates the root locally, records only its
environment reference in config, prints no value, and requires a service
restart before LLM controls appear. A manifest requesting `llm.chat` cannot
activate without a currently approved active grant/profile/connection, so a
missing root or operator binding preserves the prior active release.

Production adapters have fixed official HTTPS destinations and reject
redirects. They are not a generic HTTP proxy. Local test endpoints and
transports are injection seams used only by adapter conformance tests.

## Consequences

The operator can disable a connection or revoke a grant and the next request
denies; a revoked grant is terminal and must be recreated through an explicit
future lifecycle rather than silently re-approved by a stale page. Browser
discovery exposes only active safe limits and a fixed external-content notice,
never the selected provider or model. A provider or persistence ambiguity can conservatively retain reserved
tokens, reducing availability instead of undercounting spend. Prompts and
completions are intentionally absent from persistence and audit, so TinyHost
does not offer provider replay or server-owned conversation history.

This is deliberately post-V1: it adds an external provider dependency and
operator secret lifecycle without adding a listener, backend runtime, remote
credential authority, arbitrary URL, or browser-visible secret.
