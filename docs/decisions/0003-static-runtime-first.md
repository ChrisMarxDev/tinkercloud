# ADR 0003: Static runtime first

**Status:** Accepted; blob scope amended by [ADR 0030](0030-v1-lightweight-local-blob-storage.md)

## Context

The core value is structural authorization, not arbitrary compute. Backend
execution adds sandboxing, process lifecycle, egress control, supply-chain,
resource scheduling, secret injection, and host escape concerns.

## Decision

V1 deploys static releases served by TinyHost and offers narrowly scoped
platform capabilities. Current-user, bounded KV, lightweight local blobs, and
ephemeral realtime ship in V1; durable realtime and user-controlled server
processes do not. Blob scope is restricted by ADR 0030 to a bounded,
gateway-authorized, app-scoped capability backed by TinyHost's private local
data directory; it does not introduce app code execution or a public file
server.

## Consequences

- The gateway and release pipeline can be proven with a smaller attack surface.
- AI-generated client apps still support useful data workflows.
- Apps requiring private secrets, arbitrary integrations, or server computation
  remain unsupported.
- Remote object stores, mounted bucket filesystems, and standalone storage
  services remain outside V1.
- Backend runtime work requires a separate concept, threat model, and ADR.
