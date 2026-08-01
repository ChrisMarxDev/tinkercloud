# 0039: Defer deployer-selected release rollback

**Status:** Accepted post-V1

## Context

Tinkercloud already preserves the last known-good deployment when staging or
activation fails. Exposing selection of an older release to deployers adds a
second deployment mutation, policy transition, audit surface, and dashboard
interaction before V1 needs it.

## Decision

Deployer-initiated application rollback is deferred beyond V1. The `tinker`
CLI, control bearer API, dashboard form/action, event catalog, and current V1
documentation expose no rollback operation. Unknown former paths deny as
ordinary absent routes. The internal immutable release and activation
components may retain their recovery seams; they are not a deployer authority.

## Consequences

V1 deployers create and activate new immutable releases, manage access and
tokens, and delete their own apps. A failed candidate preserves the existing
active release. Signed server-update rollback and root-only VPS recovery remain
in scope and are unrelated to this deferred app-control feature. A later
rollback design must introduce a new contract, authorization, policy/evidence
requirements, UI, audit semantics, and negative test charter before exposure.
