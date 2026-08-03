# Hetzner-First Deployment

The canonical V1 topology is a dedicated public VPS with public DNS and inbound
TCP 80/443. See [common setup scenarios](setup-scenarios.md) for the complete
solo/startup flow and the current VPN-only support gap.

## Current installation

The following `v0.1.6` command is prepared for the next beta but is
unpublished. Run it only after that exact GitHub prerelease has been published;
until then it is not an installation path and must not be replaced with
`v0.1.5`, `latest`, or another selector. From the root shell of a clean Hetzner
Cloud Ubuntu 24.04 LTS or Ubuntu 26.04 LTS x86-64 VPS dedicated to Tinkercloud,
install the signed server binary and systemd unit with that exact version:

```bash
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-host.sh | sh
```

The released installer embeds that exact immutable GitHub release directory,
permits only HTTPS asset redirects, and verifies checksums plus pinned Ed25519
evidence before installation. Do not pass an origin or secret to this
root-shell command.

An operator who already uses the workstation CLI may instead run `tinker host
install root@HOST`; that optional SSH flow derives the exact release directory
from a released CLI version. A development CLI must pass a reviewed explicit
`--release-base HTTPS_RELEASE_DIRECTORY` whose signed host installer was built
for that same directory.

Installation does not guess platform configuration. Initialize the installed
host with the implemented guided command:

```bash
sudo tinkercloud setup
```

It discovers host state and asks only for the controlled base domain, initial
operator email, verified Resend sender, and a root-readable Resend API-key
file. It derives conventional platform/app hostnames and the internal ACME
contact from the operator email, generates internal HMAC material, pauses with
one exact DNS or Resend action when needed, and resumes without re-asking
valid answers. See the [complete operator flow](../../concept/flows/operator.html).

The install script is a thin convenience wrapper. It:

1. Detects supported Linux architecture.
2. Downloads the matching `tinkercloud` release.
3. Verifies its checksum and signature.
4. Installs the binary in a standard executable path.
5. Runs `tinkercloud install-service`.

All meaningful initialization logic lives in the signed binary, not a large
mutable shell script. Operators may download and verify the binary manually
instead. Generated config is the resulting system record and automation
interface, not a document the human must author before setup.

## Deterministic non-interactive initialization

Automation and recovery still use the explicit contract:

```bash
sudo tinkercloud init --non-interactive \
  --domain example.com \
  --operator-email operator@example.com \
  --email-provider postmark \
  --email-from access@example.com \
  --email-api-key-file /root/tinkercloud-postmark-token \
  --hmac-key-file /root/tinkercloud-hmac.key
```

`init --non-interactive` requires explicit non-secret flags and root-readable
secret files. `--email-provider` accepts `resend`, `postmark`, `sendgrid`, or
`smtp` and defaults to Resend for compatibility; all use the neutral
`--email-api-key-file` boundary for the API key or SMTP password. SMTP also
requires its host, port, username, and TLS-mode flags. It derives the internal
ACME contact from the required normalized operator email; it never accepts a
separate ACME-contact flag. The implemented human `setup` assistant guides the
Resend path, accepts only a root-readable protected key file as its credential
source, and creates HMAC material privately. Neither path places a secret in
argv, ordinary config, or terminal output. Initialization
copies the values into
`/etc/tinkercloud/credentials/tinkercloud.env` at mode `0600`; the config contains
no provider credential or selection. Do not pass API keys or passwords as
command-line values. See [Outbound Mail Setup](mail-setup.md) for every adapter
and the environment-variable priority.

The shared initialization domain:

1. Validates the exact Ubuntu 24.04 LTS/amd64 or Ubuntu 26.04 LTS/amd64
   allowlist, NTP synchronization, and exclusive availability of public ports
   80 and 443. Interim, end-of-life, malformed, and future unverified releases
   deny before host mutation. DNS and disk are checked by `tinkercloud doctor`
   after initialization, not by the resumable preflight.
2. Creates the dedicated `tinkercloud` service user and private data directory.
3. Writes root-owned secret references and non-secret typed configuration.
4. Initializes SQLite and applies embedded migrations.
5. Creates the first operator.
6. validates the root-only selected email-provider credential file and config
   references without sending a test message;
7. Generates, installs, enables, and starts a hardened systemd unit whose
   writable allowlist contains exactly the configured data and ACME directories.
8. Verifies the service is locally active.
9. Validates the public admin gateway through verified TLS at
   `https://admin.{domain}/api/v1/version`. The probe follows no redirect and
   accepts only its final configured host, `200` bounded
   `{"api_version":1}` JSON, and `no-store`/`nosniff` gateway headers. Per-app
   certificate readiness is verified at deployment activation, not bootstrap.
10. Prints the dashboard URL.

Initialization is resumable and idempotent. It records each completed durable
step in `/etc/tinkercloud/init-state.json`; a failed step remains retryable and
the command never advertises the service as ready until both health probes pass.

## Required external preparation

The operator must provide:

- a clean supported Hetzner VPS with a public IP;
- one wildcard `A`/`AAAA` record for the root domain;
- a verified sender and Resend, Postmark, SendGrid, or authenticated TLS SMTP
  credential.

Strict VPN-only ingress is not supported in V1. Public ACME HTTP-01 and the
public HTTPS health proof must succeed; initialization never bypasses them.
The accepted first post-V1 direction is one operator-supplied certificate/key
pair covering the admin and app hostnames, as described in
[ADR 0028](../decisions/0028-operator-supplied-tls-for-vpn-only.md).

`sudo tinkercloud doctor` validates and explains these dependencies after the
service starts, but Tinkercloud cannot safely create DNS or verify email-domain
ownership without additional provider credentials. Unlike the offline-safe
`tinkercloud status`, doctor reads the root-only systemd credential file directly
for its read-only selected email-provider check; it never prints the file path,
references, key, or provider response body. The default is
`/etc/tinkercloud/credentials/tinkercloud.env`; use `--config` and `--credentials`
only for an explicit root-owned test or recovery layout.

## Resulting host shape

```text
/usr/local/bin/tinkercloud          # one signed server binary
/etc/tinkercloud/config.yaml        # non-secret configuration
/etc/tinkercloud/credentials/       # root-owned secrets
/var/lib/tinkercloud/               # SQLite, releases, keys, update rollback
/etc/systemd/system/tinkercloud.service
```

Tinkercloud's installed network-exposure delta is exactly its public TCP 80/443
gateway listeners. The installer does not modify the operator's firewall, SSH,
or pre-existing listeners. The service runs as the unprivileged `tinkercloud` user
after installation. Its systemd sandbox grants exactly
`CAP_NET_BIND_SERVICE` through both the ambient and bounding capability sets,
default-denies socket binds, allows only TCP 80/443, and restricts socket
address families while retaining `NoNewPrivileges=yes` and strict filesystem
protections; it does not run the gateway as root.

## Updating

```bash
# Use the configured updates.release_base, or name the HTTPS release directory.
sudo tinkercloud update
sudo tinkercloud update \
  --release-base https://github.com/ChrisMarxDev/tinkercloud/releases/download/v1.0.0/
```

For an air-gapped host, copy the server artifact triplet and signed
`release-manifest.json` triplet onto the VPS. Use `--binary`, `--metadata`,
`--signature`, `--release-manifest`, `--release-manifest-metadata`, and
`--release-manifest-signature` together. The command does not
follow redirects, accepts only the pinned signed `tinkercloud-linux-amd64` release,
and rejects private or link-local release origins. It derives public health from
installed state and deterministically selects a locally verified active app for
the anonymous capability-denial probe. If no active app exists, it proves that
exact state plus platform health and safe unknown-app-host denial; the first
deployment later proves full protected-app denial. Callers cannot supply an app
slug, probe host, path, or URL.

To avoid repeating `--release-base`, set a release directory (not an artifact
file) in the root-owned config:

```yaml
updates:
  release_base: https://github.com/ChrisMarxDev/tinkercloud/releases/download/v1.0.0/
```

Changing this value selects where the manually invoked updater looks; it never
enables scheduled updates.

The update command:

```text
check compatibility
→ download
→ verify signature
→ create bounded rollback state
→ stop/drain service
→ apply binary and migrations
→ restart
→ run health and anonymous-denial probes
→ keep new version or roll back automatically
```

The control-credential separation migration revokes every bearer token created
before that upgrade because old rows could have represented either a dashboard
cookie or a CLI token. After the healthy upgrade, operators and deployers run
`tinker login` once to obtain a fresh CLI-only bearer; browser dashboard users
sign in again. No legacy credential is converted or retained.

Automatic updates remain off by default. To opt into the official signed beta
or stable channel, use:

```bash
sudo tinkercloud updates enable --channel beta
sudo tinkercloud updates status
sudo tinkercloud updates disable
```

The fixed systemd timer discovers only a strictly newer channel-matching GitHub
release and then runs the same signed update, health, denial, and rollback
gates. It refuses persistence-schema changes and never replaces configuration,
credentials, releases, app data, sessions, or ACME state.

## Recovery

Root access is authoritative:

```bash
sudo tinkercloud recover operator --email new@example.com
sudo tinkercloud doctor
sudo tinkercloud update --rollback
```

Recovery commands require a local root shell and are never exposed as remote
HTTP bypasses. The update `--rollback` flag restores the server binary after a
failed update; it is not a deployer application-release rollback.

## Deployer installation

Deployer machines install only the smaller `tinker` client:

```bash
curl --proto '=https' --tlsv1.2 -fsSL \
  https://github.com/ChrisMarxDev/tinkercloud/releases/download/vVERSION/install-client.sh | sh
tinker login
tinker deploy .
```

Run the installer as the deployer user, never with `sudo`. It validates the
chosen client against the embedded release key before writing to
`~/.local/bin`; obtain the script from a reviewed checkout or signed source
release rather than treating an unauthenticated `curl | sh` fetch as a trust
root.

The server and client negotiate API compatibility. A client/server mismatch
produces a direct upgrade instruction rather than a generic deployment error.

## Deployer authorization and login

Hosting is not open merely because someone knows the platform URL:

1. The operator edits one revision-protected exact active-deployer allowlist in
   the dashboard. New/reactivated addresses require explicit broadening
   confirmation. Removed addresses immediately lose current API/control
   credentials while their immutable IDs and owned apps remain. Audit failure
   rolls back the entire reconciliation.
   Root recovery/automation may still authorize one normalized address with
   `tinkercloud deployers authorize <email>`. The root-only command grammar is
   `tinkercloud deployers <authorize|suspend|revoke> [--config PATH] <email>`;
   `--config` defaults to `/etc/tinkercloud/config.yaml` and must appear before
   the email. It writes SQLite in a fixed child that drops to the unprivileged
   `tinkercloud` service identity, including a narrowly validated handoff of any
   legacy root-owned DB/WAL/SHM artifacts; it never loosens database modes.
   Once the child has committed and closed, the root parent refreshes an
   already-running Tinkercloud service and confirms it is active. It never starts
   an inactive service. If that refresh fails, the command reports that the
   authorization was applied but service refresh failed; run `sudo tinkercloud
   doctor` before relying on the new deployer.
2. The deployer runs `tinker login`. If no verified default platform exists, the
   human CLI asks once for `https://admin.example.com`, proves compatibility
   without redirects, saves only that URL, and continues. JSON never prompts.
3. The CLI requests an OTP; the platform returns the same safe response whether
   or not the address is authorized.
4. The deployer enters the emailed code in the CLI.
5. After OTP verification and a current deployer-status check, the server
   issues a server-bound, scoped CLI token.
6. The CLI stores the token in its protected per-user Tinker credential file,
   never a project file: `os.UserConfigDir()/tinker/<sha256(normalized-server)>.json`.
   The configuration directory is mode `0700`; the regular non-symlinked
   credential file is mode `0600` and is atomically replaced in place. `tinker
   login` is needed once per valid token; later CLI commands reuse it silently.
7. Every control-plane request rechecks token validity, deployer status, scope,
   and target ownership. Revocation takes effect on the next request.

The canonical deploy-first human flow is `tinker deploy .`: it can perform the
same missing-platform/login steps, derive safe local project defaults, and
generate a missing `tinker.yaml` as a reviewable receipt. It never runs the
project build command. See the
[complete deployer flow](../../concept/flows/deployer.html).

An unauthorized email never receives authority to create an app, upload a
release, or mutate policy, even if OTP delivery was requested successfully.

## Explicit V1 limits

- Hetzner Ubuntu 24.04 LTS and 26.04 LTS x86-64 server support.
- No numeric Ubuntu version range: interim, end-of-life, and future unverified
  releases fail preflight.
- No Docker requirement or bundled reverse proxy.
- No operator backup/disaster-recovery feature.
- Resend and Postmark are the only shipped email adapters; exactly one is
  selected behind the provider-neutral interface.
- Private apps only; there is no public-app switch in V1.
