# ADR 0034: Update denial health uses a protected capability route

**Status:** Accepted for V1

## Context

Anonymous app document navigations may legitimately redirect through the
platform identity broker. The update health gate previously probed `/`, so a
correct `303` document-navigation redirect looked like a failed anonymous
gateway denial and unnecessarily rolled back the candidate.

## Decision

The updater probes `GET /_tiny/api/v1/capabilities` on the active app host.
That route is a stable, gateway-protected API endpoint and must return the
existing exact `401 not_authorized` JSON evidence for an anonymous request.
The updater derives the app host from the locally verified active slug and the
configured app suffix; it accepts no caller-controlled probe host or path.

## Consequences

The health gate remains evidence that the composed gateway resolves the active
private app and denies before capability dispatch. Redirects, 404s, arbitrary
401s, malformed evidence, and public responses remain unhealthy. Browser
document-navigation behavior is not part of update health evidence.
