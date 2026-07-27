# ADR 0029: Candidate doctor tolerates only its expected rollback snapshot

**Status:** Accepted for V1

## Context

The signed updater preserves `data_directory/update-rollback/previous` until
the replacement binary has restarted and all health gates pass. Ordinary
`tinyhost doctor` correctly reports any remaining snapshot as degraded so an
operator can recover an interrupted update. Using that unchanged doctor as the
candidate's local health gate created a deadlock: every candidate necessarily
saw its own rollback snapshot and rolled itself back.

## Decision

The updater invokes the replacement binary's doctor in an internal
candidate-health mode. That mode executes the complete doctor suite and
changes only the rollback-snapshot interpretation: the canonical private,
regular, non-symlinked `update-rollback/previous` artifact is expected until
the same updater transaction commits it away.

Normal `status` and normal operator `doctor` retain their degraded
`rollback pending` result. Candidate-health mode does not accept a missing,
unsafe, malformed, symlinked, or group/world-accessible snapshot, and it does
not suppress any other local, network, public-health, or anonymous-denial
failure.

## Consequences

Candidates can prove the full intended health set before commit while the
previous binary remains recoverable. A failed candidate still restores and
restarts the known-good binary. The narrowly scoped exception preserves the
operator-visible signal for genuinely pending rollback recovery and creates no
new listener, remote recovery surface, or client-controlled health input.
