# Deployer data access contract

This contract defines the bounded control-plane API used by an authenticated
deployer and the `tiny data` CLI to inspect and deliberately repair managed app
data. It extends the existing KV and JSON document-collection primitives. It
does **not** expose SQLite, SQL, database schemas, files, connection strings,
or a second database listener.

## Deny-path charter

The implementation must prove these denials before reporting a positive data
operation as complete:

- Anonymous requests, global browser identity cookies, app viewer cookies, and
  app-scoped viewer credentials cannot call this bearer
  control API.
- A deployer cannot read or mutate an app they do not own, including when the
  slug, key, collection, document ID, cursor, or API token is guessed.
- A missing, expired, revoked, suspended, wrong-app, or wrong-scope deployer
  credential is denied before an app database is opened.
- A deployment-agent token has no data access unless it has an explicit
  app-scoped `data:read` or `data:write` scope. An app-scoped token for one app
  cannot address another owned app.
- Invalid, duplicate, overlong, encoded-separator, traversal-like, or otherwise
  malformed slugs, keys, collection names, document IDs, cursors, limits,
  expected versions, JSON bodies, and idempotency keys fail closed.
- A stale expected version, quota/rate-limit rejection, unavailable/locked app
  database, cancellation, or durable pre-write audit-intent failure cannot
  commit a partial mutation.
- Responses, audit records, logs, and error envelopes never reveal another
  app's values, an app-database path, SQL/schema detail, token material, or
  document bodies outside the successful response to the authorized caller.
- App deletion denies every data operation as soon as deletion starts. A
  deleted/unknown app and a foreign app return the same safe authorization
  envelope.

## Trust and ownership boundary

The route slug is a control-plane resource name, not an app/database selector.
After bearer authentication, the control gateway verifies the deployer's
current active status, exact token scope, optional token app binding, and
ownership of the slug. Only then may it resolve the immutable app ID and issue
a sealed typed `DeployerDataAuthorizationContext` to the app-data service.

That private typed scope contains only server-derived app identity and the
authenticated owner subject. Mutation adapters receive the validated
idempotency/request key separately. It is distinct from an app viewer
authorization context; neither a synthetic viewer session nor an untyped
`authorized=true` flag may reach KV or collection repositories. The
repositories continue to receive only server-derived immutable app identity.

The control database remains the authority for deployer, token, ownership,
app-lifecycle, and audit state. The selected app-local SQLite file remains the
only authority for that app's KV/documents. This API creates no remote database
authority, file download route, or raw-SQL surface.

## Actor and lifecycle semantics

| Actor | `data:read` | `data:write` |
| --- | --- | --- |
| Exact current app owner using a global CLI bearer (stored as deployer or operator) | May read owned active or suspended app data | May mutate an owned active app only |
| App-scoped deployment-agent token | May read only its bound owned app when explicitly scoped | May mutate only its bound owned active app when explicitly scoped |
| Non-owner operator, dashboard browser session, viewer, anonymous caller | Not granted by this contract | Not granted by this contract |

`deleting`, deleted, unavailable, or ownership-ambiguous apps deny both
access types. Deployer suspension or token revocation takes effect on the next
request. Reads for a suspended owned app support diagnosis; writes require the
app to be active. Operator status alone never grants remote data access to an
unowned app; an operator who is the exact current owner acts through the same
owner-scoped CLI path as a deployer.

## API shape

All routes are on the admin host under `/api/v1` and require
`Authorization: Bearer` plus the normal released-client control API/version
headers. Responses use the common JSON envelope and safe error codes in
`http-contract.md`. App data is never served from an app origin through this
surface.

### Read operations

```text
GET /api/v1/apps/{slug}/data/kv?prefix=&limit=&cursor=
GET /api/v1/apps/{slug}/data/kv/{key}

GET /api/v1/apps/{slug}/data/collections?limit=&cursor=
GET /api/v1/apps/{slug}/data/collections/{collection}/documents?limit=&cursor=
GET /api/v1/apps/{slug}/data/collections/{collection}/documents/{id}
```

Every read requires `data:read`, is bounded by the existing KV/collection
limits, and returns deterministic ID/key ordering. The current cursor is a
bounded last key/document ID ordering marker, not an opaque authorization
credential: reuse can only affect ordering inside the already server-derived
app/resource scope. It cannot select another app/database or bypass collection
validation. A list always returns an array; it returns `next_cursor` only when
there is a following page. Missing records use the same safe not-found/unavailable
representation already used by the matching app capability; a caller cannot
distinguish a foreign record.

### Deliberate write operations

```text
PUT    /api/v1/apps/{slug}/data/kv/{key}
DELETE /api/v1/apps/{slug}/data/kv/{key}
POST   /api/v1/apps/{slug}/data/collections/{collection}/documents
PUT    /api/v1/apps/{slug}/data/collections/{collection}/documents/{id}
DELETE /api/v1/apps/{slug}/data/collections/{collection}/documents/{id}
```

Every mutation requires `data:write`, an active owned app, a normal bounded
idempotency key, and the same validation/quota rules as the app capability. A
KV set may carry an optional `expected_version`, matching the existing KV
primitive; KV deletes and document updates/deletes require it. A stale or
missing required version returns `409 version_conflict` without mutation.
Document creation accepts one bounded JSON object and always receives its
opaque ID from the server. There are no bulk write, wildcard delete, arbitrary
filter, schema, migration, import, or transaction-script operations.

Before an app-data write begins, TinyHost durably records a metadata-only audit
intent in the separate control database. The intent contains actor, app,
operation kind, resource name/opaque record ID, request ID, and a one-way
request digest; it never stores the JSON value/document body. Failure to
persist that intent denies the mutation before opening/mutating the app
database. After a successful app-data commit, TinyHost marks the intent
`succeeded`. Exact completed retries return the existing safe receipt; reuse
with different input denies. If the separate outcome update fails after the
app commit, a later exact retry reconciles only when current versioned app
state proves the intended result. Otherwise it fails closed. This is not a
cross-database transaction and makes no rollback or general recovery claim.
A new successful mutation, including one recovered from a provable interrupted
outcome, emits the normal app-scoped best-effort KV/collection freshness hint
after completion. A denied/failed mutation or already-completed replay emits
none; the hint remains non-durable and app clients reconcile current state.

## CLI contract

`tiny data` reuses the normal saved platform URL and authenticated deployer
credential. It never asks for a database URL, app ID, database credential, or
an ad-hoc config file. Its app argument is a public owned slug passed to the
control API; the server determines the private data scope.

```text
tiny data kv list APP [--prefix PREFIX] [--limit N] [--cursor CURSOR] [--json]
tiny data kv get APP KEY [--json]
tiny data kv set APP KEY (--file FILE | --stdin) [--expected-version N] [--json]
tiny data kv delete APP KEY --expected-version N [--confirm delete:APP:KEY] [--json]

tiny data collections list APP [--limit N] [--cursor CURSOR] [--json]
tiny data documents list APP COLLECTION [--limit N] [--cursor CURSOR] [--json]
tiny data documents get APP COLLECTION ID [--json]
tiny data documents create APP COLLECTION (--file FILE | --stdin) [--json]
tiny data documents update APP COLLECTION ID (--file FILE | --stdin) --expected-version N [--json]
tiny data documents delete APP COLLECTION ID --expected-version N [--confirm delete:APP:COLLECTION:ID] [--json]
```

Interactive human mode asks the deployer to type the exact target-bound
`delete:...` phrase when `--confirm` is omitted. Non-interactive and JSON mode
require the exact `--confirm delete:...` value; there is no unbound `--yes`
bypass. JSON mode never prompts and emits the stable API-shaped result or error
envelope only. Values
come from exactly one bounded UTF-8 JSON `--file` or explicit `--stdin` source,
never an inline argv JSON literal. The CLI does not implement a SQL shell,
stream a live database, download a database file, or add export/import/backup
commands.

Existing CLI bearers are not scope-upgraded in place. A credential issued
before deployer data access remains least-privilege and receives
`not_authorized`; an exact owner explicitly runs `tiny login --force` once to
obtain a newly scoped interactive bearer.

## Compatibility and evidence

This is an additive control API under version 1. A change to authorization
meaning, value/document response shape, cursor binding, required concurrency,
or idempotency semantics requires a versioned contract decision. The
implementation must extend the control-auth, HTTP, app-data, CLI, audit, and
security regression suites and exercise the denial charter against two
deployers/two apps before a release is considered complete.
