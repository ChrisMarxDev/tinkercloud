# ADR 0030: Lightweight local blob storage in V1

**Status:** Accepted for V1

## Context

Useful internal apps often need attachments, images, exports, or small source
documents that do not fit the JSON KV capability. TinyHost already owns a
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
  stores directly.
- A standalone store such as
  [MinIO](https://min.io/docs/minio/linux/operations/install-deploy-manage/deploy-minio-single-node-multi-drive.html)
  adds another service, listener, credential, upgrade lifecycle, and resource
  budget. That is disproportionate for one small dedicated TinyHost VPS.

Mounting object storage does not remove TinyHost's need for server-derived
tenancy, SQLite metadata, quota accounting, staging visibility, cleanup, and
failure reconciliation. It instead hides weaker remote-object semantics behind
filesystem calls.

## Decision

V1 includes the lightweight blob capability defined by
[`specs/capabilities/blob-contract.md`](../../specs/capabilities/blob-contract.md).
It is part of M4 and is exposed only through the typed SDK and authenticated
gateway.

V1 stores blob bytes in a private local namespace inside the configured
TinyHost data directory. SQLite owns the canonical catalog, state, ordering,
and quotas. Only metadata in `ready` state is publicly readable after ordinary
app authorization.

The blob domain depends on a narrow internal streaming store interface keyed
only by server-derived app and blob IDs. It does not depend on POSIX paths,
rename, storage-native listing, signed URLs, or provider-specific attributes.
The local adapter may use private same-filesystem staging and atomic rename,
but those mechanics do not enter the domain or public contract.

TinyHost will not use Mountpoint, rclone mount, s3fs, another FUSE filesystem,
or a standalone S3-compatible server in V1. V1 also will not compile or
configure a remote provider driver.

Before implementation chooses a third-party storage dependency, a focused
dependency and failure-injection spike must compare a small native local
adapter with Go CDK `blob`/`fileblob`. Go CDK's portable API is a credible
future implementation option, but adopting it does not replace TinyHost's
SQLite state machine, reconciliation, or security tests. A security-critical
dependency must remain narrow and be justified by an ADR amendment.

A later direct S3-compatible adapter may implement the same internal interface
with object operations. It remains server-side, is not mounted, and introduces
no public bucket/object URL or browser credential.

## Consequences

- V1 gains small app attachments without another daemon, listener, mount,
  bucket, or provider account.
- The `tinyhost` binary, SQLite database, and private data directory remain the
  complete V1 operational shape.
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
that point compare a direct Go CDK/AWS SDK S3-compatible adapter against the
same contract. Do not introduce a FUSE mount merely to avoid implementing the
storage interface.
