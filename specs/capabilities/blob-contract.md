# V1 Lightweight Blob Capability Contract

This contract defines Tinkercloud's smallest useful file-storage capability. It is
an app-scoped utility store for replaceable internal apps, not a filesystem,
public asset host, backup system, or durable object-storage service.

## Authority and ownership

- Every operation requires the sealed authorization context produced by the
  gateway for the current app host and viewer session.
- App ID, viewer identity, policy revision, session, and storage namespace are
  server-derived. No request field, path, query, header, or SDK option may
  select them.
- V1 uses one shared blob namespace per app, matching V1 KV semantics. Every
  currently authorized viewer of an app whose blob capability is enabled may
  upload, read, list, and delete its blobs.
- A blob ID is opaque and server-issued. A client-supplied filename is bounded
  display metadata only and is never a storage key or filesystem component.
- Capability-disabled requests deny before touching metadata or byte storage.

## Public SDK surface

```ts
interface TinkerBlob {
  id: string;
  name: string;
  size: number;
  contentType: string;
  createdAt: string;
}

tinker.blobs.upload(file, { signal? }): Promise<TinkerBlob>
tinker.blobs.get(id, { signal? }): Promise<Blob | null>
tinker.blobs.list({ cursor?, limit?, signal? }): Promise<{
  blobs: TinkerBlob[];
  nextCursor?: string;
}>
tinker.blobs.delete(id, { signal? }): Promise<{ deleted: boolean }>
```

The SDK uses same-origin relative endpoints and the current app's host-only
session. It accepts no app ID, bucket, endpoint, path, storage key, signed URL,
or provider credential.

V1 intentionally has no replace, append, directories, public/signed URLs,
resumable or multi-request chunked upload, range API, inline hosting contract,
thumbnails, transformations, metadata search, deduplication, version history,
or per-viewer ACLs.

## Default limits

Recommended V1 defaults are:

| Limit | Default |
|---|---:|
| Bytes per blob | 25 MB |
| Blobs per app | 1,000 |
| Total blob bytes per app | 250 MB |
| List page | 100 |

Upload rate, metadata bytes, filename length, content-type length, request
duration, and concurrent app uploads also have finite server-configured bounds.
Operators may tighten limits only inside the server's validated ranges.

Blob growth uses the ordinary disk write gate. An unavailable disk measurement
or critical watermark denies upload. Safe authorized reads, delete/revocation,
and cleanup remain available when possible.

## Durable model

SQLite owns the canonical blob catalog and quota accounting. Byte storage is a
narrow internal adapter addressed only with a server-derived `(app ID, blob
ID)` key.

Conceptual metadata:

```text
app_blobs
  id, app_id, state(staging|ready|deleting), display_name, content_type,
  size_bytes, content_hash, created_by_identity_id, created_at, updated_at
```

Every lookup begins with app ID. List order and cursors come from SQLite rather
than storage-provider listing behavior. Only `ready` records are visible or
readable.

## Upload state machine

```text
absent
  → staging metadata with server-issued ID
  → bounded stream to private staging while hashing
  → close/sync byte storage successfully
  → commit exact size/hash and state=ready
```

- Staging bytes are unreachable from every public route.
- The final storage key is derived only from app ID and blob ID.
- A short read, cancellation, quota failure, storage failure, metadata failure,
  or process interruption never produces a readable blob.
- A byte object installed before the `ready` commit is an unreachable orphan;
  bounded reconciliation removes it.
- A `ready` metadata row whose bytes are missing or whose verified size/hash is
  inconsistent is unavailable and must not return partial or substitute bytes.

## Download behavior

- The gateway authorizes before metadata lookup or byte opening.
- Missing and unavailable blobs return a bounded generic response without
  revealing whether another app owns the guessed ID.
- Successful downloads use the recorded byte length and content type plus
  `Content-Disposition: attachment`, `X-Content-Type-Options: nosniff`, and
  private/no-store cache behavior.
- The response exposes no filesystem path, storage key, bucket, provider
  endpoint, redirect, or signed URL.
- Cancellation closes the byte reader promptly. A failed read terminates the
  response and is never retried with another app or path.

## List behavior

- Listing is cursor/limit bounded and returns an empty array, never `null`.
- The cursor is opaque or a validated stable SQLite ordering value. It cannot
  select an app or storage prefix.
- Only `ready` metadata for the authorization context's app appears.
- Storage-adapter listing is not a source of public results or authorization.

## Delete state machine

```text
ready
  → state=deleting
  → remove bytes by server-derived key
  → remove metadata/quota row
```

Once `deleting` commits, the blob is no longer readable. Missing bytes are a
retryable reconciliation condition, not permission to derive a different path.
An interrupted delete remains unavailable and cleanup retries it. Repeating a
successful or already-absent delete is a safe bounded no-op.

## Storage adapter boundary

V1 ships one private local-filesystem adapter inside the `tinkercloud` service and
data directory. The domain depends on a narrow streaming interface equivalent
to:

```text
Put(appID, blobID, reader, limit) → size, hash
Open(appID, blobID) → bounded reader
Delete(appID, blobID)
Stat(appID, blobID) → size, hash/evidence
```

The interface deliberately excludes paths, mounts, public URLs, provider
credentials, rename assumptions, and provider-native listing. This leaves a
future direct S3-compatible implementation possible without changing the
public SDK or treating object storage as POSIX.

FUSE mounts, rclone/s3fs/Mountpoint runtime dependencies, standalone
object-store servers, and remote storage drivers are not supported in V1.
The V1 local adapter uses the Go standard library and adds no process, service,
listener, mount, package, provider credential, or storage-network dependency.

## Deny-path charter

Before the positive path is complete, executable evidence must prove:

- anonymous, wrong-app, expired, revoked, policy-removed, capability-disabled,
  and suspended-app requests expose zero blob bytes and perform no mutation;
- a caller cannot select another app through host, path, query, ID, filename,
  content type, cursor, body, or header input;
- missing/wrong Origin, cross-origin cookie mutation, malformed ID/cursor/
  metadata, empty/oversized body, declared/actual-size mismatch, excessive
  count/bytes/rate, timeout, and cancellation create no `ready` blob;
- disk watermark/source failure, short write, close/sync/rename failure,
  SQLite busy/commit failure, and interruption at every state transition leave
  no readable partial blob and preserve exact quota accounting after
  reconciliation;
- guessed cross-app IDs, metadata rows with absent/corrupt bytes, orphan bytes,
  and storage/metadata disagreement fail closed without path, key, provider,
  SQL, or policy disclosure;
- download responses cannot execute uploaded HTML/SVG as a Tinkercloud app-origin
  document through the supported API;
- delete and cleanup never accept or follow a client path, symlink, hard link,
  special file, or storage key; and
- app deletion/revocation makes affected blob operations unavailable on the
  next relevant request without exposing another app's namespace.

## Recovery and durability statement

Startup and bounded cleanup reconcile non-ready rows and server-derived storage
objects without scanning or deleting outside the configured blob namespace.
Uncertain state remains unavailable until reconciled.

V1 blobs are utility-grade local VPS data. Losing the VPS or data volume can
lose blobs. Tinkercloud V1 provides no backup, replication, remote object-store,
availability, or business-critical durability guarantee.
