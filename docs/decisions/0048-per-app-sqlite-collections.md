# ADR 0048: Per-app SQLite databases and bounded document collections

**Status:** Accepted

## Context

Tinkercloud needs a small shared-state primitive beyond key/value storage, without
turning the V1-shaped single-process host into a database service. A shared
SQLite `app_kv` table keeps control-plane and app writes on one writer and does
not physically isolate app data. A separate provider, daemon, or multi-node
database would violate the one-operator operational model before it provides
value.

## Decision

Keep normal embedded SQLite through the existing CGO-free `modernc.org/sqlite`
driver. The control plane stays in `tinkercloud.db`; each server-derived immutable
app ID owns exactly one private local database:

```text
tinkercloud.db
apps/{immutable-app-id}/data.db
```

An in-process manager validates the immutable ID before joining any path,
opens app databases lazily, bounds concurrent handles, and closes idle handles.
Each file runs app-local forward-only schema migration and contains app KV,
documents, collection revisions, and platform metadata.

### Development-stage destructive transition

Tinkercloud has not been released or populated with production app state. Migration
`0007_remove_legacy_control_app_kv.sql` therefore drops the former shared
control-database `app_kv` table instead of attempting a partial data copy. The
KV repository no longer has a control-database fallback: app KV requires an
`AppDatabaseManager` and fails closed when one is absent. This is intentionally
destructive and must not be repurposed as a production migration. A future
released-data transition requires a new ADR with resumable copy, verification,
rollback, and cutover evidence.

Document collections are a narrow JSON-object primitive: server-assigned IDs,
optimistic versions, timestamps, bounded lists/snapshots, quotas, and one
collection revision. A successful SQLite commit yields an explicit mutation
result, after which the existing in-memory, app-scoped live hub may publish a
freshness hint. The hub remains non-durable; reconnecting clients reread state.

## Consequences

- Independent apps no longer contend for one SQLite writer and their data is
  physically isolated into separately removable files.
- The single Tinkercloud process remains the only public/security boundary; no
  client receives an app ID choice, file path, SQL interface, or credential.
- Platform schema migration is bounded to the app first accessed, rather than
  extending startup time with all apps. A migration failure affects that app's
  data capability and leaves unrelated apps/control state available.
- Hard app deletion must close/remove the exact validated app database in the
  same inaccessible deletion lifecycle as releases and blobs.
- There are now multiple SQLite files, but not multiple database systems or
  services. This intentionally refines Principle 8's operational shape while
  preserving one binary, one data directory, one service, and one operator.

## Rejected alternatives

- **Turso/libSQL:** replication, remote topology, CDC, and current engine risk
  do not remove Tinkercloud's authorization, reconciliation, or WebSocket work.
- **bbolt:** small and pure Go, but adds a second persistence API and makes
  bounded JSON collection querying/indexing a platform-owned problem.
- **PostgreSQL/Redis:** add a daemon, credentials, lifecycle, recovery surface,
  and a second operational authority that V1 does not need.

## Reconsider when

Measured needs require multi-process concurrent writers, replicas, offline
synchronization, or durable change consumers. Such a change requires a new
trust/persistence ADR and migration plan.
