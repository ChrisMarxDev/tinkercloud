# GitHub Beta Release Contract

## Scope

This contract authorizes one pre-stable distribution channel: manually approved
GitHub prereleases containing the complete signed Tinkercloud release. It does
not authorize npm, JSR, Homebrew, stable GitHub releases, `latest` promotion,
DNS changes, or scheduled updates.

Product and artifact identities are locked by ADR 0052. Every release under
this contract is visibly marked as beta. Stable distribution requires a new
production signing authority, but not another product rename.

## Source identity

- A beta version is strict `MAJOR.MINOR.PATCH`.
- The source tag is exactly `vVERSION`, already exists, and resolves to a commit
  reachable from `origin/main`.
- Package, JSR, exported SDK, server, CLI, signed manifest, and source-tag
  versions are equal.
- The checkout is detached at the tag and clean before tests or signing.
- The GitHub repository is public so the documented anonymous installer and SDK
  tarball paths are truthful.
- A tag with an existing GitHub release is rejected. Published versions and
  assets are never replaced.

GitHub's prerelease state defines the beta channel. Prerelease version suffixes
are not introduced into the V1 compatibility model; another public beta uses a
new patch version.

## Publication authority

The workflow is manually dispatched with:

- the exact numeric version; and
- the exact confirmation `publish-beta-vVERSION`.

The signing job uses the protected GitHub Environment `beta-release`.
Operators configure required reviewers, prevent self-review where their GitHub
plan supports it, and restrict deployment branches to `main` and protected
version tags. They also enable GitHub release immutability before the first beta.

The committed `packaging/release-public-key.pem` authority is beta-only. Its
matching private key is stored as the environment secret
`TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64`. It is decoded only under
`RUNNER_TEMP` with a random filename and mode `0600`, immediately before the
signed release build and removed in the same step. The base64 environment value
is unset after decoding, and the builder unsets its signing-key environment
variable before invoking Go or npm child builds. Key bytes never appear in the
checkout, artifacts, cache, command arguments, logs, outputs, or release notes.

Before the first stable release, the operator generates a new production
authority outside GitHub, updates every embedded trust anchor together, and
retires the beta authority. Beta trust never silently becomes stable trust.

## Workflow

1. Validate the input, confirmation, tag, main ancestry, clean checkout,
   version parity, and absence of an existing release without signing-secret
   access.
2. Run the normal SDK, Go, race, security, vulnerability, agent-skill,
   workflow, release-tamper, and installer gates.
3. Build the complete release once with the beta authority.
4. Verify the complete signed release without executing a candidate.
5. Create a new draft GitHub prerelease and attach every verified release file.
6. Download the draft assets into a fresh directory and verify them again.
7. Publish the verified draft as a prerelease without promoting it to latest.
8. Exercise the public client installer into a temporary non-root directory and
   prove `tinker version` equals the release version.

The workflow creates no package-manager candidate and invokes no registry or
tap publisher.

## Failure and recovery

- Failure before draft creation creates no hosted release state.
- Upload or remote-verification failure leaves a draft for operator diagnosis.
- Failure after publication is reported as a release incident. The release is
  not edited, replaced, re-signed, deleted and recreated, or hidden by moving a
  tag.
- Any changed artifact or source fix is released forward under a new version.
- Workflow cancellation never triggers cleanup of a published release or
  operator-owned signing authority.

## Consumer paths

For version `VERSION`, the beta release base is:

```text
https://github.com/ChrisMarxDev/tinkercloud/releases/download/vVERSION/
```

The public client bootstrap downloads `install-client.sh` from that versioned
base. `tinker host install` and `tinker host update` use the same exact release
base. TypeScript consumers may install the exact
`tinkercloud-sdk-VERSION.tgz` GitHub asset by URL without publishing it to an npm
or JSR registry. Consumers must opt into the beta explicitly; documentation
does not advertise an unversioned stable or latest installer.
