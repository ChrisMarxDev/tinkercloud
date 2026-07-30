# Deployer data access denial test charter

This charter covers the control-plane data-administration surface defined by
[`specs/api/deployer-data-contract.md`](../../specs/api/deployer-data-contract.md).
It is required before the `tinker data` happy path counts as evidence.

## Credential and ownership matrix

- Anonymous, malformed, expired, revoked, and suspended global CLI bearers are
  denied before opening an app database.
- A global browser identity cookie, app-viewer cookie, or app-scoped viewer
  bearer presented as `Authorization: Bearer` is denied
  and does not gain data access.
- Deployer A cannot list, get, create, update, or delete Deployer B's app data;
  unknown and foreign app slugs have the same safe result.
- Global deployer tokens require the exact `data:read` or `data:write` scope.
  `data:read` cannot mutate, and a token with unrelated deployment/access
  scopes cannot inspect data.
- An app-scoped deployment-agent token may access only its own app and only
  with an explicit matching data scope. It cannot widen an omitted binding by
  supplying another slug.
- Operator authority, same normalized email text, and viewer policy do not
  implicitly bridge into this remote data authority. An operator who is the
  exact current app owner may use the owner-scoped CLI path, but has no access
  to an app they do not own.

## Selector, input, and lifecycle denial

- Invalid percent encodings, encoded separators, dot segments, duplicate
  separators, traversal-like names, overlong values, invalid Unicode/control
  characters, and invalid cursor/limit/version formats deny before repository
  access.
- Extra JSON members, duplicate JSON keys, non-object document data, invalid
  JSON, oversized bodies, and missing idempotency/expected-version inputs
  cannot partially mutate state.
- Foreign/missing KV keys, collections, and document IDs reveal no other app
  existence or bytes.
- A suspended owned app permits only `data:read`; write requests deny. A
  deleting/deleted/unavailable app denies both reads and writes.
- Token/deployer/app revocation between authentication and repository dispatch
  is rechecked so the next relevant request denies. A request cannot reuse a
  stale authorization context.

## Persistence, concurrency, and disclosure denial

- Stale/missing expected versions return `version_conflict` and leave current
  KV/document state unchanged.
- Quota rejection, context cancellation, busy/locked/unavailable app DB, and
  durable pre-write audit-intent failure leave no partial KV/document write.
- A failing pre-write audit-intent insert denies before the app database
  mutation. A failed post-commit outcome update leaves a durable attempted
  intent; an exact retry succeeds only when current versioned state proves the
  intended result. Mismatched or ambiguous replay fails closed. Tests must not
  claim a cross-database rollback that the separate SQLite files do not
  provide.
- Denied, failed, and already-completed replay operations emit no live
  freshness hint. A newly committed or safely reconciled mutation emits only
  to the server-derived app scope.
- Pages stay within configured limits. A cursor is only a bounded ordering
  marker inside the already server-derived app/resource scope: reuse may affect
  ordering but cannot select another app/database or bypass collection
  validation. An exhausted cursor returns an empty deterministic page.
- Response bodies, request logs, audit records, CLI errors, and error envelopes
  do not disclose token values, values/document bodies on denial, filesystem
  paths, SQLite/WAL details, SQL/schema internals, or another app's resource.

## End-to-end evidence

- Use two active deployers and two apps on the composed gateway. Establish
  authorized read and deliberate-write operations for the owner, then repeat
  every ownership/scope/revocation/malformed denial through the real CLI/API.
- Verify `tinker data --json` never prompts, stores a credential, or emits
  progress mixed with JSON; destructive delete requires the exact target-bound
  `--confirm delete:...` value in both human and JSON modes.
- Ensure no route, SDK call, manifest field, dashboard control, or browser app
  configuration exposes raw SQL, a database file/path, a data connection
  string, export/import/backup behavior, or an app-selected immutable app ID.
