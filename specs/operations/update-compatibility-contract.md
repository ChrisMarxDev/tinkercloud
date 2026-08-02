# Release and Client Compatibility Contract

Compatibility is an explicit release property, not a best-effort version
comparison after replacement.

## Version model

Versions are strict ASCII `MAJOR.MINOR.PATCH` values. Ranges are
`min_inclusive <= version < max_exclusive`; malformed, empty, inverted, or
unbounded ranges are invalid.

The current V1 compatibility matrix contains:

- server release version;
- control API version and CLI range;
- app API version and TypeScript SDK range;
- persisted schema version;
- release-manifest schema version.

The matrix is compiled into the server and CLI and copied into the signed
release manifest. Release verification rejects drift between those copies and
the built SDK/package versions.

## Runtime negotiation

- `GET /api/v1/version` remains the exact legacy health document
  `{"api_version":1}`.
- `GET /api/v1/compatibility` is a public, bounded, no-store document containing
  only the server version and supported CLI, SDK, API, and schema values.
- The CLI sends its version and control-API version on control requests. A
  supplied malformed or unsupported version receives `426`; missing headers
  remain accepted during V1 compatibility migration.
- The SDK sends its version and app-API version on HTTP calls and offers the
  same pair through a fixed WebSocket subprotocol. A supplied malformed or
  unsupported version is denied before a capability call or WebSocket attach.
- Compatibility denial never authenticates a caller and never reveals app,
  viewer, deployer, operator, or persistence data.

## Update preflight

- Artifact digest/signature verification and compatibility validation both
  complete before snapshot, binary replacement, or restart.
- Remote updates fetch and authenticate both the server artifact and the
  signed release manifest from the same release origin. Air-gapped updates
  require both signed triplets; mixing local and remote inputs is invalid.
- The candidate server must support the installed schema and current API
  generation. Unsupported values fail as `verification_failed`.
- Hosted apps continue to use immutable bundled SDK code. The server preserves
  the currently supported SDK range; advancing the minimum SDK requires
  installed-deployment compatibility evidence and a separate migration
  decision. V1 preparation therefore keeps the minimum at `0.1.0`.
- Post-restart health, denial evidence, rollback, and manual-update requirements
  in the M5 contract remain mandatory.

## Persistent-state preservation

A supported Tinkercloud update and its embedded schema migrations operate on the
installed control database in the configured data directory. They must preserve
every existing operator and deployer user row (including immutable ID,
normalized email, role, and status), every app's owner, and every active access
policy revision and rule. They must also preserve an otherwise-valid existing
authority credential unless a separately versioned migration contract names the
credential class and its required invalidation reason. An update must never
silently delete or recreate accounts, ownership, or access rules.

Automatic updates require the candidate schema version to equal the installed
schema version. Discovery of a different schema is a non-mutating stop that
requires a separately accepted manual migration contract. The automatic path
does not snapshot and later restore databases: retaining the same compatible
durable authority prevents binary rollback from discarding writes accepted by
either healthy binary.

The preservation assertion applies before the candidate binary is considered
healthy and again after the supported restart path. Failed verification,
migration, replacement, or post-restart health must leave the prior healthy
binary and the same durable data directory authoritative. Update validation
must include an executable seeded-state regression for operator/deployer
identity, owned-app policy, and valid authority continuity; a migration that
intentionally changes credential validity requires an explicit compatibility
decision and narrow regression in addition to this invariant.

The cross-version regression uses credentials issued before replacement. It
must prove that a stored CLI bearer, global browser identity, derived app
session, owned app, active policy, immutable release, and app-local KV,
document, and blob values remain usable after the candidate becomes healthy.
The same seeded state must remain usable after a deliberately unhealthy signed
candidate triggers automatic binary rollback.
