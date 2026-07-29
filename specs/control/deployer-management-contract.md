# Active deployer allowlist contract

Only an active operator may submit `ReplaceActiveDeployers(actor, emails, expected_revision, confirm_broadening, request_id)`. The dashboard renders the canonical sorted active-deployer snapshot and its server-derived revision in one ordinary textarea form.

The request accepts at most 100 unique normalized emails and 8192 bytes. Malformed or duplicate entries deny. A requested email that belongs to any operator denies. The transaction recomputes the active snapshot and denies a stale/missing revision. Adding a new or inactive deployer broadens deployment authority and requires `confirm_broadening`.

On success the requested set is the exact active deployer set: unknown addresses create active deployers; suspended/revoked deployers reactivate with the same immutable ID; omitted formerly-active deployers become revoked. Revocation immediately revokes all API tokens and control sessions and invalidates pending control OTPs. Reactivation invalidates pending control OTPs before the address becomes active. Apps, deployments, policies, access rules, IDs, and other ownership data never change.

The reconciliation, credential invalidation, and `deployers.reconciled` safe audit record occur in one transaction. The audit contains only requested normalized emails, revision, and broadening confirmation. Role, CSRF, origin, revision, validation, collision, confirmation, audit, idempotency, and persistence failures deny without mutation. Exact idempotent repeats succeed without another write; mismatched key reuse denies.
