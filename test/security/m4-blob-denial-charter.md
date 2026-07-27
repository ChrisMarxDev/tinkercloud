# M4 Lightweight Blob Denial Charter

This charter is written before the positive implementation. A successful
upload/download demo is not completion evidence.

Normative behavior lives in
[`specs/capabilities/blob-contract.md`](../../specs/capabilities/blob-contract.md).

## Authorization variants

Run upload, get, list, and delete through the real composed gateway with:

- no session;
- malformed, expired, and revoked sessions;
- an App A session against App B host and guessed App B blob ID;
- a viewer removed from the current policy;
- a suspended app;
- a valid app whose blob capability is disabled; and
- a valid current viewer of an enabled app.

Every denied read returns zero blob bytes. Every denied mutation creates no
ready metadata, stored object, quota change, or existence/timing oracle for
another app.

## Untrusted input

Reject before ready state:

- caller-supplied app/viewer/storage IDs in any path, query, header, or body;
- empty, malformed, oversized, path-like, traversal, absolute, Windows-drive,
  separator, NUL, control, invalid UTF-8, and confusable blob IDs/cursors;
- display filenames containing traversal, separators, controls, invalid UTF-8,
  excessive bytes/code points, or header-injection material;
- missing, empty, malformed, or excessive content type;
- empty or oversized bodies and declared/actual size disagreement;
- list limit below one, above the maximum, malformed cursor, and extra query
  fields;
- mutation without exact same-origin Origin, with a foreign/null Origin, or
  with an app/control credential of the wrong type; and
- unsupported methods, range requests, replace/append semantics, signed/public
  URL requests, and provider-native parameters.

Display names never become paths or storage keys, even after normalization.

## Quota and pressure

Exercise exact boundaries for:

- maximum blob bytes and one byte over;
- maximum app object count and one object over;
- maximum app total bytes and one byte over;
- concurrent uploads racing for the final remaining quota;
- configured upload rate/concurrency/duration limits;
- warning and critical disk watermarks; and
- unavailable or malformed disk measurements.

Quota reservation/commit/release must be exact after failure and restart. Disk
pressure denies growth but does not prevent revocation, safe ready reads,
deleting state, or bounded reconciliation.

## Failure injection and interruption

Inject failure or cancellation:

1. before staging metadata;
2. after staging metadata but before any byte;
3. during every bounded stream chunk;
4. after final byte but before close;
5. at flush/sync/close;
6. during same-filesystem local finalization;
7. after bytes are installed but before ready metadata commit;
8. during exact size/hash/quota commit;
9. after ready commit but before response;
10. after deleting-state commit but before byte deletion;
11. after byte deletion but before metadata/quota deletion; and
12. during orphan/staging/deleting reconciliation.

Inject SQLite busy/read/write/commit failure, read-only/permission errors,
short write/read, missing directory, full disk, corrupt/truncated bytes,
size/hash mismatch, missing ready bytes, orphan bytes, symlink/hard-link/special
file substitution, and process restart at each point.

Only ready metadata with matching byte evidence may stream. Uncertain state
remains unavailable; recovery never guesses a client path or marks a partial
object ready.

## Download safety

For HTML, SVG, XML, JavaScript, unknown, and misleading content types prove:

- direct supported downloads are attachment-oriented and carry `nosniff` plus
  private/no-store behavior;
- filenames cannot inject response headers or unsafe disposition parameters;
- an error after streaming begins cannot switch to another object or app;
- cancellation closes the reader promptly; and
- no response reveals a local path, object key, bucket, endpoint, credential,
  SQLite detail, policy membership, or stack trace.

## Listing and storage disagreement

Prove public listing comes only from bounded ready SQLite metadata:

- storage-native directory/provider listing is never returned;
- staging, deleting, orphan, foreign-app, and malformed rows remain absent;
- an empty page is `[]`, not `null`;
- cursors cannot skip into another app or select a storage prefix; and
- stale/missing/corrupt byte evidence makes a selected ready record unavailable
  rather than returning substitute data.

## App lifecycle

App policy/session revocation and suspension deny the next blob request. App
deletion makes blob operations unavailable before cleanup and eventually
removes only server-derived blob rows/objects for that app. Re-authorization or
app recreation never resurrects deleted blob authority or old credentials.

## Required evidence

Completion requires:

- service/repository/storage unit tests;
- migration fresh-install and upgrade tests;
- a real-gateway two-app HTTP matrix;
- built-SDK upload/get/list/delete contract tests;
- race tests for quota and delete/upload concurrency;
- restart and filesystem/SQLite failure injection; and
- agent-skill and manifest drift checks.
