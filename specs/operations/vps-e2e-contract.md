# VPS end-to-end acceptance contract

This contract describes the explicit black-box acceptance run in `test/vps`.
It is for a dedicated Ubuntu 24.04 LTS/amd64 or Ubuntu 26.04 LTS/amd64 VPS and
is not selected by default tests. The test runner performs remote installation
through SSH; Tinkercloud is still the only process allowed to serve the deployed
app publicly.

## Required environment

Required variables are fail-closed: missing, empty, multiline, unreadable, or
invalid values prevent the live run from starting.

| Variable | Requirement |
| --- | --- |
| `TINKERCLOUD_VPS_E2E` | Exact value `1`. |
| `TINKERCLOUD_VPS_SSH_TARGET` | Root SSH destination in exact `root@host` form, with no whitespace or option-like prefix. |
| `TINKERCLOUD_VPS_ACKNOWLEDGE` | Exact value of `TINKERCLOUD_VPS_SSH_TARGET`. It is the dedicated-host acknowledgement. |
| `TINKERCLOUD_VPS_KNOWN_HOSTS_FILE` | Absolute path to an existing regular file containing the target's trusted host key. |
| `TINKERCLOUD_VPS_SSH_PORT` | Optional decimal SSH port. |
| `TINKERCLOUD_VPS_SSH_IDENTITY_FILE` | Optional absolute existing private-key path. |
| `TINKERCLOUD_VPS_DOMAIN` | Existing root domain whose wildcard DNS record resolves to the VPS. The suite derives the dashboard as `admin.<domain>` and every app as `<slug>.<domain>`. |
| `TINKERCLOUD_VPS_OPERATOR_EMAIL` | Initial operator email used during host initialization and as the derived internal ACME contact. |
| `TINKERCLOUD_VPS_DEPLOYER_EMAIL` | Email authorized as the deployer used in the control login. |
| `TINKERCLOUD_VPS_VIEWER_EMAIL` | Email allowed by the smoke-app policy and used for app login. Supply a distinct identity from the deployer. |
| `TINKERCLOUD_VPS_EMAIL_FROM` | Verified Resend sender. |
| `TINKERCLOUD_VPS_RESEND_API_KEY_FILE` | Absolute path to the existing local Resend-key file. |
| `TINKERCLOUD_VPS_OTP_COMMAND` | Optional absolute executable that obtains sent OTPs. |
| `TINKERCLOUD_RESEND_READER_API_KEY_FILE` | Required by the shipped unattended Resend reader: absolute local mode-`0600`, non-symlink Resend key with sent-email read access. It must never be copied to the VPS. |
| `TINKERCLOUD_RESEND_OTP_LEDGER_FILE` | Optional absolute local mode-`0600`, non-symlink consumed-message ledger for the shipped reader. Defaults beside the reader key and is never copied to the VPS. |
| `TINKERCLOUD_VPS_RELEASE_DIR` | Optional absolute verified release directory. If absent, a clean-host run builds and signs a temporary release locally. **Required with reuse**: before any offline gate, the wrapper requires an existing caller-owned non-symlink directory; the live suite verifies it against the installed server's pinned signing key. |
| `TINKERCLOUD_VPS_REUSE` | Optional exact value `1`; permits an already-initialized disposable host only when its root-owned suite marker exactly matches the root domain and SSH target. |
| `TINKERCLOUD_VPS_UNATTENDED_REPORT_DIR` | Optional absolute local mode-`0700` directory for `run-unattended.sh` redacted timestamped status artifacts. Defaults to `.tinker/vps/unattended-reports/`; it is never copied to the VPS. |

The VPS must already have one public wildcard DNS record for `*.<domain>` plus
a verified Resend sending domain. The suite does not create DNS records or
retrieve credentials from the VPS.

## Unattended wrapper

`skills/tinkercloud-full-stack-test/scripts/run-unattended.sh` is the sole
noninteractive convenience command. It reads only already-exported variables;
it does not source an env file or evaluate environment content as shell. It
requires the checked-in absolute Resend reader path, explicit local reader-key
and ledger paths, exact `TINKERCLOUD_VPS_E2E=1` acknowledgement, and safe local
files before running anything. It creates a local owner-only, mode-`0600`,
timestamped redacted status artifact in a mode-`0700` report directory.

The wrapper runs its offline reader self-test, shared-skill drift check, VPS
package test (explicitly skipping `^TestVPSAcceptance$`), and `git diff --check`
in that order. Only if all pass does it
invoke exactly one `TestVPSAcceptance` run. It has no retry loop, no SSH
fallback, no OTP bypass, and no report field for a secret, OTP, email body,
provider response, or message ID.

## SSH, secrets, and OTP

SSH and SCP receive structured arguments with `BatchMode=yes`,
`StrictHostKeyChecking=yes`, and `UserKnownHostsFile` set to the supplied
known-hosts file. A mismatch stops the run. The suite does not accept new host
keys, disable checking, or execute a user-supplied SSH target through a local
shell.

The suite creates a fresh root-owned remote staging directory named
`/root/tinkercloud-vps-e2e-*`. It copies the signed release artifacts, the local
Resend key, and a locally generated HMAC key there; the two secret files are
set to mode `0600` and passed to `tinkercloud init` only as remote file paths.

When configured, the OTP helper is executed directly with this exact argv:

```text
${TINKERCLOUD_VPS_OTP_COMMAND} deployer|viewer EMAIL HOSTNAME
```

It must print only a 4--12 digit code on stdout. If no helper is set, a
terminal run prompts locally for the code; a noninteractive run fails.
The suite never reads OTP challenges or secrets from the VPS database, files,
HTTP endpoints, or logs.

### Local Resend OTP reader

`skills/tinkercloud-full-stack-test/scripts/read-resend-otp.py` is an opt-in local
mail-reader for unattended acceptance. It is not part of Tinkercloud production
and does not alter the deployed gateway, its database, or its authentication
flow. It calls only the fixed HTTPS `https://api.resend.com` origin with a
local reader credential. Before reading the key or ledger or making a provider
request, it accepts a deployer or viewer recipient only when its normalized
domain is exactly `christopher-marx.de`; subdomains and lookalikes deny. This
local-automation constraint does not restrict normal Tinkercloud human login.
It then accepts only an exact recipient, exact configured sender, exact
`Your sign-in code` subject, a bounded recent
timestamp, and exactly the derived `admin.<domain>` hostname for both deployer
and viewer global-identity OTP flows, plus the exact
Tinkercloud text body `Your code: NNNN...`. It fetches one matching message and
records its opaque message ID in a private consumed-ID ledger before emitting
only the 4--12 digit code on stdout.

Ambiguous matches, malformed provider responses, unreadable or unsafe key or
ledger files, provider errors, invalid permission responses, stale messages,
and timeouts all fail closed. Diagnostics on stderr must be generic: they must
not include a provider body, key, key path, email body, OTP, or message ID.
The reader never queries the VPS, Tinkercloud logs, SQLite, app state, or a test
only bypass. Its full-access Resend key is local-only, gitignored, mode `0600`,
and revocable. Prefer a separate least-privilege sending key for the VPS and a
local reader key. A disposable setup may deliberately point both local
variables at the current full-access key only when it has the required
sent-email read permission; that broader key then reaches the VPS for that
test and must be rotated or narrowed afterwards.

## Initialization readiness retry

`tinkercloud init` persists each completed durable step. Its final, public
platform-health proof can remain incomplete while first ACME issuance or DNS
propagation becomes reachable. The VPS suite therefore runs the identical
non-interactive init argv at most eight times, with a 15-second
cancellation-aware wait between attempts. It retries only when both of the
following independently observable conditions hold:

1. the failed command reports the stable `tinkercloud: public_health_failed`
   readiness result; and
2. the root-owned init-state record is valid and its next incomplete step is
   exactly `verified`.

All other init failures are terminal for the suite: in particular, preflight,
config, credential, install, local-service, SSH, and state-read failures do
not receive another mutation attempt. The suite does not re-install artifacts,
rewrite secrets, or run broad setup work between retries; resumable init state
makes each retry a final-proof attempt. Context cancellation stops immediately.

## Performed lifecycle and assertions

### Local deployer authorization command

The root-only server command has one positional and flag grammar:

```text
tinkercloud deployers ACTION [--config PATH] EMAIL
```

`ACTION` is exactly one of `authorize`, `suspend`, or `revoke`. Flags follow
the action and precede the single email argument. `--config` defaults to
`/etc/tinkercloud/config.yaml`. Unknown actions or flags, flags after `EMAIL`,
and extra positional arguments fail with a stable typed error without changing
deployer state. The command delegates email normalization and validation to the
persistence boundary; its diagnostic output does not disclose paths or secret
values. A successful durable mutation refreshes an already-running
`tinkercloud.service` and rechecks active state before it returns success; a
deliberately inactive service stays inactive. The explicit refresh-failure
result never claims to reverse the durable mutation.

1. Confirm the explicit gate, acknowledgement, file inputs, strict SSH trust,
   and a clean Tinkercloud configuration/application-data host unless
   `TINKERCLOUD_VPS_REUSE=1` is explicitly supplied with a matching suite marker.
   A prior disposable run's fixed `/var/lib/tinkercloud-acme` cache is not
   application state and may remain: initialization must still reject an
   unsafe or symlinked path, then reuse the ACME account and valid certificates.
   Clean reruns must not erase this cache merely to retest initialization,
   because doing so consumes public-CA issuance capacity without improving the
   application/authentication proof.
2. Verify supplied release artifacts or build a temporary signed release,
   install it, initialize Tinkercloud on a clean host (allowing only the bounded
   final-readiness retry defined above), run `tinkercloud doctor`, and confirm
   Tinkercloud owns listeners on ports 80 and 443 with no additional non-loopback
   listener owned by Tinkercloud. Operator-owned SSH, firewall policy, and
   pre-existing listeners remain outside Tinkercloud's mutation scope.
3. Authorize `TINKERCLOUD_VPS_DEPLOYER_EMAIL` locally on the VPS; authenticate
   that deployer through the control OTP flow. Immediately list that
   deployer's apps and remove only previously listed suite-owned fixture slugs,
   each with a fresh idempotency key. The complete fixed set is
   `vps-e2e-update-probe`, `vps-e2e-primary`, `vps-e2e-isolation`, and
   `vps-e2e-denied`; an absent fixture is not an error, while a list or delete
   failure is terminal. The suite never deletes an unlisted or unrelated app.
4. Create the fixed `vps-e2e-primary` app, restrict its access policy to
   `TINKERCLOUD_VPS_VIEWER_EMAIL`, and build the smoke archive with the same
   canonical private `tinker.yaml` allowlist. Warm its normal ACME path and
   deploy that immutable verified release. Activation installs the archive's
   manifest policy atomically, so the archive—not the earlier control request—
   remains the source of the active viewer allowlist.
   Redeploy that same immutable archive once to the same app before continuing.
   The second deployment must activate safely, supersede the prior deployment,
   retain the archive's private policy, and still pass anonymous gateway denial;
   no viewer-login handoff may be used as certificate-readiness evidence.
5. Before viewer login, deny the fixed primary app's HTML, private asset, current-
   viewer API, and WebSocket-upgrade request without returning its random
   marker or completing a WebSocket upgrade.
6. Authenticate the initial viewer once at `https://admin.<domain>/login`.
   Verify that the browser identity cookie is not sent to an app host, then
   open the first allowed app through its server-created handoff. Its derived
   host-only app session must return the generated marker and
   `/_tinker/api/v1/me` must report the configured viewer email. Deploy a second
   allowed app and prove the same browser jar completes its server-created
   handoff without requesting or reading another viewer OTP; both app hosts
   must retain distinct app cookies. Preserve an app path and repeated,
   percent-encoded query parameters through that handoff exactly. The admin
   dashboard's role check and each app policy check remain independent.
   A third app that excludes that identity must render the generic broker
   denial without app bytes or an OTP request. Replay the consumed callback and
   send it to the other app host; both must deny without issuing a replacement
   app session. Prove app-local logout leaves the browser identity and the
   other app usable, then broker-reopen the logged-out app without an OTP.
   Finally, a global dashboard logout must revoke the browser identity and all
   derived app sessions. A later explicit switch to the configured deployer
   identity must also revoke the original child sessions before the
   deployer-only app opens.
   The deployed fixture has
   `features.blobs: true` and contains a same-origin SDK-equivalent blob client
   example.
7. With that real viewer session, discover `blobs` and `db`; reject a multipart upload
   containing a second part without changing the catalog, then upload, list,
   download, and delete one binary fixture. Download bytes must be exact and
   have attachment, `private, no-store`, and `nosniff` headers. Anonymous and
   guessed cross-app reads must return denial without fixture bytes. After the
   service restart, the suite performs a bounded, context-cancellable gateway
   readiness retry only for connection-startup transport failures or 502/503/
   504 responses; every successful retry response must still prove the exact
   blob bytes and attachment, `private, no-store`, and `nosniff` headers before
   the delete assertion. Redirects, denials, all other statuses, wrong bytes,
   and wrong headers fail immediately.
8. Using the same host-derived viewer session, create, get, optimistic-update,
   list with `snapshot=1`, and delete a JSON document through
   `/_tinker/api/v1/db/{collection}`. The fixture has `features.kv` and
   `features.realtime` enabled, but sends no database, app, or viewer selector.
   Prove an anonymous request denies with no document bytes, a second allowed
   app cannot read a guessed document ID, and the updated document survives the
   service restart before deletion. Capability discovery must include `db` and
   omit ungranted `llm.chat`; an attempted chat call without a manifest request
   or operator grant denies without provider detail.
9. In reuse mode, after a newly active probe app exists, copy only the signed
   release evidence (including its signed release manifest), invoke
   `tinkercloud verify-artifact` followed by the supported local signed
   `tinkercloud update` path. The candidate must first pass full local
   release-manifest verification. Only an exact old-updater rejection of the
   `--release-manifest` flag may retry with the legacy signed
   binary/metadata/signature argv; verification, health, transport, or any
   other failure is terminal. After replacement, require normal `tinkercloud
   doctor` (including its current-version check), then once run root-only
   `tinkercloud llm enable`, restart `tinkercloud.service`, and run normal
   `tinkercloud doctor`. Never call a provider or read/print the LLM root,
   re-run init, or remove existing host state. A temporary E2E signing
   authority is therefore forbidden with reuse.

On any failed step, the suite exits non-zero and leaves the VPS state in place
for operator investigation.
