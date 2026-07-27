# ADR 0013: Confirmed app deletion is a revocation-first lifecycle transition

**Status:** Accepted for V1

## Context

Deployers need to remove apps they own without turning a request into an
unbounded filesystem deletion or leaving tokens, viewer sessions, and live
connections usable. Release directories are immutable evidence and V1 cleanup
has retention and disk-watermark responsibilities.

## Decision

Expose a scoped, owner-derived control endpoint that requires an idempotency
key plus an exact app-bound confirmation value. In one SQLite transaction it
transitions `active` or `suspended` through `deleting` to `deleted`, revokes
app-scoped API tokens and viewer sessions, and records the deletion request.
The in-memory live hub closes matching connections only after that transaction
commits. Release bytes are not deleted synchronously; cleanup owns that later.

## Consequences

- The gateway immediately fails closed because only active apps resolve.
- Retried delete requests are safe only when the actor, idempotency key, and
  slug match the original audit record.
- The interface does not accept an app ID or a filesystem path from clients.
- Disk reclamation is eventual and observable through the existing cleanup
  workflow rather than being claimed by the delete response.
