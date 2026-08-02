# Stable Distribution Denial Charter

Write and keep these denials executable before enabling the stable happy path.
No denial may be bypassed to repair a partial publication.

## Stable GitHub release

- Reject automatic triggers, malformed versions, a confirmation other than
  `publish-stable-vVERSION`, a missing or moved tag, a tag not reachable from
  `origin/main`, a dirty detached checkout, a private repository, or an
  existing GitHub release for the tag.
- Reject any stable build while `packaging/release-key-policy.json` does not
  classify the committed trust anchor as `production`, or when its recorded
  public-key fingerprint differs from `packaging/release-public-key.pem`.
- Expose the production signing secret only to the reviewer-protected
  `stable-release` job. Reject ambient inherited secrets, persistent key files,
  key material in argv/logs/artifacts, and signing before the normal test gates
  pass.
- Reject publication unless the complete local release verifies, a draft is
  created without replacing assets, the downloaded draft has the identical
  file set, and the downloaded release verifies again.
- Reject release deletion, asset clobbering, tag movement, or re-signing an
  existing version. A failure after publication is reconciled forward.
- Reject a `latest` installer smoke test whose installed `tinker version` does
  not equal the exact source version.

## npm SDK and CLI publication

- Reject automatic triggers, a non-stable GitHub release, source/release
  version drift, a source tag outside `main`, or a release that fails complete
  checksum, metadata, signature, and manifest verification.
- Reject the complete release before public mutation when either npm package
  does not already exist or either same-version package is occupied. Each first
  package must be created interactively with 2FA from exact verified artifacts
  before trusted publishing can be configured.
- Reject an already-published version, an implicit dist-tag, a private scoped
  package, a repository URL other than the canonical GitHub repository, or a
  workflow without the matching protected `npm-sdk` or `npm-cli` Environment and OIDC
  `id-token: write` permission.
- Reject a candidate containing lifecycle scripts, runtime/peer dependencies,
  the `tinkercloud` server, a download URL, a credential or signing key, or any
  platform outside Linux/macOS on amd64/arm64.
- Reject publication from a rebuilt release. Use the exact SDK tarball and
  generate the CLI wrapper only from the anonymously downloaded, verified
  stable release and source at its exact tag.
- Reject overwrite, unpublish, or concealment after a partial publication.
  Preserve the external state and reconcile forward.

## One-line installer

- Reject root execution, non-HTTPS origins, HTTPS redirects to non-HTTPS,
  unsupported OS/architecture, missing verification inputs, malformed
  metadata/checksums/signatures, or an invalid Ed25519 signature before writing
  the destination binary.
- A `latest` release changing between requests may fail closed; it must never
  weaken verification. The exact `vVERSION` form remains available for
  reproducible installation.
