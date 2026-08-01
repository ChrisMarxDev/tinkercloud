---
name: tinkercloud-full-stack-test
description: Run and maintain Tinkercloud's opt-in unattended full-stack VPS acceptance workflow, including local Resend OTP retrieval, SSH trust, deployment, viewer authorization, and negative security evidence. Use when configuring, debugging, or executing real end-to-end Tinkercloud tests against a disposable VPS.
---

<!-- shared:role-common:start -->
## Platform boundary

Use `operator` for the person who hosts Tinkercloud, `deployer` for a person
authorized to create and manage their own apps, and `viewer` for a person who
accesses an app. A deployment agent is automation acting through deployer
authority. Never transfer credentials or authority between these roles.

Tinkercloud V1 hosts private static apps. The gateway owns TLS, app routing,
viewer authentication, access policy, static files, SDK capabilities, and
deployment activation. It derives the app from the hostname and the viewer from
an opaque app-host session. App code must never select either identity.

Fail closed. Missing, stale, malformed, redirected, incompatible, or ambiguous
security state is a denial, not a value to guess. Never expose or request a
deployer token, OTP, provider secret, database credential, app ID, or viewer ID
in chat, argv, source code, `tinker.yaml`, browser storage, logs, or output.

V1 is private-only. The app owner is always an implicit viewer. There is no
public mode. Each app has its own private SQLite data file for KV and bounded
JSON document collections; blobs and that data are utility-grade state on one
VPS, so losing the VPS can lose them. Realtime is app-scoped, in-memory,
best-effort notification with no history, replay, ordering, or delivery
guarantee. Live collection events are freshness hints: recover current state
with a snapshot after first connect, reconnect, visibility recovery, and every
hint.

`llm.chat`, when an operator grants it, is a narrow server-side capability.
Capability discovery is absent until the app requests it and the operator grant
is active. If present, treat its disclosure as a notice that prompt content is
sent to an operator-selected external AI provider; discovery limits are safe
current bounds, not a promise that a later request will be admitted.
Provider credentials, connection IDs, model names, and upstream URLs never
enter app code, browser storage, the deployer manifest, or the SDK request.

Start from the requested outcome. Inspect trusted local and server state, reuse
verified values, and choose a secure default before asking anything. Ask only
for a required value or decision that is still unknown, cannot be discovered,
and cannot be defaulted safely. Group unresolved optional choices into one
review. Never infer broader authority.

Do not report success from a happy path alone. A deployment is complete only
after a fresh anonymous request through the real HTTPS gateway proves that no
app content is exposed.
<!-- shared:role-common:end -->

## Unattended VPS workflow

1. Read `specs/operations/vps-e2e-contract.md` and
   `test/security/unattended-vps-otp-reader-denial-charter.md` first.
2. Run offline gates before connecting anywhere:

   ```bash
   PYTHONDONTWRITEBYTECODE=1 python3 skills/tinkercloud-full-stack-test/scripts/test_read_resend_otp.py
   ./scripts/check-skill-drift
   go test ./test/vps -count=1
   git diff --check
   ```

3. Keep `TINKERCLOUD_RESEND_READER_API_KEY_FILE` local, absolute, regular,
   non-symlink, owned by the invoking user, and mode `0600`. Keep the
   consumed-message ledger local and mode `0600`. Never copy either to the VPS
   or print its contents.
4. Set `TINKERCLOUD_VPS_OTP_COMMAND` to the absolute path of
   `scripts/read-resend-otp.py`. Supply one `TINKERCLOUD_VPS_DOMAIN` and the
   sender values; the runner derives `admin.<domain>` and `<slug>.<domain>`.
   The reader receives only `deployer|viewer EMAIL HOST`; both V1 OTP purposes
   must use exactly `admin.<domain>` because the global browser identity owner
   owns the flow. Local automation accepts a recipient only when its normalized
   domain is exactly `christopher-marx.de`; this is not a production login
   restriction. It must print only a 4--12 digit code.
5. Require `TINKERCLOUD_VPS_E2E=1`, the exact target acknowledgement, a checked
   known-hosts file, and normal `TINKERCLOUD_VPS_REUSE=1` marker gating. Never
   weaken SSH trust or introduce an OTP/auth bypass. A clean rerun may remove
   Tinkercloud configuration, app data, binary, and service state, but preserve
   the fixed `/var/lib/tinkercloud-acme` transport cache. Initialization must
   still validate its path; do not erase reusable ACME account/certificate
   state merely to repeat app/auth testing. In the real browser-jar
   proof, read exactly one initial viewer OTP at `admin.<domain>/login`; the
   first and second allowed apps must receive separate host-only app sessions
   without another OTP, while an excluded app renders a generic no-app-bytes/
   no-OTP denial. Assert the global identity and the non-authorizing browser-
   binding cookies are dashboard-only and app session cookies are distinct per
   app host. Preserve a path with repeated and percent-encoded query values
   through a handoff. The binding must exist after dashboard sign-in, never
   appear on an app host, and remain after an account switch. Replay a consumed
   callback and target it at a sibling app host as denials; app-local logout
   must preserve global identity and sibling access, while global dashboard
   logout must revoke the identity and every child session. A later account
   switch uses its intentionally separate viewer-purpose OTP and revokes all
   old child sessions.
   For one app, redeploy the exact same immutable archive before viewer login.
   Require the later deployment to activate, supersede the earlier release,
   retain its canonical private policy, and pass anonymous denial again. A
   After deployer login, list only that deployer's apps and delete only the
   listed fixed acceptance fixtures (`vps-e2e-update-probe`,
   `vps-e2e-primary`, `vps-e2e-isolation`, and `vps-e2e-denied`), using a
   fresh idempotency key per deletion. Do not delete unknown, absent, or
   unrelated apps; list and deletion errors are terminal. Their stable hosts
   are intentional and must be reused across clean and reuse runs so the
   exact-host certificates are issued once and retained. Keep the per-archive
   marker randomized to prevent stale-content evidence. The
   certificate-readiness check uses only the non-mutating protected
   `/_tinker/api/v1/app` endpoint; it must never start an app login handoff.
6. Treat blob evidence as a complete capability sequence, not just a 2xx:
   discover the enabled capability, reject a multipart request with an extra
   part and prove no catalog mutation, then authenticate a viewer and prove
   upload/list/exact-byte attachment download/delete. Prove anonymous and a
   second app's guessed-ID download contain no blob bytes. Restart the service
   before delete and reread the same blob through a bounded,
   context-cancellable readiness retry. Retry only connection-startup transport
   failures or 502/503/504; a redirect, denial, other status, wrong bytes, or
   wrong attachment/private-no-store/nosniff headers fails immediately. Never
   query the VPS filesystem, SQLite, or logs to substitute for this gateway
   evidence.
7. Treat document collections as the same kind of gateway evidence: deploy a
   fixture with `features.kv` and `features.realtime`, discover `db`, then
   create/get/optimistic-update/list with `snapshot=1`/delete one document at
   `/_tinker/api/v1/db/{collection}`. The request must contain no app, database,
   or viewer selector. Prove anonymous denial with no document bytes, guessed
   cross-app ID denial, and the updated document after the bounded service
   restart readiness proof. A fixture with no LLM manifest request/grant must
   omit `llm.chat` from discovery and deny direct chat invocation without
   provider details. Do not substitute a VPS database/filesystem inspection.
8. Reuse never re-initializes or wipes the VPS. The supported signed update
   preserves the configured deployer, owned apps, and current private
   access-policy revision/rules; verify the post-update deployer can still
   read the owned app/policy before treating the update as accepted. It requires
   an explicit local `TINKERCLOUD_VPS_RELEASE_DIR`; verify it through the installed server's pinned
   key and use only `tinkercloud update` with its active-app health gate. The
   unattended wrapper fails before offline gates unless this is an absolute,
   existing, caller-owned non-symlink directory. After the trusted update,
   require `tinkercloud doctor`'s current-version check before root-only
   `tinkercloud llm enable`, then restart `tinkercloud.service` and run doctor
   again. A legacy updater retry is permitted only for its exact rejection of
   `--release-manifest`, and uses only signed binary/metadata/signature after
   full local release-manifest verification; every other update failure is
   terminal. Never read or print the LLM root, call a provider, copy new
   secrets, re-run init, or clean existing host state. A
   temporary test signing key cannot update a reused host.
9. For one noninteractive run using already-exported environment, invoke only
   the checked-in wrapper. It runs the offline gates before exactly one live
   pass. Its offline VPS package gate explicitly skips `^TestVPSAcceptance$`,
   requires the shipped local Resend reader and strict SSH inputs, and
   leaves a private redacted timestamped status artifact. It never sources an
   env file, evaluates environment values as shell, retries the suite, or
   prints secrets:

   ```bash
   skills/tinkercloud-full-stack-test/scripts/run-unattended.sh
   ```

10. For a terminal-guided run, execute one deliberate acceptance pass only:

   ```bash
   go test ./test/vps -run TestVPSAcceptance -count=1 -v
   ```

Do not create an infinite deploy loop. On ambiguity, malformed provider data,
timeouts, unsafe local state, or any failed denial assertion, stop and retain
only redacted diagnostics. The acceptance result proves Resend API acceptance
and Tinkercloud OTP flow; perform an occasional manual mailbox-delivery smoke
separately.

## Bundled scripts

- `scripts/read-resend-otp.py`: fixed-origin, fail-closed Resend reader.
- `scripts/test_read_resend_otp.py`: offline deterministic reader tests.
- `scripts/run-unattended.sh`: one-pass noninteractive wrapper with a private
  redacted status artifact.
- `scripts/test_run_unattended.sh`: deterministic no-network wrapper
  preflight, ordering, single-live-pass, and report self-test.
