# ADR 0009: V1 KV and ephemeral realtime

**Status:** Accepted for V1

## Context

TinyHost should make small staff apps feel alive without turning the first
release into a database or distributed messaging platform.

## Decision

V1 exposes an app-scoped JSON key-value store with get, set, delete, bounded
prefix listing, optimistic versions, and quotas. It also exposes an
authenticated WebSocket hub for bounded custom app channels and KV change
events.

The hub is single-node and in memory. Connections are bound to server-derived
app, identity, and session context. Revocation closes affected connections.
Delivery is best-effort with no history, replay, ordering guarantee, or resume
cursor. KV is authoritative; reconnecting clients reread current state.

## Consequences

- Realtime is useful in the first product without becoming durable state.
- App isolation and revocation must be tested across HTTP and WebSocket paths.
- Connections, subscriptions, rates, frame sizes, and slow consumers need hard
  limits.
- Durable replay or multi-node fan-out requires a later ADR.
