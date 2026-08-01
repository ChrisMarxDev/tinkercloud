# Public reach and local insights denial charter

Happy-path evidence is incomplete until these denials pass through repository,
service, gateway, native UI/control API, and real-listener integration layers as
applicable.

## Public static

- Operator gate off, missing, corrupt, stale, or unavailable exposes zero
  anonymous release bytes.
- Private, unknown, suspended, deleting, failed, corrupt, wrong-host, and
  cross-app requests expose zero anonymous bytes.
- A public candidate with KV, collections, blobs, realtime, LLM, or any future
  browser capability enabled cannot verify or activate.
- Every `/_tinker/*` route on a public app denies before an auth, SDK,
  repository, WebSocket, blob store, or provider dispatcher is touched.
- Public-to-private and global gate-disable transitions deny the next anonymous
  request and cannot be extended by cache state.
- Malformed/unknown policy modes and untyped public booleans cannot create a
  static authorization context.
- Public candidate proof fails on redirects, wrong host/scheme, unexpected
  bytes, missing assets, mismatched indexing headers, or any usable reserved
  route and preserves the previous active release.
- The real-listener and VPS checks exercise representative `/_tinker` auth,
  identity, SDK/capability, KV, collection, blob, live/WebSocket, and LLM
  paths, and each denial contains neither the public document nor an asset
  marker. A public cookie must not turn any reserved route into an allow.

## Catalog

- Anonymous, expired, revoked, malformed, or unavailable global identity state
  returns no private app metadata.
- App A policy never reveals App B metadata; policy revocation removes a card
  on the next server read.
- Browser search and tag filtering operate only on already-authorized rendered
  cards and cannot request, reconstruct, or reveal denied records. Search may
  inspect only the rendered slug, active description, and tags; tag selection
  matches only rendered canonical tags exactly.
- Invalid, duplicate, excessive, non-lowercase, or boundary-hyphen tags reject
  the manifest before activation.

## Insights

- App A owner cannot read App B insights. Viewers, public visitors, SDK/app
  sessions, deployment agents, revoked users, and wrong-scope bearers cannot
  read any aggregate.
- Asset, API, range, WebSocket, probe, error, redirect, non-GET, non-HTML, and
  unsuccessful requests do not increment counters.
- Malformed/oversized cookies, dates, ranges, slugs, and digests fail within
  bounds and store no marker.
- Injected analytics database/write/cleanup failure does not alter an otherwise
  authorized static response's status or body.
- Database, logs, audit, and HTTP evidence contain no raw analytics cookie,
  session, identity, email, IP, URL/query/path, referrer, user agent, content,
  geography, or device data.
- Retention cleanup removes data older than 30 days, and hard app deletion
  leaves no aggregate or marker for that app.

## Required matrices

Run two-app/two-owner, anonymous, revoked, malformed, database-failure,
restart, activation-failure, public/private transition, and operator-gate
transition matrices on a real listener and clean VPS before general release.

For insights, the matrix also proves two successful top-level HTML requests in
one app-host cookie jar produce two page views but one approximate visitor in
the owner/operator read model; a second app or owner cannot observe those
aggregates.
