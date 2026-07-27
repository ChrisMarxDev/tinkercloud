# TinyHost VPS acceptance test

`go test ./test/vps -run TestVPSAcceptance -v` is a destructive, opt-in black-box acceptance run for a **disposable dedicated Ubuntu 24.04 LTS or Ubuntu 26.04 LTS amd64 VPS**. It cross-builds and uploads `tinyhost`, runs native initialization, authorizes a second deployer, performs real deployer and viewer OTP logins, deploys an app, proves anonymous content denial, then proves the allowed viewer can access the app and server-derived identity endpoint. The SSH target must be a root SSH destination in exact `root@host` form.

It never reads OTP data from SQLite, the server filesystem, or logs. Configure a local OTP reader executable; TinyHost invokes it with exactly three arguments: `deployer|viewer`, email, hostname. Its stdout must contain only a 4–12 digit code.

Required environment:

```sh
export TINYHOST_VPS_E2E=1
export TINYHOST_VPS_SSH_TARGET=root@203.0.113.10
export TINYHOST_VPS_ACKNOWLEDGE="$TINYHOST_VPS_SSH_TARGET"
export TINYHOST_VPS_KNOWN_HOSTS_FILE=/absolute/path/to/known_hosts
export TINYHOST_VPS_PLATFORM_HOST=tiny.example.com
export TINYHOST_VPS_APP_SUFFIX=apps.example.com
export TINYHOST_VPS_OPERATOR_EMAIL=operator@example.com
export TINYHOST_VPS_DEPLOYER_EMAIL=deployer@example.com
export TINYHOST_VPS_VIEWER_EMAIL=viewer@example.com
export TINYHOST_VPS_EMAIL_FROM=operator@example.com
export TINYHOST_VPS_ACME_EMAIL=operator@example.com
export TINYHOST_VPS_RESEND_API_KEY_FILE=/absolute/path/to/resend-key
export TINYHOST_RESEND_READER_API_KEY_FILE=/absolute/path/to/local-resend-reader-key
export TINYHOST_RESEND_OTP_LEDGER_FILE=/absolute/path/to/local-consumed-otp-ledger.json
export TINYHOST_VPS_OTP_COMMAND=$PWD/skills/tiny-full-stack-test/scripts/read-resend-otp.py
go test ./test/vps -run TestVPSAcceptance -count=1 -v
```

For the recommended repo-local, gitignored key, pinned host key, and SSH config
layout, follow [the external VPS setup guide](../../docs/operations/vps-e2e.md).
The helper is:

```sh
./scripts/setup-vps-ssh.sh 203.0.113.10
```

Optional `TINYHOST_VPS_SSH_PORT` and `TINYHOST_VPS_SSH_IDENTITY_FILE` are passed as structured SSH arguments. Host-key checking is always enabled; there is no `StrictHostKeyChecking=no` mode.

By default the suite refuses hosts containing TinyHost paths. For a disposable
pre-initialized test host, `TINYHOST_VPS_REUSE=1` requires the root-owned suite
marker at `/var/lib/tinyhost-vps-e2e/marker` to exactly match the SSH target,
platform host, and app suffix. It does not create a test-only authorization or
network path.

For unattended real OTPs, use the local-only Resend reader documented in the
[VPS guide](../../docs/operations/vps-e2e.md#unattended-resend-otp-reading).
It reads sent mail through Resend rather than TinyHost/VPS state, filters one
exact fresh message, and emits only its numeric code. The reader key and its
consumed-ID ledger must be absolute local mode-`0600` non-symlink files and
are never copied to the VPS. The existing `TINYHOST_VPS_E2E`, target
acknowledgement, strict SSH trust, and `TINYHOST_VPS_REUSE=1` marker rules
remain mandatory.
