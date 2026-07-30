# Update and Client Compatibility Denial Charter

- Empty, malformed, noncanonical, prerelease, overflowing, inverted, or
  unsupported versions/ranges fail closed.
- A supplied incompatible CLI/SDK or API version returns `426` without
  bypassing gateway authentication or app authorization.
- Compatibility endpoints contain no identity, hostname, app, credential,
  filesystem, provider, or persistence-derived field.
- A WebSocket with an offered but malformed/unsupported Tinker subprotocol is
  denied before hub attachment. Authorization, origin, and revocation checks
  remain independent.
- Unsigned or signature-valid-but-incompatible update artifacts never create a
  rollback snapshot, replace the binary, restart the service, or mutate schema.
- Missing compatibility headers remain a temporary V1 migration allowance, not
  evidence that an unknown client is compatible with a future API generation.
- A supported update, migration, or restart must not delete or recreate an
  existing operator/deployer identity, change its immutable ID/role/status,
  detach an app from its owner, or remove/replace the active access-policy
  revision or rules. Seeded-state regression proves those invariants before and
  after the supported migration/open path.
- Existing valid authority remains usable after update unless an explicit,
  versioned migration contract names that credential class and its required
  invalidation. Failed candidate verification, migration, replacement, or
  health never makes a partial or empty control database authoritative.
