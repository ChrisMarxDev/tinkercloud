# Distribution Skill Contract

TinyHost has two maintainer-facing distribution skills:

- `distribute-tiny-cli` owns the native workstation CLI, signed installer,
  npm-family executable package, and Homebrew formula;
- `distribute-tiny-sdk` owns the TypeScript browser client package on npm and
  JSR.

They are independent of the deployer and operator role skills. They may share
the signed release and compatibility contracts, but they never transfer
operator, deployer, viewer, registry, or release authority between actors.

## Modes

Every invocation is classified before action:

1. `inspect`: read-only readiness and evidence;
2. `prepare`: local build, pack, generation, and verification only;
3. `publish`: explicit external mutation.

Inspect and prepare are the default. Publish requires a direct user request
that identifies or approves the exact version, finalized public identity,
destinations, and channel/tag. “Get ready,” “prepare distribution,” “test the
release,” and equivalent requests do not authorize publication.

During the rename freeze, one exception exists: an explicitly authorized
GitHub beta may use working names under the beta-release contract. It publishes
one complete signed prerelease through the protected repository workflow and
never mutates npm, JSR, Homebrew, stable/latest, or DNS state.

## Shared gates

- Read `PRINCIPLES.md`, then `PRD.md`, then the relevant distribution,
  compatibility, SDK/API, and release contracts.
- Preserve unrelated dirty-worktree changes.
- Require strict version parity and a complete verified signed release before
  deriving a channel candidate.
- Treat placeholders and working names as preparation-only inputs.
- Keep registry, release, tap, and signing credentials out of argv, chat,
  source, artifacts, logs, and reports.
- Never overwrite an immutable release or published version.
- On partial publication, stop and report exact external state; reconcile
  forward rather than rebuilding or concealing drift.

## CLI-specific gates

- All npm-family managers consume one package containing the four supported
  native CLI binaries and reviewed launcher.
- The approved command and product identity agree across the npm `bin` key,
  launcher, native artifact names, Homebrew formula, installer, and
  documentation. A generation parameter affecting only one channel does not
  establish rename completeness.
- The CLI package contains no server binary, runtime dependency, lifecycle
  downloader, or install-time release-origin selection.
- Homebrew and the one-line installer refer to the same verified release
  artifacts. Preparation does not mutate a tap or hosted origin.

## SDK-specific gates

- npm, JSR, `SDK_VERSION`, `APP_API_VERSION`, examples, compatibility range,
  and the signed release are version-consistent.
- The npm tarball contains only the five files allowed by the SDK distribution
  contract, and canonical repository metadata identifies the reviewed
  provenance source.
- The package has no runtime/peer dependency, consumer install hook, CDN
  import, secret, or server-internal dependency.
- JSR publication is blocked when the publishing CLI is unpinned or its dry-run
  file list has not been verified.

## Evidence

Each skill reports its mode, version, source commit, signed release, channel
identities, tests actually run, external verification actually observed, and
every unresolved placeholder or partial-publication state.
