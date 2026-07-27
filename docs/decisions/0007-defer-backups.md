# ADR 0007: Defer operator backups beyond V1

**Status:** Accepted

## Context

Initial TinyHost deployments host replaceable toy and utility apps. A complete
backup product requires retention policy, encryption, remote destinations,
credential management, consistency, compatibility, and restore drills. That
work does not prove the private deployment promise.

## Decision

Do not ship operator-managed backup or disaster recovery in V1. Preserve a
cohesive data directory and immutable release manifests so the feature can be
added later without changing app identity.

Signed self-update may create a bounded local rollback snapshot strictly for
returning from a failed update. It is not user-addressable, remote, retained as
a backup, or presented as disaster recovery.

## Consequences

- V1 is smaller and reaches useful private apps sooner.
- Loss of the VPS can lose app state and releases; setup documentation must say
  this plainly.
- Apps needing durable business-critical data are outside the initial product
  promise.
- Backup becomes a ranked advanced feature once usage justifies it.
