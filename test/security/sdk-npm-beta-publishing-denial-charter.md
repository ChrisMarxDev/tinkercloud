# SDK npm Beta Publishing Denial Charter

The SDK npm beta path is incomplete until executable checks deny every case
below before registry mutation.

## Authority and trigger denials

- Any trigger other than `workflow_dispatch` is denied.
- Missing or mismatched `publish-npm-sdk-beta-vVERSION` confirmation is denied.
- A job without Environment `npm-sdk`, or with OIDC granted outside the
  mutation job, is denied.
- `NODE_AUTH_TOKEN`, `NPM_TOKEN`, repository npm secrets, inherited secrets,
  developer sessions, and credential-bearing `.npmrc` files are denied.
- npm publication of `@tinkercloud/cli`, the server, JSR, Homebrew, `latest`,
  `next`, or an input-selected dist-tag is denied.

## Source and release denials

- Malformed or suffixed versions, a missing tag, a tag outside `main`, a dirty
  checkout, or source/package/exported-version drift are denied.
- A missing, draft, or stable GitHub release is denied; only an already-public
  GitHub prerelease is accepted.
- A production-key claim, mismatched trust-anchor fingerprint, failed complete
  release verification, missing SDK asset, or multiple SDK assets is denied.
- Rebuilding, repacking, running `prepack`/`prepublishOnly`, or selecting bytes
  outside the verified signed release is denied.

## Package denials

- An unbootstrapped package is denied in the OIDC workflow and routed to the
  documented one-time 2FA bootstrap.
- An existing npm version, non-public package, wrong package name, wrong
  version, extra tarball file, unexpected export, runtime dependency, peer
  dependency, bundled dependency, binary, or install lifecycle hook is denied.
- Publishing without explicit `--access public`, `--tag beta`,
  `--ignore-scripts`, or provenance is denied.

## Post-publication denials

- A registry version mismatch, `beta` dist-tag mismatch, missing provenance
  attestation, repository metadata drift, or failed clean import leaves the
  workflow failed and the incident visible.
- The workflow never unpublishes, replaces, deprecates, removes a dist-tag, or
  reuses a version to conceal partial publication.
