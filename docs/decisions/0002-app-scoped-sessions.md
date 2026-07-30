# ADR 0002: App-scoped opaque sessions

**Status:** Accepted for app-session isolation; browser-identity topology
superseded by ADR 0051

## Context

A shared parent-domain session reduces login prompts but expands cookie scope
across untrusted app origins. JWT-only sessions also complicate immediate
revocation and current-policy enforcement.

## Decision

Use a host-only opaque session for each app hostname. Store only a hash of the
random session secret in SQLite. Bind each record to one app and identity.
Evaluate current app policy on every protected request. ADR 0051 retains this
app-local boundary while adding the exact `admin.<domain>` global browser
identity and one-time app-bound exchange; it does not add a parent-domain app
cookie.

The admin dashboard authenticates from that same global browser identity but
does a separate current-role lookup. CLI/agent tokens are never browser
sessions.

## Consequences

- Strong cross-app isolation and straightforward revocation.
- Users authenticate once per browser profile; an allowed additional app gets a
  policy-checked short-lived, single-use exchange without broadening the app
  cookie.
- Cookie naming must account for the fact that the `__Host-` prefix requires
  `Secure`, `Path=/`, and no `Domain`; the exact name is an implementation
  contract to test.
