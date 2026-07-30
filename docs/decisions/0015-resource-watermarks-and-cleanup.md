# ADR 0015: Resource watermarks gate growth but not revocation

**Status:** Accepted for V1

## Context

A single-VPS installation can run out of disk through archives, release trees,
or app KV. A generic write lock would also prevent the operator or a deployer
from revoking access during incident response, while filesystem-only cleanup
could delete a release that SQLite still identifies as active.

## Decision

Tinkercloud derives a disk write gate from a fail-closed disk source and bounded
configuration. At the critical watermark it rejects only resource-growth paths:
app creation, deployment creation, and KV mutation. Static reads and all
revocation/suspension operations bypass the gate.

Cleanup reads deployment metadata and the current deployment pointer from
SQLite. It groups content-addressed hashes per app, retains the active hash and
the configured number of inactive hashes (at least one), then removes only a
server-derived path below `data/releases`. It does not delete database history;
filesystem failures remain retryable.

## Consequences

- A failed disk-stat call denies growth rather than assuming free space.
- Releasing disk does not require a public or app-facing administrative route.
- Content-equivalent deployments share a cleanup decision and cannot cause an
  active hash to be reclaimed through a duplicate row.
- The next server composition must wire the shared gate into deployment,
  control-app creation, and persistent KV adapters.
