# ADR 0003: Static runtime first

**Status:** Accepted

## Context

The core value is structural authorization, not arbitrary compute. Backend
execution adds sandboxing, process lifecycle, egress control, supply-chain,
resource scheduling, secret injection, and host escape concerns.

## Decision

V1 deploys static releases served by TinyHost and offers narrowly scoped
platform capabilities. Current-user, bounded KV, and ephemeral realtime ship in
V1; blob storage and durable realtime do not. No user-controlled server process
starts in V1.

## Consequences

- The gateway and release pipeline can be proven with a smaller attack surface.
- AI-generated client apps still support useful data workflows.
- Apps requiring private secrets, arbitrary integrations, or server computation
  remain unsupported.
- Backend runtime work requires a separate concept, threat model, and ADR.
