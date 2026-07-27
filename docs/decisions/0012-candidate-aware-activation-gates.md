# ADR 0012: Candidate-aware immutable activation gates

**Status:** Accepted

Activation gates inspect server-derived immutable candidate deployment records
before the current pointer changes. Client-supplied paths are forbidden; gate
denial preserves the previous active release.

The candidate's strict, server-canonical `tiny.yaml` private allowlist is part
of the immutable deployment metadata. It is not valid to approve a candidate
because a different, older app policy happens to exist.

## Decision

Activation creates a fresh private policy revision from the candidate manifest
and changes `current_deployment_id` in the same SQLite transaction. Rollback
uses the selected immutable deployment's manifest to create its replacement
policy revision in the same transaction as its pointer change. The deployer
owner remains server-derived and implicit in all such policies.

Any candidate-policy validation, audit insertion, transition, or commit
failure rolls back the whole transaction, preserving the last active release
and policy pair. After a successful policy-bearing activation or rollback, the
live hub closes affected app sockets so a replaced policy cannot remain usable
on an existing connection.
