# GitHub Beta Release Denial Charter

- A branch push, tag push, pull request, schedule, or repository dispatch never
  publishes a beta; only an explicit manual dispatch may enter `release.yml`.
  Its beta signing and npm modules accept only internal workflow calls.
- A malformed version, mismatched confirmation, absent tag, tag not reachable
  from `origin/main`, dirty checkout, non-public repository, version drift, or
  existing GitHub release fails before signing-secret access.
- Missing protected-environment approval or a missing, malformed, or
  non-matching beta private key fails before artifact publication.
- Tests and security gates run without the beta private key in their
  environment.
- Go and npm child builds do not inherit the base64 secret or signing-key path
  environment variable.
- A missing, extra, unsigned, incompatible, checksum-drifted, or
  signature-drifted release file prevents draft creation.
- A remote draft asset set or byte stream that differs from the locally
  verified release prevents publication and leaves the release as a draft.
- The workflow never uses asset clobbering, moves a version tag, overwrites a
  release, marks a beta as stable/latest, or silently deletes partial state.
- Missing npm package bootstrap, an occupied or unreadable SDK/CLI version, a
  wrong SDK/CLI beta tag, missing OIDC, or a missing package Environment denies
  the complete release. The workflow never runs `jsr publish`, `brew`, mutates
  a Homebrew tap, reserves a namespace, changes DNS, or enables scheduled/silent
  server updates.
- The signing key never appears in argv, the checkout, cache, artifacts,
  metadata, logs, outputs, release notes, or the installed CLI/server.
- A post-publication installer failure is a release incident and requires
  forward recovery with a new numeric version.
- The beta signing authority is rejected as the first stable-release authority;
  stable publication remains blocked until every embedded trust anchor is
  rotated together to a new operator-controlled production authority.
