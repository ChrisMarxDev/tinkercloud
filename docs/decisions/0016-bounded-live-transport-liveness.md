# ADR 0016: Bounded live transport liveness

**Status:** Accepted for V1

## Context

The V1 live hub is intentionally in-memory and app-scoped, but an authenticated
WebSocket can still retain a server reader, writer, and bounded outbound queue
if its peer becomes silent. Session, policy, and app revocation must also take
effect promptly rather than waiting for an arbitrary network timeout.

## Decision

Each authenticated live transport has configured, bounded defaults: a
60-second idle lifetime, a ping every 20 seconds, a 10-second pong deadline,
and a 3-second outbound write deadline. A successful pong resets liveness. A
silent peer, a full outbound queue, or an expired write/pong deadline closes
that transport. The server configuration can tighten these values only within
validated V1 bounds.

The hub continues to own revocation. Its matching connection close directly
closes the transport and releases the socket; it does not wait for heartbeat
expiry. The gateway still authenticates and authorizes before upgrade, and the
hub remains app/session scoped.

## Consequences

- Idle live sockets have a finite resource lifetime.
- A slow or dead viewer does not block healthy subscribers.
- Revocation stays immediate on this node.
- Ping/pong is liveness only; it does not add history, replay, ordering,
  delivery, or durability semantics.
