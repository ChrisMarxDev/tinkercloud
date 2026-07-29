# 0038: Bounded pre-activation certificate readiness retry

**Status:** Accepted for V1

## Context

The first deployment of a new app host can stage successfully before ACME has
finished issuing its certificate. A single readiness GET caused a valid first
candidate to fail, while an immediate retry by the deployer often succeeded.

## Decision

The server retries only the pre-activation certificate-readiness proof within
one context-cancellable 45-second budget. It has eight finite attempts, each
bounded to five seconds, with finite backoff between attempts. Every attempt
uses the exact `https://{app-host}/_tiny/auth/login` origin, follows no
redirect, requires a verified TLS chain, and rejects redirects and 5xx status.
The HTTP client is copied for the probe so redirect policy is not mutated on a
shared client.

## Consequences

First certificate issuance gets a small, predictable opportunity to complete.
Cancellation, budget exhaustion, wrong-host, redirect, unverified-TLS, and all
other non-ready results still prevent activation and preserve the last active
release. The retry is deliberately not a generic deployment retry: it does not
repeat policy, ownership, archive, anonymous-denial, or post-activation checks.
