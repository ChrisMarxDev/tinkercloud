# Definition of Done

## Any change

- Outcome and non-goals are explicit.
- Relevant contract and documentation match behavior.
- Unit and integration tests cover success and failure.
- Logs/audit omit secrets and personal data not required by the actor.
- Limits, cancellation, idempotency, and concurrency are considered.
- No unresolved TODO weakens authentication, authorization, isolation, update
  recovery,
  or activation.

## New protected request surface

- Route is classified in the registry.
- Handler requires `AuthorizationContext`.
- App and identity are server-derived.
- Anonymous, wrong-app, revoked, suspended, and dependency-failure cases deny.
- Response/body limits and redaction are tested.
- App JavaScript cannot use ambient credentials to cross into the control plane.

## Deployment/release change

- Archive and filesystem inputs are treated as hostile.
- Transition is represented in the deployment state machine.
- Interruption before/after durable writes recovers deterministically.
- Previous active release remains usable on failure.
- Audit and security probes complete before success is reported.

## SDK or capability change

- Typed SDK method, capability discovery, and HTTP contract agree.
- Method accepts neither app ID nor provider secret.
- Examples compile and pass against a real test server.
- The packed SDK contains its README, Apache-2.0 license, declarations, and
  runtime module, with development source and tests excluded.
- npm and JSR manifests match the exported SDK and release versions; the npm
  tarball installs and imports in a clean offline consumer.
- Publication checks are dry runs until namespace ownership and a reviewed
  trusted-publishing release are explicitly approved.
- Permissions, quotas, external data disclosure, and typed failures are clear.
- Relevant coding-agent skill and verification workflow are updated.
- Future provider adapters never expose a read-secret or generic secret-injection
  path to browser code.

## Persistence/migration change

- Fresh install and upgrade paths pass.
- Failed/interrupted migration behavior is defined.
- Update compatibility and failed-health rollback are tested.
- Tenant keys/indexes preserve app scope.
- Roll-forward recovery is documented; destructive downgrade is not assumed.

## Blob/storage change

- SQLite catalog state and byte-store state have an explicit interruption
  model; only ready metadata can serve bytes.
- Uploads, downloads, and cleanup accept no app ID, filesystem path, storage
  key, bucket, endpoint, public URL, or provider credential from app input.
- Wrong-app, revoked, partial-write, quota/disk, cancellation, missing/corrupt
  bytes, orphan, and metadata/storage disagreement cases fail closed.
- Download headers prevent the supported API from treating uploaded active
  content as an inline TinyHost app-origin document.
- Local storage remains inside the configured private data directory; any
  future remote adapter is direct and server-side, not a FUSE mount or browser
  credential.

## Release candidate

- All milestone exit gates pass.
- Race, fuzz corpus, dependency, and secret scans pass.
- CI records bounded fuzz, pinned Go vulnerability, locked npm audit, and
  version-controlled-candidate secret-pattern scan evidence, including files
  intended for a repository's first commit; its secret scanner has private-key
  and live-token deny tests plus placeholder/public-key allow tests. Gitignore
  assertions keep local VPS/operator credentials and independent checkouts
  excluded without hiding locks, examples, or the committed release public
  key.
- Fresh Hetzner install and failed-update rollback drills pass.
- TinyHost owns only its TCP 80/443 public listeners; an additional
  TinyHost-owned listener fails without attributing operator-owned services to
  TinyHost.
- Anonymous probes cover every registered protected surface.
- Binary/artifacts have checksums, signatures, and provenance.
- Release metadata, a changed binary, and a changed signature are each proven
  to fail verification before a release candidate is accepted.
- Known gaps are explicitly accepted and do not violate core principles.
