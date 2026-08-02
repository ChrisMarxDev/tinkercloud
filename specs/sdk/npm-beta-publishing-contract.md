# SDK npm Beta Publishing Contract

## Scope

This contract governs the SDK registry stage of the unified pre-stable release:
publishing the exact `@tinkercloud/sdk` tarball from the same-version verified
Tinkercloud GitHub prerelease to the public npm registry under the `beta`
dist-tag.

It does not govern the separately isolated CLI npm stage and does not authorize
publication of the `tinkercloud` server, JSR, Homebrew, a stable GitHub release,
npm's `latest` dist-tag, automatic triggers, scheduled publication, or
rebuilding registry bytes.

## Version and source identity

- The version is strict numeric `MAJOR.MINOR.PATCH`, matching the existing V1
  compatibility model.
- Beta state is represented by the GitHub prerelease flag and npm's exact
  `beta` dist-tag. A prerelease suffix is not added to the package version.
- The source tag is exactly `vVERSION`, exists, and resolves to a commit
  reachable from `origin/main`.
- The corresponding GitHub release is published, is a prerelease, and is not a
  draft.
- Package, JSR, exported SDK, signed manifest, SDK tarball, and source-tag
  versions are equal.
- A published npm version is immutable. Every later beta uses a new patch
  version; an existing version is denied even if its dist-tag is wrong.

## Publication authority

Publication follows the sole `release.yml` dispatch with channel `beta`, the
exact numeric version, and confirmation `publish-beta-vVERSION`. The mutation job uses the
reviewer-protected GitHub Environment `npm-sdk` and grants `id-token: write`
only in that job.

The npm package trusts only GitHub Actions from repository
`ChrisMarxDev/tinkercloud`, workflow `release.yml`, Environment
`npm-sdk`, for `npm publish`. No npm token, developer npm session, `.npmrc`
credential, or inherited secret is available to the workflow. npm trusted
publishing supplies short-lived OIDC authentication and provenance.

Because npm cannot attach a trusted publisher before the package exists, the
first version is a one-time interactive bootstrap. It must publish the exact
SDK tarball from the verified GitHub prerelease with 2FA, public access,
scripts disabled, provenance where supported, and the `beta` dist-tag. The
trusted publisher is configured immediately afterward; later versions use only
the protected workflow.

## Workflow

1. Before any public mutation, the unified preflight validates input,
   confirmation, tag, main ancestry, repository visibility, version parity,
   package bootstrap, and absence of both npm versions. The SDK stage repeats
   its relevant checks before entering the protected Environment.
2. Check out the exact source commit, download every asset from the canonical
   GitHub prerelease, and verify the complete signed release with the committed
   beta trust anchor.
3. Select `tinkercloud-sdk-VERSION.tgz` without rebuilding or repacking it.
4. Verify the tarball name, exact five-file payload, package metadata, version,
   exports, lack of runtime dependencies and install hooks, and runtime
   `SDK_VERSION`.
5. Recheck that the npm version is absent immediately before mutation.
6. Run `npm publish` on that exact tarball with public access, lifecycle scripts
   disabled, provenance enabled, and the fixed `beta` dist-tag.
7. Wait for bounded registry propagation, then verify the exact public version,
   `beta` dist-tag, provenance attestation, repository identity, and clean
   consumer import.

## Failure and recovery

- Failure before `npm publish` creates no npm state.
- Failure after npm accepts the version is a visible partial-publication
  incident. Preserve the workflow evidence and do not unpublish, replace, or
  reuse the version.
- A missing or incorrect `beta` dist-tag is repaired only through a separately
  reviewed operator action; the publish workflow never silently changes tags.
- Changed bytes or source fixes advance to a new patch version and repeat the
  canonical GitHub prerelease flow first.
- Stable publication requires the separately controlled production authority;
  beta trust is never promoted implicitly.
