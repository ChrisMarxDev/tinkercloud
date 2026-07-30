# ADR 0049: Typed deployer access to managed app data

**Status:** Accepted

## Context

Tinkercloud deployers can manage an owned app but could not inspect or repair the
bounded KV and document state that the app stores through the SDK. Giving a
deployer a database URL, SQLite file, SQL console, or a second network listener
would bypass the gateway's server-derived tenancy model and create a remote
database authority. It would also expose internal schema/path details and make
the single-operator deployment less understandable.

The existing app data repositories correctly depend on a server-derived
immutable app ID, but their app-viewer authorization context must not be
forged from a control request. The platform needs a narrow way for the bearer
control plane to reach the same managed-data capabilities without making a
deployer a viewer or merging CLI/control credentials with browser sessions.

## Decision

Introduce a control-plane deployer-data surface with two explicit scopes:

- `data:read` for bounded KV/document inspection; and
- `data:write` for one version-checked KV/document mutation at a time.

The bearer-control gateway authenticates the deployer, checks active status,
token revocation/expiry, exact scope, optional app binding, and ownership of
the route slug. It then resolves the immutable app ID internally and constructs
a sealed typed `DeployerDataAuthorizationContext`. Only the control gateway
and the app gateway may construct their respective typed authorization
contexts; neither can call an app-data dispatcher with a boolean or a
client-supplied app ID.

The private typed scope carries the server-derived app identity and
authenticated owner subject; the validated mutation request key remains a
separate adapter input. Common app-data services may accept this narrow scope
or a shared sealed app-data-authority interface, but repositories continue to
receive only the already-derived immutable app ID. The caller never obtains a
database path, connection string, SQLite handle, schema, SQL operation, or a
public data listener.

The first product surface is the saved-credential `tinker data` CLI plus its
matching bearer HTTP API. It supports bounded reads and deliberate individual
writes against the existing KV/document semantics: KV set may carry an optional
expected version, while destructive KV/document updates/deletes require one.
It does not support a SQL
REPL, joins/filter language, migrations, raw database download, bulk delete,
transaction scripts, export/import, or a backup product. Writes use the
existing optimistic versions. Before a write begins, Tinkercloud stores a
metadata-only audit intent in the control database. The separate control/app
SQLite files cannot form one cross-database transaction. After an app-data
commit, Tinkercloud marks the matching intent succeeded. An exact retry can return
the existing receipt, and an interrupted outcome update is reconciled only
when current versioned app state proves the same result. This is deliberately
not a rollback or general cross-database recovery claim.

Global interactive deployer bearers may use `data:read` for owned active or
suspended apps. `data:write` is active-app only. App-scoped deployment-agent
tokens receive no data scope by default and must be explicitly app-bound and
scoped. Browser identity cookies, app viewer credentials, and matching email
text receive no remote data authority from this decision. Operator status alone does
not grant access to an unowned app; an operator who is that app's exact current
owner uses the same owner-scoped CLI path. Deleting, deleted, unavailable,
foreign, or ambiguity-resolved-false apps deny data access.

The normative interface and denial evidence are
[`specs/api/deployer-data-contract.md`](../../specs/api/deployer-data-contract.md)
and
[`test/security/deployer-data-denial-charter.md`](../../test/security/deployer-data-denial-charter.md).

## Consequences

- A deployer can inspect and safely repair app-managed state through the same
  server/process/database-manager boundary used by the app capability.
- Per-app SQLite files stay private operational implementation details; Tinkercloud
  remains one server process, one embedded SQLite engine, one private data
  root, and no database daemon/listener.
- Authorization stays current on every request. Deployer, token, ownership,
  app-lifecycle, and app-binding changes deny the next applicable operation.
- The API must test both context construction paths and prove that no
  client-controlled selector can cross app boundaries or become a viewer
  session.
- Data writes add a pre-write audit-intent dependency. Intent failure fails
  closed before the app mutation. A post-commit outcome enables exact replay;
  incomplete outcomes reconcile only from provable versioned state and
  otherwise fail closed. There is no false cross-database rollback claim.

## Rejected alternatives

- **Raw SQLite download or remote SQLite listener:** leaks internal storage
  authority and bypasses per-operation authorization/audit.
- **SQL console:** adds an unbounded public interface coupled to internal schema
  and makes safe limits/authorization substantially harder.
- **Treat deployers as synthetic viewers:** conflates browser policy access with
  management authority and makes actor/audit semantics ambiguous.
- **Operator dashboard data browser by default:** broadens a separate role and
  is not needed to satisfy the deployer workflow.
- **App-selected database ID/path:** violates server-derived tenancy and risks
  cross-app file selection.

## Reconsider when

Measured demand requires a portable export/import workflow, richer filtered
inspection, or multi-app data administration. Any such change requires a new
contract and ADR that retains the private database authority, bounded resource
model, and negative cross-app evidence.
