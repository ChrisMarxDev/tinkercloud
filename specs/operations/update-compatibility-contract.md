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
