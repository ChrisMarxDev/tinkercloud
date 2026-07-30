# 0017 — Deployer-side public denial proof

Status: Accepted for V1

## Context

Candidate-aware activation verifies policy, certificate readiness, and a
server-side anonymous probe before atomically installing a release. That is
necessary, but a `tinker deploy` success message also promises that the deployer
can reach the public DNS/TLS gateway path a viewer will use. Trusting only
boolean activation fields would let an in-process or incorrectly composed
control path appear verified without exercising that public route.

One root `domain` is configured: for example, `example.com`. The dashboard is
the reserved `admin.example.com` host and every app is one label beneath the
same root. The client therefore accepts the server's explicit domain rather
than deriving it from its control client URL.

## Decision

The authenticated activation result carries a server-derived `domain` with the
protected app URL. Before `tinker deploy` returns success, the client requires
that URL to be exactly `https://{slug}.{domain}/` and makes a new
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
- This does not replace candidate-aware atomic activation or failed-activation
  preservation; it is a client-side success gate after that server transition.
