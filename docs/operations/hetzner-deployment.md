# Hetzner-First Deployment

The canonical V1 topology is a dedicated public VPS with public DNS and inbound
TCP 80/443. See [common setup scenarios](setup-scenarios.md) for the complete
solo/startup flow and the current VPN-only support gap.

## Target experience

On a clean Hetzner Cloud Ubuntu 24.04 LTS or Ubuntu 26.04 LTS x86-64 VPS
dedicated to Tinkercloud:

```bash
sudo sh ./packaging/install.sh ./tinkercloud ./tinkercloud.json ./tinkercloud.sig
sudo tinkercloud setup
```

The guided setup discovers the host, asks for one controlled base domain and
the initial operator email, derives conventional platform/app/sender values,
and pauses with exact DNS and Resend actions. It resumes without asking for
prior valid answers, ingests the provider key into the root-owned credential
boundary, generates non-secret config and internal secrets, and completes the
ordinary health/security gates. See the
[complete operator flow](../../concept/flows/operator.html).

The install script is a thin convenience wrapper. It:

1. Detects supported Linux architecture.
2. Downloads the matching `tinkercloud` release.
3. Verifies its checksum and signature.
4. Installs the binary in a standard executable path.
5. Runs `tinkercloud install-service`.

All meaningful setup logic lives in the signed binary, not a large mutable
shell script. Operators may download and verify the binary manually instead.
Generated config is the resulting system record and automation interface, not a
document the human must author before setup.

## Deterministic non-interactive initialization

Automation and recovery still use the explicit contract:

```bash
sudo tinkercloud init --non-interactive \
  --domain example.com \
  --operator-email operator@example.com \
  --email-from access@example.com --acme-email operator@example.com \
  --resend-api-key-file /root/tinkercloud-resend.key \
  --hmac-key-file /root/tinkercloud-hmac.key
```

`init --non-interactive` requires explicit non-secret flags and root-readable
secret files. It never prompts or derives missing values. The human `setup`
assistant generates these inputs and may accept the Resend secret through a
no-echo prompt only if that path passes the supported-shell secret-handling
audit; otherwise it guides creation/selection of a protected file. Neither path
places a secret in argv, ordinary config, or terminal output. Initialization
copies the values into
`/etc/tinkercloud/credentials/tinkercloud.env` at mode `0600`; the config contains
only `env:` references. Do not pass API keys as command-line values.

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
6. validates the root-only Resend credential file and config references without
   sending a test message;
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
- a verified sending domain and Resend API key.

Strict VPN-only ingress is not supported in V1. Public ACME HTTP-01 and the
public HTTPS health proof must succeed; initialization never bypasses them.
The accepted first post-V1 direction is one operator-supplied certificate/key
pair covering the admin and app hostnames, as described in
[ADR 0028](../decisions/0028-operator-supplied-tls-for-vpn-only.md).

`sudo tinkercloud doctor` validates and explains these dependencies after the
service starts, but Tinkercloud cannot safely create DNS or verify email-domain
ownership without additional provider credentials. Unlike the offline-safe
`tinkercloud status`, doctor reads the root-only systemd credential file directly
for its read-only Resend check; it never prints the file path, references, key,
or provider response body. The default is
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
enables scheduled or unattended updates.

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

No silent auto-update in V1. A later opt-in schedule can call the same command.

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
TINKER_RELEASE_BASE=https://github.com/ChrisMarxDev/tinkercloud/releases/download/v1.0.0/ \
  ./packaging/install-client.sh
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
- Resend is the only shipped email adapter, behind a provider interface.
- Private apps only; there is no public-app switch in V1.
