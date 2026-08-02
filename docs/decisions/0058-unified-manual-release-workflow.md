# ADR 0058: One workflow owns a complete public release

## Status

Accepted

## Decision

One manually dispatched `release.yml` workflow owns each complete public
release. Its exact `vVERSION` tag is the change gate and it has explicit
`beta` or `stable` channel input. Before any public mutation it denies an
existing GitHub release and either existing npm version. It builds one signed
GitHub release, then publishes the exact SDK tarball and release-derived CLI
package from that same version.

The signing job keeps separate protected `beta-release` and `stable-release`
environments. The npm jobs keep separate protected `npm-sdk` and `npm-cli`
environments and OIDC boundaries, while both packages trust the one workflow
filename. Beta tags are `beta` (SDK) and `next` (CLI); stable uses `latest`.

npm versions and GitHub releases are immutable. A partial post-publication
failure preserves evidence and advances with a new version; it never overwrites,
unpublishes, or reuses a version.

## Consequences

Each npm package needs an initial exact-artifact 2FA bootstrap, then must trust
`release.yml` and its own protected environment for `npm publish`.
