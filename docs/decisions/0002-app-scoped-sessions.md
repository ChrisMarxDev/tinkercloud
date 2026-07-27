# ADR 0002: App-scoped opaque sessions

**Status:** Accepted for V1

## Context

A shared parent-domain session reduces login prompts but expands cookie scope
across untrusted app origins. JWT-only sessions also complicate immediate
revocation and current-policy enforcement.

## Decision

Use a host-only opaque session for each app hostname. Store only a hash of the
random session secret in SQLite. Bind each record to one app and identity.
Evaluate current app policy on every protected request.

The platform/control plane uses a separate cookie and session type. CLI/agent
tokens are never viewer sessions.

## Consequences

- Strong cross-app isolation and straightforward revocation.
- Users may authenticate once per app.
- Central SSO can later issue a short-lived, single-use app exchange without
  broadening the app cookie.
- Cookie naming must account for the fact that the `__Host-` prefix requires
  `Secure`, `Path=/`, and no `Domain`; the exact name is an implementation
  contract to test.
