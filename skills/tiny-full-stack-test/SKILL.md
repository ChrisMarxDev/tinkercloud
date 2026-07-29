---
name: tiny-full-stack-test
description: Run and maintain TinyHost's opt-in unattended full-stack VPS acceptance workflow, including local Resend OTP retrieval, SSH trust, deployment, viewer authorization, and negative security evidence. Use when configuring, debugging, or executing real end-to-end TinyHost tests against a disposable VPS.
---

<!-- shared:role-common:start -->
## Platform boundary

Use `operator` for the person who hosts TinyHost, `deployer` for a person
authorized to create and manage their own apps, and `viewer` for a person who
accesses an app. A deployment agent is automation acting through deployer
authority. Never transfer credentials or authority between these roles.

TinyHost V1 hosts private static apps. The gateway owns TLS, app routing,
viewer authentication, access policy, static files, SDK capabilities, and
deployment activation. It derives the app from the hostname and the viewer from
an opaque app-host session. App code must never select either identity.

Fail closed. Missing, stale, malformed, redirected, incompatible, or ambiguous
security state is a denial, not a value to guess. Never expose or request a
deployer token, OTP, provider secret, database credential, app ID, or viewer ID
in chat, argv, source code, `tiny.yaml`, browser storage, logs, or output.

V1 is private-only. The app owner is always an implicit viewer. There is no
public mode. KV and blobs are utility-grade data on one VPS; losing the VPS can
lose them. Realtime is app-scoped, in-memory, best-effort notification with no
history, replay, ordering, or delivery guarantee.

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
   PYTHONDONTWRITEBYTECODE=1 python3 skills/tiny-full-stack-test/scripts/test_read_resend_otp.py
   ./scripts/check-skill-drift
   go test ./test/vps -count=1
   git diff --check
   ```

3. Keep `TINYHOST_RESEND_READER_API_KEY_FILE` local, absolute, regular,
   non-symlink, owned by the invoking user, and mode `0600`. Keep the
   consumed-message ledger local and mode `0600`. Never copy either to the VPS
   or print its contents.
4. Set `TINYHOST_VPS_OTP_COMMAND` to the absolute path of
   `scripts/read-resend-otp.py`. Supply exact platform/app host and sender
   environment values. The reader receives only `deployer|viewer EMAIL HOST`;
   both V1 OTP purposes must use exactly `TINYHOST_VPS_PLATFORM_HOST` because
   the global identity broker owns the flow. It must print only a 4--12 digit
   code.
5. Require `TINYHOST_VPS_E2E=1`, the exact target acknowledgement, a checked
   known-hosts file, and normal `TINYHOST_VPS_REUSE=1` marker gating. Never
   weaken SSH trust or introduce an OTP/auth bypass. In the real browser-jar
   proof, read exactly one initial viewer OTP for the first allowed app through
   the platform identity broker; the second allowed app must get a separate
   host-only app session without another OTP, while an excluded app renders a
   generic no-app-bytes/no-OTP denial. Assert the global identity and the
   non-authorizing browser-binding cookies are platform-only and app session
   cookies are distinct per app host. The binding must exist after the initial
   broker form, never appear on an app host, and remain after a global account
   switch. Replay a consumed callback and target it at a sibling app host as
   denials; app-local logout must preserve global identity and sibling access,
   while a later account switch uses its intentionally separate viewer-purpose
   OTP and revokes all old child sessions.
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
7. Reuse never re-initializes or wipes the VPS. It requires an explicit local
   `TINYHOST_VPS_RELEASE_DIR`; verify it through the installed server's pinned
   key and use only `tinyhost update` with its active-app health gate. The
   unattended wrapper fails before offline gates unless this is an absolute,
   existing, caller-owned non-symlink directory. A
   temporary test signing key cannot update a reused host.
8. For one noninteractive run using already-exported environment, invoke only
   the checked-in wrapper. It runs the offline gates before exactly one live
   pass. Its offline VPS package gate explicitly skips `^TestVPSAcceptance$`,
   requires the shipped local Resend reader and strict SSH inputs, and
   leaves a private redacted timestamped status artifact. It never sources an
   env file, evaluates environment values as shell, retries the suite, or
   prints secrets:

   ```bash
   skills/tiny-full-stack-test/scripts/run-unattended.sh
   ```

9. For a terminal-guided run, execute one deliberate acceptance pass only:

   ```bash
   go test ./test/vps -run TestVPSAcceptance -count=1 -v
   ```

Do not create an infinite deploy loop. On ambiguity, malformed provider data,
timeouts, unsafe local state, or any failed denial assertion, stop and retain
only redacted diagnostics. The acceptance result proves Resend API acceptance
and TinyHost OTP flow; perform an occasional manual mailbox-delivery smoke
separately.

## Bundled scripts

- `scripts/read-resend-otp.py`: fixed-origin, fail-closed Resend reader.
- `scripts/test_read_resend_otp.py`: offline deterministic reader tests.
- `scripts/run-unattended.sh`: one-pass noninteractive wrapper with a private
  redacted status artifact.
- `scripts/test_run_unattended.sh`: deterministic no-network wrapper
  preflight, ordering, single-live-pass, and report self-test.
