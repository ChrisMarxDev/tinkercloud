# ADR 0030: Lightweight local blob storage in V1

**Status:** Accepted for V1

## Context

Useful internal apps often need attachments, images, exports, or small source
documents that do not fit the JSON KV capability. Tinkercloud already owns a
private data directory, server-derived app authorization, bounded streaming
upload primitives, SQLite quota state, and a disk write-stop gate. A narrow
blob capability can reuse those foundations without adding backend execution.

The original V1 scope deferred all blobs. The accepted smaller scope is one
app-shared, local-disk-backed capability with upload, download, list, metadata,
and delete. It does not promise public hosting, provider durability, or a
general filesystem.

Several existing storage tools were evaluated:

- [Mountpoint for Amazon S3](https://docs.aws.amazon.com/AmazonS3/latest/userguide/mountpoint.html)
  presents S3 through a filesystem interface, but intentionally lacks full
  POSIX behavior, including modification of existing files, symlinks, and file
  locking.
- [rclone mount](https://rclone.org/commands/rclone_mount/) supports many
  providers through FUSE/VFS, but normal filesystem compatibility depends on a
  local write-back cache and a separately supervised mount process. Cache
  timing, restart, overlapping instances, and host FUSE/AppArmor configuration
  become additional failure boundaries.
- [s3fs](https://github.com/s3fs-fuse/s3fs-fuse#limitations) offers broader
  filesystem emulation, but documents no atomic rename, no coordination between
  multiple mounts, whole-object rewrites, and potentially stale listings.
- [Go CDK blob](https://gocloud.dev/howto/blob/) provides a portable streaming
  object API with local-filesystem and S3-compatible drivers. Its
  [fileblob](https://pkg.go.dev/gocloud.dev/blob/fileblob) adapter uses a
  temporary file plus rename, while its S3 driver can address S3-compatible
  stores directly. However, the Go CDK documentation positions local drivers
  primarily for testing and local development, the generic blob layer imports
  OpenTelemetry, and `fileblob` owns filename escaping and optional sidecar
  metadata that Tinkercloud does not need.
- A standalone store such as
  [MinIO](https://min.io/docs/minio/linux/operations/install-deploy-manage/deploy-minio-single-node-multi-drive.html)
  adds another service, listener, credential, upgrade lifecycle, and resource
  budget. That is disproportionate for one small dedicated Tinkercloud VPS.

Mounting object storage does not remove Tinkercloud's need for server-derived
tenancy, SQLite metadata, quota accounting, staging visibility, cleanup, and
failure reconciliation. It instead hides weaker remote-object semantics behind
filesystem calls.

### Lightweight acceptance gate

For Tinkercloud, a storage implementation is lightweight only when all of these
remain true:

- production still consists of one `tinkercloud` process, one embedded SQLite
  engine with a control database plus isolated app-local data files, one private
  data directory, and one systemd service;
- installation requires no FUSE/kernel extension, mount unit, extra package,
  daemon, listener, provider account, storage credential, or separate health,
  upgrade, and restart lifecycle;
- disabling blobs starts no storage process and performs no storage-network
  activity;
- upload and download memory remain bounded independently of blob size;
- SQLite remains the only catalog, list, readiness, ordering, and quota source
  of truth; and
- a dependency is accepted only when it materially reduces Tinkercloud-owned
  security or recovery code, with its module graph, binary-size delta, idle
  memory delta, failure modes, maintenance state, and license recorded in an
  ADR amendment.

Mountpoint, rclone mount, s3fs, and MinIO fail the production-shape and operator
setup portions of this gate. Go CDK passes the no-extra-process portion, but
does not remove Tinkercloud's state machine and adds a generic dependency layer.

Measurement baseline on 2026-07-27: with Go 1.25.12,
`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath ./cmd/tinkercloud`
produced a 21,982,869-byte server binary before blob implementation. The blob
slice records its before/after result with the same command plus idle RSS on
the smallest supported VPS. This measurement is evidence, not a permanent byte
cap across compiler upgrades.

## Decision

V1 includes the lightweight blob capability defined by
[`specs/capabilities/blob-contract.md`](../../specs/capabilities/blob-contract.md).
It is part of M4 and is exposed only through the typed SDK and authenticated
gateway.

V1 stores blob bytes in a private local namespace inside the configured
Tinkercloud data directory. SQLite owns the canonical catalog, state, ordering,
and quotas. Only metadata in `ready` state is publicly readable after ordinary
app authorization.

The blob domain depends on a narrow internal streaming store interface keyed
only by server-derived app and blob IDs. It does not depend on POSIX paths,
rename, storage-native listing, signed URLs, or provider-specific attributes.
The V1 local adapter is implemented inside Tinkercloud with the Go standard
library. It uses private same-filesystem temporary files, bounded streaming,
hashing, sync/close, and atomic rename beneath the existing data directory.
Those mechanics do not enter the domain or public contract.

Tinkercloud will not use Mountpoint, rclone mount, s3fs, another FUSE filesystem,
or a standalone S3-compatible server in V1. V1 also will not compile or
configure a remote provider driver. It will not depend on Go CDK for the V1
local adapter.

A later direct S3-compatible adapter may implement the same internal interface
with object operations. It remains server-side, is not mounted, and introduces
no public bucket/object URL or browser credential. Go CDK may be reconsidered
for that later adapter, but only through the lightweight acceptance gate above.

## Consequences

- V1 gains small app attachments without another daemon, listener, mount,
  bucket, or provider account.
- The `tinkercloud` binary, SQLite database, and private data directory remain the
  complete V1 operational shape.
- The native adapter is deliberately small and uses no new Go module. Tinkercloud
  owns the few filesystem operations and their failure-injection tests.
- The public SDK remains stable if a later operator selects a direct remote
  adapter because storage-native identities never cross the domain boundary.
- App viewers have the same app-shared mutation authority as V1 KV. Apps
  requiring per-viewer file ACLs need a later capability model.
- Blob bytes consume the same VPS volume and disk-watermark budget as releases
  and KV. Operators receive no backup or durability claim.
- App deletion and cleanup must include blob metadata and bytes through the
  server-derived lifecycle; raw recursive client paths remain forbidden.
- The V1 exit gate expands to include wrong-app, revoked, partial-write,
  quota/disk, metadata/storage disagreement, cleanup, and SDK contract evidence.

## Reconsider when

Reconsider the local-only adapter after real usage shows that blob capacity,
durability, or migration—not hypothetical scale—is the limiting factor. At
that point compare a small direct S3-compatible adapter, Go CDK, and the AWS SDK
against the same contract and lightweight gate. Do not introduce a FUSE mount
merely to avoid implementing the storage interface.
