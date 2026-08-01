# ADR 0055: Defer dashboard token management pending a dedicated overhaul

## Status

Accepted — 2026-08-01

## Context

The operator dashboard exposed a per-app token panel with a free-form scope
field, lifetime in seconds, display-once creation, token IDs, and revoke
buttons. Its presentation and authorization model did not agree: the panel was
operator-only, token mutations required the current actor to own the app, and
operators could see apps owned by deployers. The form also accepted scopes
that were not all useful for an app-bound credential and the inventory omitted
expiry, last-used, and revoked state that an actor needs to make a safe decision.

Tinkercloud already has a scoped control API, deterministic Tinker CLI token
workflow, display-once secret response, hash-at-rest storage, exact-scope and
app-binding checks, expiry, immediate revocation, safe last-used metadata, and
audited mutations. Removing those mechanisms would break deployment-agent
credentialing; leaving the partial dashboard surface would make authority
harder to understand.

## Decision

Hide dashboard token management completely until a dedicated product and
security overhaul is accepted. Operator and deployer dashboards render no token
count, inventory, scope or lifetime input, create action, revoke action, or
hidden/disabled token-management form.

Keep the existing scoped control API, Tinker CLI workflow, and display-once
token result. Their authorization, ownership, app binding, expiry, revocation,
safe metadata, and audit behavior do not change.

This decision is reflected canonically by PRD requirements `FR-UI-007` and
locked decision `D11`.

Before token management returns to the dashboard, a replacement design must
define:

- which actor manages credentials for a deployer-owned app;
- task-oriented least-privilege scope presets and whether every admitted scope
  is meaningful for an app-bound token;
- safe lifetime defaults and limits;
- complete active, expired, revoked, and last-used inventory state;
- display-once secret handling and its handoff to automation; and
- exact revocation targets, confirmation, and post-revocation feedback.

The reintroduction must update the UI contract, design guidance, showcase,
tests, and native-UI skill together.

## Consequences

- The dashboard no longer presents a control that can fail because the visible
  operator is not the app owner.
- Operators and deployers continue to use the scoped Tinker CLI or control API
  for token management.
- The server may continue to build a credential-safe token read model, but the
  dashboard template must not serialize it.
- Direct token creation still returns the secret exactly once; later dashboard
  requests disclose neither that secret nor token metadata.
- This changes only the browser interface. It does not change the gateway,
  bearer format, token persistence, permissions, or revocation semantics.
