# ADR 0035: Bounded local listener readiness before update health gates

**Status:** Accepted for V1

## Context

`systemctl restart` for the service can return before the replacement process
has bound its configured HTTP and HTTPS listeners. Running doctor immediately
can therefore observe a transient closed local port and restore a healthy
candidate before its public health checks begin.

## Decision

After restart, the updater first performs a bounded, context-cancellable local
TCP listener-readiness check for exactly the configured HTTP and HTTPS
addresses. It retries only failed connection establishment with capped
exponential backoff for at most ten seconds. The candidate doctor, platform
public-health, and strict anonymous-denial predicates remain ordered one-shot
gates after readiness succeeds.

## Consequences

The updater tolerates only the service-manager/listener startup race. Database,
credential, DNS, TLS, provider, public gateway, and authorization failures are
not retried or reclassified, and still restore the known-good binary.
