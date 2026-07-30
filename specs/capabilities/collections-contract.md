# Reactive collections contract

This contract defines the bounded, app-scoped JSON document primitive. It
extends the existing private gateway capability model; it does not expose SQL,
database credentials, app-selected paths, joins, arbitrary filters, or a
second listener.

## Scope and ownership

- The gateway derives the immutable app ID from the validated host and passes
  it in a sealed `AuthorizationContext`. Browser input never selects an app
  database, collection namespace, or document storage path.
- Tinkercloud stores each app's data at `apps/{immutable-app-id}/data.db` below
  the private data root. The control database remains separate and owns apps,
  policy, sessions, deployments, audit, and all authorization state.
- A collection is app-shared current state. It has no per-viewer ACL,
  server-side query language, durable event stream, or business-critical
  durability guarantee.

## Document API

Each document is an object of the form:

```json
{
  "id": "doc_abcdefghijklmnopqrstuv",
  "data": { "title": "Ship Tinkercloud", "done": false },
  "version": 1,
  "created_at": "2026-07-29T12:00:00Z",
  "updated_at": "2026-07-29T12:00:00Z"
}
```

- Collection names are 1–64 ASCII bytes, start and end with lowercase ASCII
  alphanumeric characters, and may contain lowercase ASCII alphanumerics,
  `_`, and `-` internally.
- IDs are server-issued opaque `doc_` identifiers. Callers may not supply one
  for creation. A route ID must match Tinkercloud's server-issued format.
- Data must be a JSON object, not an array, scalar, or `null`.
- Create, get, update, delete, bounded ID-ordered list, and bounded snapshot
  are the initial operations. Updates/deletes accept an optional expected
  document version and return conflict on stale or missing state.
- Every committed create/update/delete increments the collection revision and
  returns `Mutation{collection,id,version,deleted,revision}`. Only after that
  commit may the app-scoped in-memory live hub publish a freshness hint.

## Limits

Defaults are 100 collections per app, 10,000 documents per collection, a
64 KiB JSON object, 100 MiB total document bytes per app, list pages of at
most 100 documents, and snapshots of at most 1,000 documents. Requests that
would cross a limit deny without partial mutation.

The collection limit counts only active collections (those with at least one
current document). Empty `collection_revisions` rows remain solely to preserve
the monotonic revision when a deleted collection name is recreated; historical
names do not consume the active-collection limit.

## Persistence and recovery

Opening an app database is lazy. The manager holds a bounded number of handles
and closes idle files without a background worker. It configures SQLite WAL,
foreign keys, and a bounded busy timeout. The app-local platform schema is
forward-only and records its schema version in `platform_metadata`.

KV uses the same app-local SQLite file. The former control-database `app_kv`
table is removed by the pre-release destructive transition; production request
composition has no shared-control-database fallback.

Realtime is only a post-commit hint. There is no history, replay, ordering,
or resume cursor. On reconnect or a missed hint, the SDK reads an authoritative
snapshot/list from SQLite.

The SDK multiplexes managed KV-prefix and collection subscriptions for one
Tinker client over one app-scoped WebSocket. A listener's cleanup affects only
that listener; the connection closes when the last managed listener leaves.
The typed `unsubscribe_kv` and `unsubscribe_collection` frames remove the
corresponding server-side subscription immediately, so a removed listener
cannot receive a later hint. Reconnect creates a new connection, resubscribes
only the remaining subscriptions, and recovers current state through the same
authoritative reads. Explicit `tinker.live.channel(name)` channels remain
independent application channels.

## Deny-path charter

Tests must prove that missing/malformed authorization cannot touch a repository;
path-like app IDs cannot escape the app data root; App A never reads or mutates
App B's data; invalid collection names, caller IDs, non-object JSON, stale
versions, oversized documents, quota overages, and snapshots over the bound
deny without mutation; commit failures publish no mutation; concurrent stale
writes have one winner; and deletion removes only the validated app namespace.
