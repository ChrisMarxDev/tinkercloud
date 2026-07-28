# ADR 0001: Modular Go monolith

**Status:** Accepted for V1

## Context

TinyHost targets one small Linux VPS, one operator, a single public gateway, and
minimal operational dependencies. Splitting security-critical request handling
across services would add private-network authentication, deployment
orchestration, and distributed failure modes before they provide user value.

## Decision

Build the server as one Go executable with explicit internal package boundaries.
Use SQLite in WAL mode for metadata/app KV and a private filesystem data
directory for immutable releases.

Suggested baseline:

- Go standard `net/http`, `html/template`, `embed`, `crypto`, and reverse-proxy
  packages where applicable;
- `modernc.org/sqlite` initially to preserve CGO-free cross-compilation, subject
  to benchmark and security review;
- direct Resend HTTP adapter behind an internal email interface;
- small CLI framework only if the standard flag package materially harms UX;
- embedded SQL migrations with forward-only production execution;
- server-rendered UI with minimal local JavaScript.

## Consequences

- Simple installation, update recovery, local development, and request tracing.
- Authorization context can remain a typed in-process boundary.
- Heavy work needs bounded queues so it cannot starve gateway requests.
- SQLite write behavior, migration recovery, and filesystem/database
  coordination need explicit tests.
- “Single executable” does not mean one undifferentiated package.

## Rejected V1 alternative: SlateDB over S3-compatible storage

Using an S3-compatible store for blobs and
[SlateDB](https://slatedb.io/) for app KV is technically coherent for a future
stateless or multi-reader platform, but it is rejected for V1:

- TinyHost would still need SQLite for identities, policies, sessions,
  deployments, audit, jobs, and relational integrity, creating two persistence
  authorities and cross-store recovery work.
- SlateDB's official Go binding requires cgo and a separately loaded Rust
  library, conflicting with the self-contained CGO-free server artifact.
- Durable writes depend on remote object-store latency and availability, while
  compaction, garbage collection, caching, bucket credentials, and provider
  recovery expand the operational model.
- Self-hosting the S3-compatible store adds another service; using a managed
  store adds an external recovery authority and provider secret.

V1 therefore keeps app KV in SQLite and blob bytes in TinyHost's native private
local adapter. This alternative is reconsidered only if measured needs justify
stateless compute or multiple readers and the runtime, object-store conditional
write compatibility, outage behavior, quotas, recovery, and migration path all
pass a separate architecture spike.

## Reconsider when

One node can no longer meet measured load/recovery needs, or an independently
isolatable component provides more security than coordination risk.
