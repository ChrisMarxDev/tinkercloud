# External VPS smoke acceptance

The opt-in VPS suite installs TinyHost on a dedicated Ubuntu 24.04 LTS/amd64 or
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
  inbound TCP 80/443 for TinyHost and ACME;
- root SSH access using a dedicated key (the suite installs and recovers the
  host as root, while the gateway itself runs as the unprivileged `tinyhost`
  service user);
- an `A` record for the platform host and an `A` wildcard record for the app
  suffix, both pointing at the VPS;
- a verified Resend sending domain and API key;
- distinct operator, deployer, and viewer email addresses, plus an ACME contact
  email; and
- a known-hosts entry whose fingerprint was checked through the Hetzner console
  or another out-of-band trusted channel.

The host must not run Docker, a reverse proxy, or another public service on
ports 80/443. The acceptance suite is destructive by design and refuses a host
with existing TinyHost state unless the explicit disposable-host reuse gate
matches.

## Repo-local SSH connection files

The repository ignores `.tiny/`, so keep the dedicated private key, public key,
pinned host key, and SSH config in `.tiny/vps/`. Create them with:

```bash
./scripts/setup-vps-ssh.sh 203.0.113.10
```

The helper reuses a matching keypair already at that path, or invokes
`ssh-keygen` for a new Ed25519 key and asks you to choose a passphrase. Add
`.tiny/vps/id_ed25519.pub` as the root SSH key while creating the VPS. If the
key must support unattended tests, load it into an SSH agent first:

```bash
ssh-add "$PWD/.tiny/vps/id_ed25519"
```

Fetch a candidate host key only after the VPS exists:

```bash
ssh-keyscan -p 22 -t ed25519 203.0.113.10 \
  > "$PWD/.tiny/vps/known_hosts.candidate"
ssh-keygen -lf "$PWD/.tiny/vps/known_hosts.candidate"
```

Compare that fingerprint with the server console or another out-of-band source.
Only after it matches, promote it and test the pinned connection:

```bash
mv "$PWD/.tiny/vps/known_hosts.candidate" "$PWD/.tiny/vps/known_hosts"
ssh -F "$PWD/.tiny/vps/ssh_config" tinyhost-test
```

Do not disable SSH checking or trust `ssh-keyscan` without the independent
fingerprint comparison.

```bash
export TINYHOST_VPS_E2E=1
export TINYHOST_VPS_SSH_TARGET='root@203.0.113.10'
export TINYHOST_VPS_ACKNOWLEDGE="$TINYHOST_VPS_SSH_TARGET"
export TINYHOST_VPS_KNOWN_HOSTS_FILE="$PWD/.tiny/vps/known_hosts"
export TINYHOST_VPS_SSH_IDENTITY_FILE="$PWD/.tiny/vps/id_ed25519"

export TINYHOST_VPS_PLATFORM_HOST='tiny.example.com'
export TINYHOST_VPS_APP_SUFFIX='apps.example.com'
export TINYHOST_VPS_OPERATOR_EMAIL='operator@example.com'
export TINYHOST_VPS_DEPLOYER_EMAIL='deployer@example.com'
export TINYHOST_VPS_VIEWER_EMAIL='viewer@example.com'
export TINYHOST_VPS_EMAIL_FROM='tiny@example.com'
export TINYHOST_VPS_ACME_EMAIL='operator@example.com'
export TINYHOST_VPS_RESEND_API_KEY_FILE="$PWD/.tiny/vps/resend-api-key"
export TINYHOST_VPS_OTP_COMMAND="$PWD/.tiny/vps/read-tinyhost-otp"
```

The OTP helper is called directly as:

```text
read-tinyhost-otp deployer|viewer EMAIL HOSTNAME
```

It must emit only a 4--12 digit code. Without a helper, run from an interactive
terminal and enter each received code when prompted; unattended execution
fails closed.

Then run:

```bash
go test ./test/vps -run TestVPSAcceptance -count=1 -v
```

The suite confirms the 80/443 listeners belong to TinyHost, deploys a
randomized smoke archive, denies anonymous HTML/asset/API/WebSocket access
without its marker, and verifies the allowed viewer's post-OTP app and identity
access. It leaves the VPS state available after a failure. For a disposable
already-initialized host only, set `TINYHOST_VPS_REUSE=1`; the existing
root-owned suite marker must match the target, platform host, and app suffix.
Reuse does not reset or clean up the host.

First ACME issuance and public DNS propagation can make the final platform
health proof temporarily unavailable. The suite safely reruns the same init
command up to eight times, 15 seconds apart, only after the binary reports its
stable `public_health_failed` readiness result and the remote init state shows
that every earlier durable step completed. Setup, credentials, install, local
service, SSH, or state failures stop immediately; no installer or secret-copy
step is repeated between readiness attempts.

[The checked-in VPS smoke app](../../examples/test-apps/vps-smoke/) is a
manifest fixture and documentation example. The live suite instead creates a
fresh archive with a randomized slug and marker for each run.
