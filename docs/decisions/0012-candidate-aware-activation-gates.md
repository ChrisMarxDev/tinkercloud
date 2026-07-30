# ADR 0012: Candidate-aware immutable activation gates

**Status:** Accepted for activation; deployer rollback portion superseded by
[ADR 0039](0039-defer-deployer-release-rollback.md)

Activation gates inspect server-derived immutable candidate deployment records
before the current pointer changes. Client-supplied paths are forbidden; gate
denial preserves the previous active release.

The candidate's strict, server-canonical `tinker.yaml` private allowlist is part
of the immutable deployment metadata. It is not valid to approve a candidate
because a different, older app policy happens to exist.

## Decision

Activation creates a fresh private policy revision from the candidate manifest
and changes `current_deployment_id` in the same SQLite transaction. The
deployer owner remains server-derived and implicit in those policies.

Any candidate-policy validation, audit insertion, transition, or commit
failure rolls back the whole transaction, preserving the last active release
and policy pair. After a successful policy-bearing activation, the live hub
closes affected app sockets so a replaced policy cannot remain usable on an
existing connection. Deployer-selected release rollback is deferred by ADR
0039.
