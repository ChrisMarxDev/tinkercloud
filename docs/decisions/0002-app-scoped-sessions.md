# ADR 0002: App-scoped opaque sessions

**Status:** Superseded in part by ADR 0033

## Context

A shared parent-domain session reduces login prompts but expands cookie scope
across untrusted app origins. JWT-only sessions also complicate immediate
revocation and current-policy enforcement.

## Decision

Use a host-only opaque session for each app hostname. Store only a hash of the
random session secret in SQLite. Bind each record to one app and identity.
Evaluate current app policy on every protected request. ADR 0033 retains this
app-local boundary while adding a separate platform-host global viewer identity
and one-time app-bound exchange; it does not add a parent-domain app cookie.

The platform/control plane uses a separate cookie and session type. CLI/agent
tokens are never viewer sessions.

## Consequences

- Strong cross-app isolation and straightforward revocation.
- Users authenticate once per browser profile; an allowed additional app gets a
  policy-checked short-lived, single-use exchange without broadening the app
  cookie.
- Cookie naming must account for the fact that the `__Host-` prefix requires
  `Secure`, `Path=/`, and no `Domain`; the exact name is an implementation
  contract to test.
