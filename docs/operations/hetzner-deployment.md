# Hetzner-First Deployment

The canonical V1 topology is a dedicated public VPS with public DNS and inbound
TCP 80/443. See [common setup scenarios](setup-scenarios.md) for the complete
solo/startup flow and the current VPN-only support gap.

## Target experience

On a clean Hetzner Cloud Ubuntu 24.04 LTS or Ubuntu 26.04 LTS x86-64 VPS
dedicated to TinyHost:

```bash
sudo sh ./packaging/install.sh ./tinyhost ./tinyhost.json ./tinyhost.sig
sudo tinyhost init --non-interactive \
  --platform-host tiny.example.com --app-suffix apps.example.com \
  --operator-email operator@example.com \
  --email-from operator@example.com --acme-email operator@example.com \
  --resend-api-key-file /root/tinyhost-resend.key \
  --hmac-key-file /root/tinyhost-hmac.key
```

The script is a thin convenience wrapper. It:

1. Detects supported Linux architecture.
2. Downloads the matching `tinyhost` release.
3. Verifies its checksum and signature.
4. Installs the binary in a standard executable path.
5. Runs `tinyhost install-service`.

All meaningful setup logic lives in the signed binary, not a large mutable shell
script. Operators may download and verify the binary manually instead.

## Non-interactive initialization

V1 deliberately has no interactive secret prompt: terminal input and argv are
too easy to retain in scrollback, shell history, and process inspection. `init`
requires `--non-interactive`, explicit non-secret flags, and root-readable
secret files. It copies the values into
`/etc/tinyhost/credentials/tinyhost.env` at mode `0600`; the config contains
only `env:` references. Do not pass API keys as command-line values.

`tinyhost init`:

1. Validates the exact Ubuntu 24.04 LTS/amd64 or Ubuntu 26.04 LTS/amd64
   allowlist, NTP synchronization, and exclusive availability of public ports
   80 and 443. Interim, end-of-life, malformed, and future unverified releases
   deny before host mutation. DNS and disk are checked by `tinyhost doctor`
   after initialization, not by the resumable preflight.
2. Creates the dedicated `tinyhost` service user and private data directory.
3. Writes root-owned secret references and non-secret typed configuration.
4. Initializes SQLite and applies embedded migrations.
5. Creates the first operator.
6. validates the root-only Resend credential file and config references without
   sending a test message;
7. Generates, installs, enables, and starts a hardened systemd unit whose
   writable allowlist contains exactly the configured data and ACME directories.
8. Verifies the service is locally active.
9. Validates the public platform gateway through verified TLS at
   `https://{platform_host}/api/v1/version`. The probe follows no redirect and
   accepts only its final configured host, `200` bounded
   `{"api_version":1}` JSON, and `no-store`/`nosniff` gateway headers. Per-app
   certificate readiness is verified at deployment activation, not bootstrap.
10. Prints the dashboard URL.

Initialization is resumable and idempotent. It records each completed durable
step in `/etc/tinyhost/init-state.json`; a failed step remains retryable and
the command never advertises the service as ready until both health probes pass.

## Required external preparation

The operator must provide:

- a clean supported Hetzner VPS with a public IP;
- `A`/`AAAA` records for the platform host and wildcard app host;
- a verified sending domain and Resend API key.

Strict VPN-only ingress is not supported in V1. Public ACME HTTP-01 and the
public HTTPS health proof must succeed; initialization never bypasses them.
The accepted first post-V1 direction is one operator-supplied certificate/key
pair covering the platform and wildcard app hostnames, as described in
[ADR 0028](../decisions/0028-operator-supplied-tls-for-vpn-only.md).

`sudo tinyhost doctor` validates and explains these dependencies after the
service starts, but TinyHost cannot safely create DNS or verify email-domain
ownership without additional provider credentials. Unlike the offline-safe
`tinyhost status`, doctor reads the root-only systemd credential file directly
for its read-only Resend check; it never prints the file path, references, key,
or provider response body. The default is
`/etc/tinyhost/credentials/tinyhost.env`; use `--config` and `--credentials`
only for an explicit root-owned test or recovery layout.

## Resulting host shape

```text
/usr/local/bin/tinyhost          # one signed server binary
/etc/tinyhost/config.yaml        # non-secret configuration
/etc/tinyhost/credentials/       # root-owned secrets
/var/lib/tinyhost/               # SQLite, releases, keys, update rollback
/etc/systemd/system/tinyhost.service
```

TinyHost's installed network-exposure delta is exactly its public TCP 80/443
gateway listeners. The installer does not modify the operator's firewall, SSH,
or pre-existing listeners. The service runs as the unprivileged `tinyhost` user
after installation. Its systemd sandbox grants exactly
`CAP_NET_BIND_SERVICE` through both the ambient and bounding capability sets,
default-denies socket binds, allows only TCP 80/443, and restricts socket
address families while retaining `NoNewPrivileges=yes` and strict filesystem
protections; it does not run the gateway as root.

## Updating

```bash
# Use the configured updates.release_base, or name the HTTPS release directory.
sudo tinyhost update --config /etc/tinyhost/config.yaml --app-slug payroll
sudo tinyhost update --config /etc/tinyhost/config.yaml --app-slug payroll \
  --release-base https://releases.example.net/tinyhost/v1.0.0/
```

For an air-gapped host, copy all three signed artifact files onto the VPS and
use `--binary`, `--metadata`, and `--signature` together. The command does not
follow redirects, accepts only the pinned signed `tinyhost-linux-amd64` release,
and rejects private or link-local release origins. It derives public health from
the configured platform host and requires `--app-slug` to name an existing
active app for the anonymous-denial probe; URLs cannot be supplied as probes.

To avoid repeating `--release-base`, set a release directory (not an artifact
file) in the root-owned config:

```yaml
updates:
  release_base: https://releases.example.net/tinyhost/v1.0.0/
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
`tiny login` once to obtain a fresh CLI-only bearer; browser dashboard users
sign in again. No legacy credential is converted or retained.

No silent auto-update in V1. A later opt-in schedule can call the same command.

## Recovery

Root access is authoritative:

```bash
sudo tinyhost recover operator --email new@example.com
sudo tinyhost doctor
sudo tinyhost update --config /etc/tinyhost/config.yaml --app-slug payroll --rollback
```

Recovery commands require a local root shell and are never exposed as remote
HTTP bypasses.

## Deployer installation

Deployer machines install only the smaller `tiny` client:

```bash
TINYHOST_CLIENT_RELEASE_BASE=https://releases.example.net/tinyhost/v1.0.0/ \
  ./packaging/install-client.sh
tiny login
tiny deploy .
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

1. The operator authorizes a normalized deployer email in the dashboard or with
   `tinyhost deployers authorize <email>`. The root-only command grammar is
   `tinyhost deployers <authorize|suspend|revoke> [--config PATH] <email>`;
   `--config` defaults to `/etc/tinyhost/config.yaml` and must appear before
   the email. It validates root authority, then writes SQLite as the unprivileged
   `tinyhost` service identity, including a narrowly validated handoff of any
   legacy root-owned DB/WAL/SHM artifacts; it never loosens database modes.
2. The deployer runs `tiny login --server https://tiny.example.com`.
3. The CLI requests an OTP; the platform returns the same safe response whether
   or not the address is authorized.
4. The deployer enters the emailed code in the CLI.
5. After OTP verification and a current deployer-status check, the server
   issues a server-bound, scoped CLI token.
6. The CLI stores the token in the operating-system credential store, never a
   project file.
7. Every control-plane request rechecks token validity, deployer status, scope,
   and target ownership. Revocation takes effect on the next request.

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
