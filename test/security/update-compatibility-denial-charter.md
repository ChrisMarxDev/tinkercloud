# Update and Client Compatibility Denial Charter

- Empty, malformed, noncanonical, prerelease, overflowing, inverted, or
  unsupported versions/ranges fail closed.
- A supplied incompatible CLI/SDK or API version returns `426` without
  bypassing gateway authentication or app authorization.
- Compatibility endpoints contain no identity, hostname, app, credential,
  filesystem, provider, or persistence-derived field.
- A WebSocket with an offered but malformed/unsupported Tiny subprotocol is
  denied before hub attachment. Authorization, origin, and revocation checks
  remain independent.
- Unsigned or signature-valid-but-incompatible update artifacts never create a
  rollback snapshot, replace the binary, restart the service, or mutate schema.
- Missing compatibility headers remain a temporary V1 migration allowance, not
  evidence that an unknown client is compatible with a future API generation.
