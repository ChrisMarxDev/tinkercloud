# ADR 0013: Confirmed app deletion permanently removes an app playground

**Status:** Accepted for V1

## Context

Tinkercloud is presently a playground for replaceable applications. Deployers need
confirmed deletion to remove an app rather than leaving a tombstone, retained
release bytes, or a restore-like record. The operation still must not let a
client choose a filesystem path or leave sessions, tokens, or live connections
usable while removal is under way.

## Decision

Expose a scoped, owner-derived control endpoint that requires an idempotency
key plus an exact app-bound confirmation value. The service derives all
database keys and storage paths from the owned app. It first makes the app fail
closed and closes matching live connections, then permanently removes the
application row and every app-owned policy, deployment/release, token, viewer
session, KV, blob, and app-specific audit record, along with the corresponding
server-derived release/blob bytes. A transient `deleting` state may exist only
while work is in progress; successful deletion leaves no `deleted` tombstone
or restoration path.

## Consequences

- The gateway immediately fails closed while deletion is in progress and the
  app no longer resolves after completion.
- A retry may resume only a server-derived in-progress deletion. Once deletion
  succeeds, the former slug is indistinguishable from an unknown app.
- The interface does not accept an app ID or a filesystem path from clients.
- Completion is not reported until the app's owned data and server-derived
  storage have been removed. A failed cleanup cannot be represented as a
  successful deletion.
