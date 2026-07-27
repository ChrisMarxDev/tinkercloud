# 0017 — Deployer-side public denial proof

Status: Accepted for V1

## Context

Candidate-aware activation verifies policy, certificate readiness, and a
server-side anonymous probe before atomically installing a release. That is
necessary, but a `tiny deploy` success message also promises that the deployer
can reach the public DNS/TLS gateway path a viewer will use. Trusting only
boolean activation fields would let an in-process or incorrectly composed
control path appear verified without exercising that public route.

The control-plane hostname and wildcard app suffix are separately configured:
for example, `tiny.example.com` and `*.apps.example.com`. The client therefore
cannot derive the app host from its control server URL.

## Decision

The authenticated activation result carries a server-derived `app_suffix` with
the protected app URL. Before `tiny deploy` returns success, the client
requires that URL to be exactly `https://{slug}.{app_suffix}/` and makes a new
anonymous GET using the selected real HTTP/TLS transport.

That probe uses no bearer token and no cookie jar, follows no redirects, and
accepts only the gateway's bounded `401` JSON `not_authorized` envelope with a
valid request ID matching `X-Request-ID`, `Cache-Control: no-store`, and
`X-Content-Type-Options: nosniff`.

## Consequences

- CLI success covers both server candidate gates and the public protected-route
  denial path.
- A 404, public response, redirect, malformed/oversized evidence, wrong URL,
  timeout, TLS, or transport failure is deployment verification failure.
- The deployer credential remains restricted to the control-plane requests and
  is never delivered to the untrusted app origin.
- This does not replace candidate-aware atomic activation or its rollback
  behavior; it is a client-side success gate after that server transition.
