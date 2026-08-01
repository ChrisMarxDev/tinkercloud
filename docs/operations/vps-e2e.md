# External VPS smoke acceptance

The opt-in VPS suite installs Tinkercloud on a dedicated Ubuntu 24.04 LTS/amd64 or
Ubuntu 26.04 LTS/amd64 VPS, authorizes a deployer, deploys a unique private
smoke app, and signs in a separate viewer identity. It is intentionally outside
`go test ./...`.

Read the [VPS E2E contract](../../specs/operations/vps-e2e-contract.md) first.
It defines the exact trust, environment, and evidence boundary.

## VPS prerequisites

Prepare:

- a disposable, dedicated Hetzner Cloud VPS using Ubuntu 24.04 LTS or Ubuntu
  26.04 LTS on x86-64;
- a public IPv4 address, synchronized system clock, and unused ports 80/443;
- inbound SSH restricted to the operator's source IP where practical, plus
  inbound TCP 80/443 for Tinkercloud and ACME;
- root SSH access using a dedicated key (the suite installs and recovers the
  host as root, while the gateway itself runs as the unprivileged `tinkercloud`
  service user);
- one wildcard `A` record for `*.<domain>` pointing at the VPS. Tinkercloud
  derives `admin.<domain>` for the dashboard and `<slug>.<domain>` for apps;
- a verified Resend sending domain and API key;
- distinct operator, deployer, and viewer email addresses; the suite derives its
  internal ACME contact from the operator email; and
- a known-hosts entry whose fingerprint was checked through the Hetzner console
  or another out-of-band trusted channel.

The host must not run Docker, a reverse proxy, or another public service on
ports 80/443. The acceptance suite is destructive by design and refuses a host
with existing Tinkercloud configuration, application data, binary, or service
unit unless the explicit disposable-host reuse gate matches. A fixed
`/var/lib/tinkercloud-acme` directory from an earlier disposable run may remain:
it is transport cache rather than app state, is revalidated by initialization,
and avoids needless duplicate certificate issuance. Do not delete that cache
between clean reruns for the same domain.

## Repo-local SSH connection files

The repository ignores `.tinker/`, so keep the dedicated private key, public key,
pinned host key, and SSH config in `.tinker/vps/`. Create them with:

```bash
./scripts/setup-vps-ssh.sh 203.0.113.10
```

The helper reuses a matching keypair already at that path, or invokes
`ssh-keygen` for a new Ed25519 key and asks you to choose a passphrase. Add
`.tinker/vps/id_ed25519.pub` as the root SSH key while creating the VPS. If the
key must support unattended tests, load it into an SSH agent first:

```bash
ssh-add "$PWD/.tinker/vps/id_ed25519"
```

Fetch a candidate host key only after the VPS exists:

```bash
ssh-keyscan -p 22 -t ed25519 203.0.113.10 \
  > "$PWD/.tinker/vps/known_hosts.candidate"
ssh-keygen -lf "$PWD/.tinker/vps/known_hosts.candidate"
```

Compare that fingerprint with the server console or another out-of-band source.
Only after it matches, promote it and test the pinned connection:

```bash
mv "$PWD/.tinker/vps/known_hosts.candidate" "$PWD/.tinker/vps/known_hosts"
ssh -F "$PWD/.tinker/vps/ssh_config" tinkercloud-test
```

Do not disable SSH checking or trust `ssh-keyscan` without the independent
fingerprint comparison.

```bash
export TINKERCLOUD_VPS_E2E=1
export TINKERCLOUD_VPS_SSH_TARGET='root@203.0.113.10'
export TINKERCLOUD_VPS_ACKNOWLEDGE="$TINKERCLOUD_VPS_SSH_TARGET"
export TINKERCLOUD_VPS_KNOWN_HOSTS_FILE="$PWD/.tinker/vps/known_hosts"
export TINKERCLOUD_VPS_SSH_IDENTITY_FILE="$PWD/.tinker/vps/id_ed25519"

export TINKERCLOUD_VPS_DOMAIN='example.com'
export TINKERCLOUD_VPS_OPERATOR_EMAIL='operator@example.com'
export TINKERCLOUD_VPS_DEPLOYER_EMAIL='deployer@example.com'
export TINKERCLOUD_VPS_VIEWER_EMAIL='viewer@example.com'
export TINKERCLOUD_VPS_EMAIL_FROM='tinker@example.com'
export TINKERCLOUD_VPS_RESEND_API_KEY_FILE="$PWD/.tinker/vps/resend-api-key"
export TINKERCLOUD_RESEND_READER_API_KEY_FILE="$PWD/.tinker/vps/resend-reader-api-key"
export TINKERCLOUD_RESEND_OTP_LEDGER_FILE="$PWD/.tinker/vps/resend-otp-consumed.json"
export TINKERCLOUD_VPS_OTP_COMMAND="$PWD/skills/tinkercloud-full-stack-test/scripts/read-resend-otp.py"
```

The OTP helper is called directly as:

```text
read-tinkercloud-otp deployer|viewer EMAIL HOSTNAME
```

It must emit only a 4--12 digit code. Without a helper, run from an interactive
terminal and enter each received code when prompted; unattended execution
fails closed.

Then run:

```bash
go test ./test/vps -run TestVPSAcceptance -count=1 -v
```

For one unattended overnight pass after exporting the same variables, use:

```bash
skills/tinkercloud-full-stack-test/scripts/run-unattended.sh
```

It runs the offline reader tests, skill-drift check, VPS package test with
`^TestVPSAcceptance$` explicitly skipped, and diff check before exactly one
live acceptance invocation. It will only use the
shipped local Resend reader, refuses unsafe local secret/ledger files and any
SSH acknowledgement mismatch, and never sources an env file. It prints the
path of a private mode-`0600` redacted timestamped status artifact (default
`.tinker/vps/unattended-reports/`) on either success or failure. Set an absolute,
owner-only mode-`0700` `TINKERCLOUD_VPS_UNATTENDED_REPORT_DIR` to choose another
local location. The report intentionally contains step outcomes only; it never
contains credentials, OTPs, mail data, or provider responses.
With `TINKERCLOUD_VPS_REUSE=1`, it also refuses before offline gates unless
`TINKERCLOUD_VPS_RELEASE_DIR` is an absolute, existing, caller-owned,
non-symlink directory; the live suite still verifies the release signature.

The suite confirms the 80/443 listeners belong to Tinkercloud, then after deployer
login lists that deployer's apps and deletes only its four fixed fixture names
when they are present: `vps-e2e-update-probe`, `vps-e2e-primary`,
`vps-e2e-isolation`, and `vps-e2e-denied`. It uses a fresh idempotency key for
each deletion and stops on a list or deletion error; it never deletes an
unrelated app. It deploys those stable-host fixtures while keeping randomized
archive markers, denies anonymous HTML/asset/API/WebSocket access without a
marker, and verifies the allowed viewer's post-OTP app and identity access. It
also deploys a `features.blobs: true` SDK-equivalent fixture and
proves capability discovery plus authenticated upload/list/exact binary
download/delete. It rejects an extra multipart part without catalog mutation,
proves anonymous and cross-app guessed-ID reads contain no blob bytes, requires
attachment/private-no-store/nosniff download headers, and checks the bytes
survive a service restart. It leaves the VPS state available after a failure. For a disposable
already-initialized host only, set `TINKERCLOUD_VPS_REUSE=1`; the existing
root-owned suite marker must match the target and root domain.
Reuse does not reset or clean up the host. It additionally requires
`TINKERCLOUD_VPS_RELEASE_DIR` to name a release verified by the installed
server's pinned release key. Once a fresh active probe app exists, the suite
uses the normal signed `tinkercloud update` command and its rollback health gate;
it never re-runs initialization or replaces the binary directly.

First ACME issuance and public DNS propagation can make the final platform
health proof temporarily unavailable. The suite safely reruns the same init
command up to eight times, 15 seconds apart, only after the binary reports its
stable `public_health_failed` readiness result and the remote init state shows
that every earlier durable step completed. Setup, credentials, install, local
service, SSH, or state failures stop immediately; no installer or secret-copy
step is repeated between readiness attempts.

Clean application-state reruns preserve `/var/lib/tinkercloud-acme`. Repeatedly
deleting it and requesting the same certificate again can exhaust the public
CA's duplicate-certificate allowance while proving nothing additional about
Tinkercloud. Destroy it only when retiring the VPS/domain or deliberately rotating
that transport state outside the acceptance loop.

[The checked-in VPS smoke app](../../examples/test-apps/vps-smoke/) is a
manifest fixture and documentation example. The live suite creates a fresh
archive with a randomized marker for every run, but always deploys it to the
same bounded fixture hostnames so repeat runs reuse their cached exact-host
certificates.

## Unattended Resend OTP reading

For an unattended real-OTP test, keep a Resend reader key **only on this local
machine**. It needs sent-email read access (Resend Full access) and must never
be placed in the VPS secret file, server configuration, browser, SDK, or Git.
Create and lock down the file once:

```bash
umask 077
mkdir -p "$PWD/.tinker/vps"
${EDITOR:-vi} "$PWD/.tinker/vps/resend-reader-api-key"
chmod 600 "$PWD/.tinker/vps/resend-reader-api-key"
```

The shipped reader is dependency-free and uses only Resend's fixed HTTPS API.
It polls for at most 60 seconds, accepts one exact recent Tinkercloud OTP for the
requested deployer/viewer and host, records an opaque consumed message ID in
the local mode-`0600` ledger, then prints only the code to the existing test
harness. It never reads Tinkercloud/VPS state or logs. A malformed, stale,
ambiguous, unrelated, already-consumed, or provider-error response stops the
run without printing a code.

For a disposable test-only setup, set
`TINKERCLOUD_RESEND_READER_API_KEY_FILE` to the existing local send-key file only
if that key has been deliberately granted sent-email read permission. Do not
broaden a production VPS key solely for testing. Prefer separate least-
privilege sending and local reader keys; for tonight's disposable test, the
current full-access file may be used for both variables, then rotate or narrow
it afterwards.
