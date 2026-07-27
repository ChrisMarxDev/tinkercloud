# 0022 — App-host document login and account switching

Status: Accepted for V1

## Context

Private app routes correctly return the gateway's JSON denial to anonymous
requests, but presenting that response for a normal browser navigation is an
unhelpful sign-in experience. The improvement must not turn API, asset, range,
or WebSocket requests into redirects, nor must it substitute platform login for
the app-scoped viewer identity.

## Decision

Only a GET protected-static request with browser document-navigation metadata
and an HTML accept value redirects to the same app host's login route with a
validated relative return URI. All other denied protected routes retain the
JSON 401 gateway denial.

Account switch is an app-host form POST to the existing logout route. It
requires an exact HTTPS Origin matching the current host, validates the return
path, revokes the current app session, expires that host-only cookie, and sends
the browser back to that app's login route. GET logout is never a mutation.

## Consequences

- Browser navigation is usable without making private app content public.
- The platform cookie remains irrelevant to application authorization.
- A forced wrong-app cookie cannot revoke a session belonging to another app.
- Clients without unambiguous document metadata continue to receive the stable
  programmatic denial response.
