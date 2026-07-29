# ADR 0027: Immutable deployment descriptions and stable dashboard launch

**Status:** Accepted for descriptions/launch; deployer rollback portion
superseded by [ADR 0039](0039-defer-deployer-release-rollback.md)

## Context

Operators need a short human-oriented explanation of a deployed app and a
convenient dashboard launch action. Those conveniences must not create mutable
application metadata, expose release storage, or bypass the gateway.

## Decision

`tiny.yaml` accepts an optional bounded single-line `description`. Validation
trims edge whitespace, permits empty text, and rejects invalid UTF-8, controls,
Unicode line separators, and values longer than 280 Unicode code points. The
canonical value is persisted only in the existing immutable deployment
`manifest_json`; no database migration or application description column is
introduced.

Dashboard release rows read their own immutable manifest descriptions. The app
summary reads only the deployment selected by `current_deployment_id`, so
activation changes the matching description atomically with the existing
release pointer. Failed activation leaves the active summary intact.
Deployer-selected rollback is deferred by ADR 0039. Missing or malformed final
manifest data fails the dashboard read model closed; intermediate records may
have no description.

The dashboard may render a compact, accessible external link only for an active
current app at the server-derived stable `https://{slug}.{app_suffix}/` origin.
It opens in a new tab with `noopener noreferrer`. Immutable release hashes,
release paths, storage paths, and raw release URLs are never public interface
values.

## Consequences

The feature has no new storage authority and does not weaken authorization:
launching reaches the normal gateway and remains subject to app authentication
and policy. Historical releases retain truthful labels. Corrupt immutable
metadata causes an operationally visible unavailable state instead of an
unsafe or misleading empty UI.
