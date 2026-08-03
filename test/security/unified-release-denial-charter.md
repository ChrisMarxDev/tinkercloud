# Unified release denial charter

The sole release invocation denies before public mutation when channel,
strict numeric version, or `publish-CHANNEL-vVERSION` confirmation is wrong;
the tag is missing, unreachable from `main`, dirty, or version-drifted; the
selected authority is wrong; a GitHub release already exists; either npm
package version exists or cannot be checked; either package lacks 2FA
bootstrap; the release fails signature verification; or required protected
environment/OIDC permissions are missing.

Beta denies `latest` and requires SDK `beta` plus CLI `next`. Stable denies
prerelease tags and requires `latest` for both. Neither flow rebuilds the SDK
tarball, overwrites, unpublishes, hides, or reuses public state. Any
post-publication failure preserves evidence and fixes forward with a new version.

The release builder's SDK packaging self-test rejects an npm invocation or npm
pack output path rooted in the tracked `sdk/typescript` source directory. SDK
installation, build, and packing must occur in a task-local workspace before
the resulting tarball is signed.

The executable workflow checker rejects a beta or stable reusable validation
workflow that references a release signing secret, attaches its release
Environment, or gains release-write authority. It also rejects ambient secret
inheritance: the selected signing secret must be attached directly to the
normal protected job in top-level `release.yml`.
