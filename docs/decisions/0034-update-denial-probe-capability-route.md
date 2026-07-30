# ADR 0034: Update denial health uses a protected capability route

**Status:** Accepted for V1

## Context

Anonymous app document navigations may legitimately redirect through the
platform identity broker. The update health gate previously probed `/`, so a
correct `303` document-navigation redirect looked like a failed anonymous
gateway denial and unnecessarily rolled back the candidate.

## Decision

When at least one active app exists, the updater deterministically selects the
lexicographically first locally verified active app from installed server
state. It probes `GET /_tiny/api/v1/capabilities` on that app host. The route is
a stable, gateway-protected API endpoint and must return the existing exact
`401 not_authorized` JSON evidence for an anonymous request. The updater derives
the app host from the selected active slug and configured root domain; it
accepts no caller-controlled app slug, probe host, or path.

When no active app exists, there are no app release bytes or app capabilities
to expose. The updater proves that exact installed-state fact, platform health,
socket confinement, and safe unknown-app-host denial. The first deployment
remains responsible for the full active-app capability denial gate before it
can succeed.

## Consequences

The health gate remains evidence that the composed gateway resolves the active
private app and denies before capability dispatch. Redirects, 404s, arbitrary
401s, malformed evidence, and public responses remain unhealthy. Browser
document-navigation behavior is not part of update health evidence.

The operator no longer supplies an app slug that the server can select safely.
A failure to read or deterministically classify installed active-app state is
unhealthy; it is never treated as the no-app case.
