# Tinkercloud VPS acceptance test

`go test ./test/vps -run TestVPSAcceptance -v` is a destructive, opt-in black-box acceptance run for a **disposable dedicated Ubuntu 24.04 LTS or Ubuntu 26.04 LTS amd64 VPS**. It cross-builds and uploads `tinkercloud`, runs native initialization, authorizes a second deployer, authenticates one browser identity at `admin.<domain>`, then deploys protected apps. It proves that same browser reaches a second allowed app without another OTP, retains a dashboard-only global identity plus distinct host-only app sessions, preserves app paths and repeated encoded query parameters through a handoff, and sees a friendly no-OTP denial for an excluded app with no app bytes. It also rejects replayed and wrong-host callbacks, proves app-local logout stays local, then proves global dashboard logout revokes every derived app session. It also proves anonymous content denial, server-derived identity, SDK-equivalent blob capability discovery, one-file multipart upload/list/exact binary attachment download/delete, multipart extra-part denial without mutation, anonymous and cross-app denial without blob bytes, and restart persistence. The SSH target must be a root SSH destination in exact `root@host` form.

It never reads OTP data from SQLite, the server filesystem, or logs. Configure a local OTP reader executable; Tinkercloud invokes it with exactly three arguments: `deployer|viewer`, email, hostname. Its stdout must contain only a 4–12 digit code.

Required environment:

```sh
export TINKERCLOUD_VPS_E2E=1
export TINKERCLOUD_VPS_SSH_TARGET=root@203.0.113.10
export TINKERCLOUD_VPS_ACKNOWLEDGE="$TINKERCLOUD_VPS_SSH_TARGET"
export TINKERCLOUD_VPS_KNOWN_HOSTS_FILE=/absolute/path/to/known_hosts
export TINKERCLOUD_VPS_DOMAIN=example.com
export TINKERCLOUD_VPS_OPERATOR_EMAIL=operator@example.com
export TINKERCLOUD_VPS_DEPLOYER_EMAIL=deployer@example.com
export TINKERCLOUD_VPS_VIEWER_EMAIL=viewer@example.com
export TINKERCLOUD_VPS_EMAIL_FROM=operator@example.com
export TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN=example.com
export TINKERCLOUD_VPS_RESEND_API_KEY_FILE=/absolute/path/to/resend-key
export TINKERCLOUD_RESEND_READER_API_KEY_FILE=/absolute/path/to/local-resend-reader-key
export TINKERCLOUD_RESEND_OTP_LEDGER_FILE=/absolute/path/to/local-consumed-otp-ledger.json
export TINKERCLOUD_VPS_OTP_COMMAND=$PWD/skills/tinkercloud-full-stack-test/scripts/read-resend-otp.py
go test ./test/vps -run TestVPSAcceptance -count=1 -v
```

After exporting the required values, the sole noninteractive wrapper is:

```sh
skills/tinkercloud-full-stack-test/scripts/run-unattended.sh
```

It does not load an env file. It fail-closes on a missing or malformed
`TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN`, unsafe reader key/ledger, untrusted
SSH input, acknowledgement mismatch, or failed offline gate; then it
runs exactly one real `TestVPSAcceptance` pass (the offline package gate
explicitly skips it). Its private timestamped status
artifact defaults to `.tinker/vps/unattended-reports/`; override that location
only with an absolute owner-only mode-`0700`
`TINKERCLOUD_VPS_UNATTENDED_REPORT_DIR`.
In reuse mode it also requires an absolute, existing, caller-owned,
non-symlink `TINKERCLOUD_VPS_RELEASE_DIR` before it begins offline gates.

For the recommended repo-local, gitignored key, pinned host key, and SSH config
layout, follow [the external VPS setup guide](../../docs/operations/vps-e2e.md).
The helper is:

```sh
./scripts/setup-vps-ssh.sh 203.0.113.10
```

Optional `TINKERCLOUD_VPS_SSH_PORT` and `TINKERCLOUD_VPS_SSH_IDENTITY_FILE` are passed as structured SSH arguments. Host-key checking is always enabled; there is no `StrictHostKeyChecking=no` mode.

By default the suite refuses hosts containing Tinkercloud paths. For a disposable
pre-initialized test host, `TINKERCLOUD_VPS_REUSE=1` requires the root-owned suite
marker at `/var/lib/tinkercloud-vps-e2e/marker` to exactly match the SSH target
and root domain. Reuse also requires
`TINKERCLOUD_VPS_RELEASE_DIR`: the suite verifies the supplied candidate against
the installed server's pinned key and uses only the normal signed update and
health-gate path after a new active probe app exists. It does not create a
test-only authorization or network path, re-initialize, or clean the host.

For unattended real OTPs, use the local-only Resend reader documented in the
[VPS guide](../../docs/operations/vps-e2e.md#unattended-resend-otp-reading).
It reads sent mail through Resend rather than Tinkercloud/VPS state, filters one
exact fresh message, and emits only its numeric code. The reader key and its
consumed-ID ledger must be absolute local mode-`0600` non-symlink files and
are never copied to the VPS. The existing `TINKERCLOUD_VPS_E2E`, target
acknowledgement, strict SSH trust, and `TINKERCLOUD_VPS_REUSE=1` marker rules
remain mandatory.
