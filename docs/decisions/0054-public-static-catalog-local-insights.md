# ADR 0054: Operator-gated public static access, authorized catalog, and local insights

## Status

Accepted — 2026-08-01

## Context

Tinkercloud V1 is private-only. The accepted post-V1 reach loop must let teams
find apps, deliberately publish reviewed static work, and learn whether it was
visited without adding a second listener, analytics provider, backend runtime,
anonymous capability surface, or tracking profile.

Jot validates the product loop of named public slugs, opt-in indexing,
searchable metadata, and local usage counters. Tinkercloud retains its stricter
trust model: public reach cannot inherit app capabilities or identity authority.

## Decision

Add three surfaces behind the existing gateway and control SQLite database:

1. A verified-identity team catalog filtered by current policy before render.
2. Bounded local page-view and approximate-browser aggregates retained for 30
   days and readable only by the app owner or operator.
3. Public static access requiring both an explicit app policy and a durable
   revisioned operator gate that defaults off.

The gateway produces a sealed static-access sum type with private-viewer and
public-static variants. Static serving accepts either variant. Every reserved
or capability dispatcher continues to require the existing viewer-bearing
authorization context; an untyped `public=true` or `authorized=true` is
forbidden.

Public candidates must have no browser capability enabled. Activation is
posture-aware: private candidates prove anonymous denial with zero release
bytes; public candidates prove the expected immutable candidate bytes are
anonymously reachable while reserved routes remain denied. Gate or policy
changes affect the next request. Public indexing is a separate immutable
manifest opt-in and defaults off.

Manifest version 1 keeps its exact private-only meaning. The extension adds
version 2 for bounded tags, `access.indexing`, and explicit public mode; the
post-V1 generator emits version 2 while the server continues to accept valid
private version-1 receipts. This prevents an old strict receipt or client from
silently acquiring broader semantics.

Analytics counts only successful top-level HTML GET or validated SPA fallback.
The request path offers records to one fixed-capacity local queue and drops on
full or unavailable state rather than blocking an app response. An app-origin
random 30-day cookie returned by the browser is transformed into an app-scoped
keyed digest. Only the digest marker and daily aggregates are stored. Analytics
does not retain the raw cookie, identity/session, IP, URL, query, referrer, user
agent, content, geography, or device data. Analytics failure is best-effort and
cannot alter authorization, bytes, or availability.

## Consequences

- The public internet can read reviewed immutable app files only after two
  current authorization decisions; it gains no platform capability authority.
- Owner/email/domain rules remain useful for private access when the operator
  gate is off or the app returns to private.
- Catalog queries must authorize in SQL/service code before rendering; browser
  filtering is presentation only.
- Public document cache and indexing directives depend on current policy/gate
  state and therefore cannot extend access after revocation.
- Hard app deletion must cascade analytics state.
- This is an explicit post-V1 extension and does not rewrite the V1 historical
  exit claim.
