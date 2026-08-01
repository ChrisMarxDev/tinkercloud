# Public Reach and Local Insights Acceptance Evidence

Status: accepted on 2026-08-01

This record closes the acceptance list in
[`concept/features/public-reach-and-local-insights.html`](../../concept/features/public-reach-and-local-insights.html).
It contains no OTP, provider response, credential, application content, raw
analytics identifier, or VPS filesystem/database inspection. The live proof
used only public gateway requests, the supported CLI/control surfaces, strict
SSH host verification for installation and root-local operator commands, and
the fixed acceptance identities and hosts defined by the
[`VPS E2E contract`](../../specs/operations/vps-e2e-contract.md).

## Accepted build and environment

Public hostnames and local report names in this record are deliberately
replaced with reserved placeholders. The operator retains the exact redacted
evidence outside the repository.

- One fresh clean-host run with the strengthened catalog, policy-revocation,
  activation-failure, and dashboard-series assertions completed against the disposable
  `testing.tinkercloud.example` environment on 2026-08-01.
- `admin.testing.tinkercloud.example` and the stable one-label fixture app hosts
  resolved to the acknowledged VPS before the run.
- The reset removed only the exact marked Tinkercloud installation and test
  state. The existing `/var/lib/tinkercloud-acme` cache remained a real,
  non-symlink directory before and after the reset.
- The checked-in unattended wrapper reported `preflight`, local OTP-reader
  tests, skill drift, the offline VPS package, diff validation, and the live
  VPS acceptance as passed. The owner-only redacted result is
  `.tinker/vps/unattended-reports/vps-e2e-<timestamp>-<run>.status`.
- The complete local `go test ./... -count=1` suite, JavaScript syntax check,
  skill-drift check, OTP-reader tests, and `git diff --check` passed before the
  live run.
- The guarded deployment-agent wrapper then performed exactly one explicit
  `--confirm-public` deployment of the tracked first-party
  `examples/public-static-product-story` as `dev@christopher-marx.de`. It used
  the normal OTP flow and returned the expected public URL under the reserved
  `testing.tinkercloud.example` placeholder used in this record.
- One separate verification-only `TestPublicExampleAcceptance` pass compared
  the public HTML and CSS byte-for-byte with the tracked example, proved opt-in
  indexing and reserved-route denial, made two document requests in one fresh
  app-host cookie jar, and observed exactly two page views, one approximate
  visitor, nonempty last activity, and 30 UTC daily rows in the owner's normal
  OTP-authenticated dashboard. The example was left live.

## Requirement evidence

1. **One gateway and operator control.** `tinkercloud public enable|disable`
   changed the durable, revisioned default-off gate without adding a listener,
   proxy, daemon, provider, or runtime. The live socket inventory stayed at the
   exact public HTTP/HTTPS listeners owned by the single Tinkercloud service.
2. **Deliberate capability-free publication.** The live matrix established a
   private baseline, proved gate-off, unacknowledged, malformed, and
   capability-bearing public candidates could not replace it, then activated
   the acknowledged public candidate only after the public gateway proof.
3. **Anonymous bytes without anonymous capabilities.** A fresh anonymous
   client received the exact public HTML and asset plus the selected indexing
   behavior. Representative auth, identity, SDK/KV, collection, blob,
   live/WebSocket, and LLM routes all denied without either release marker.
4. **Immediate transitions with owner continuity.** Disabling the operator gate
   denied the next anonymous document and asset request. The already verified
   owner then completed a fresh exact app handoff and received only that app's
   host-only session. Re-enabling the gate restored public static access, and a
   later public-to-private activation denied the next anonymous request.
5. **Authorized searchable catalog.** The verified viewer catalog explicitly
   contained the allowed private primary card and both effective-public cards,
   while omitting the owner-only denied fixture and all release markers. A
   supported revisioned policy change removed the primary card and denied the
   existing app session on the next read/request; restoring that policy made
   the same still-valid host-only session usable only after current-policy
   re-evaluation. Two owners' catalog/control/dashboard views remained
   isolated, while the operator view intentionally contained both owners.
   Local search and canonical tag filtering were exercised only over the
   already-authorized rendered cards by the web regression suite.
6. **Bounded local insights.** Two successful top-level HTML requests through
   one app-host cookie jar produced exactly two page views and one approximate
   visitor in the owner/operator read model. Both the clean matrix fixture and
   the deployed first-party example required nonempty last activity and exactly
   30 daily rows. The clean matrix retained public bytes and aggregate state
   across service restart and did not expose the aggregate to the unrelated
   owner.
7. **Privacy, retention, deletion, and availability.** Repository and gateway
   tests prove the marker is random, host-only, app-scoped through a keyed
   digest, retained for at most 30 days, and cascades on hard app deletion.
   They also prove assets, APIs, ranges, errors, redirects, non-document
   requests, and conditional responses do not count; full/unavailable/write-
   failing analytics state neither blocks nor changes authorized static bytes.
8. **Required failure matrix.** The production-composition real-listener test,
   persistence tests, and clean black-box VPS run jointly covered two apps/two
   owners, anonymous access, current-policy revocation, denied identity,
   malformed policy/gate/manifest input, targeted analytics database failure,
   bounded retention and hard-delete cascade, restart persistence, failed
   candidate validation/activation with previous-release preservation,
   operator-gate transitions, public/private transitions, callback replay,
   sibling-host targeting, app-local logout, account switching, and global
   logout with child-session revocation. Physical retention/purge and targeted
   SQLite failure are intentionally proven in the production-composition and
   persistence layers: the clean VPS suite does not inspect or mutate its
   SQLite/filesystem/logs or add a production test hook.

## Authoritative executable coverage

- `TestPublicReachProductionCompositionLocalAcceptance` is the real-listener
  production-composition matrix, including public posture verification,
  reserved-route denial, catalog filtering, exact insights, injected database
  failure, gate transitions, owner handoff, and hard deletion.
- `TestVPSAcceptance` is the opt-in black-box clean-host matrix and obtains real
  login codes through the fail-closed local Resend reader; it contains no auth
  bypass and never substitutes VPS SQLite/filesystem/log inspection for public
  gateway evidence.
- `TestPublicExampleAcceptance` is the separate verification-only release
  proof. It performs no install, deploy, clean, SSH, retry, database inspection,
  or deletion and leaves the first-party public example available.
- Persistence tests cover revisioned public-gate failure, atomic activation and
  policy transitions, current-policy catalog filtering, owner-scoped insights,
  30-day cleanup, hard deletion, and previous-release preservation.
- Gateway, analytics, release-manifest, CLI, web, SDK contract, and integration
  tests cover the sealed static decision, app-scoped digest/cookie behavior,
  bounded manifest metadata, explicit broadening acknowledgement, native UI,
  and capability isolation required by ADR 0054 and the deny charter.

The evidence proves the accepted public-static reach loop. It does not claim a
public backend runtime, anonymous Tinkercloud capabilities, custom analytics
events, unique people, custom domains, or any other concept non-goal.
