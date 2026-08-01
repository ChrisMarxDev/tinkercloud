# ADR 0023: Optimistic concurrency for access-policy replacement

**Status:** Accepted for V1; deployer rollback references superseded by
[ADR 0039](0039-defer-deployer-release-rollback.md)

## Context

The dashboard renders a policy revision, but policy replacement previously did
not bind a mutation to that revision. A deployment activation or another policy
edit could therefore change the active immutable/current policy
between page render and submission, then be silently overwritten.

## Decision

Every access-policy replacement includes a positive server-rendered
`expected_revision`. The control service compares it transactionally with the
owned app's current revision before it creates a new private revision, audit
event, or live revocation. A mismatch is a conflict with no mutation.

The service also identifies additions against that current policy. Adding an
email or domain requires explicit `confirm_broadening`; reductions and exact
replacements do not. The owner remains implicit and cannot be removed.

## Consequences

- Dashboard and API clients must refresh and retry after a conflict.
- Deployment activation retains its authority to atomically install the
  selected immutable manifest policy. Deployer-selected rollback is deferred
  by ADR 0039.
- A confirmed broadening is explicit in both the request and dashboard result;
  no client-supplied app or owner identity is introduced.
