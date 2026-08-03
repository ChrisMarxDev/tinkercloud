---
name: tinkercloud-operator
description: Install, configure, diagnose, update, and recover a Tinkercloud server. Use for operator-owned Hetzner VPS setup, DNS, TLS, Resend, Postmark, SendGrid, or SMTP mail, deployer authorization, status, doctor, signed updates, disk incidents, root recovery, and Tinkercloud security verification.
---

# Tinkercloud operator

<!-- shared:role-common:start -->
## Platform boundary

Use `operator` for the person who hosts Tinkercloud, `deployer` for a person
authorized to create and manage their own apps, and `viewer` for a person who
accesses an app. A deployment agent is automation acting through deployer
authority. Never transfer credentials or authority between these roles.

Tinkercloud V1 hosts private static apps. The accepted post-V1 public-static
extension remains gateway-only: it permits only reviewed, capability-free,
immutable static files after both a current app policy and a default-off,
revisioned operator gate succeed. The gateway owns TLS, app routing,
viewer authentication, access policy, static files, SDK capabilities, and
deployment activation. It derives the app from the hostname and the viewer from
an opaque app-host session. App code must never select either identity.

Fail closed. Missing, stale, malformed, redirected, incompatible, or ambiguous
security state is a denial, not a value to guess. Never expose or request a
deployer token, OTP, provider secret, database credential, app ID, or viewer ID
in chat, argv, source code, `tinker.yaml`, browser storage, logs, or output.

V1's historical release boundary is private-only, and the app owner is always
an implicit viewer. In the accepted post-V1 extension, public is never a
default or a capability grant: a public release needs `access.mode: public`,
no browser capabilities, explicit deployer acknowledgement, and the current
operator gate. Reserved `/_tinker/*` routes remain private-viewer-only; a
public release cannot expose identity, SDK, KV, collections, blobs, realtime,
or LLM access. Each app has its own private SQLite data file for KV and bounded
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

Do not report success from a happy path alone. A private deployment is complete
only after a fresh anonymous HTTPS request proves no app content is exposed. A
public-static deployment additionally needs the exact anonymous document/asset
and indexing proof plus denial of every reserved route through that gateway.
<!-- shared:role-common:end -->

<!-- shared:operator:start -->
## Operator workflow

Operate Tinkercloud as one small security boundary: one `tinkercloud` server, one
SQLite database, one private data directory, and one systemd service on a
dedicated supported Hetzner VPS. Only Tinkercloud listens publicly on TCP 80/443.
Do not add Docker as the primary install, a reverse proxy, another file server,
a storage mount, a public object URL, a backend runtime, or a second recovery
authority in V1.

Use this repository's `PRINCIPLES.md`, `PRD.md`, `specs/operations/`, and
`docs/operations/` for implementation detail when available. Before host
mutation, confirm the exact target is a clean dedicated x86-64 Hetzner VPS
running explicitly supported Ubuntu 24.04 LTS or 26.04 LTS. Ambiguous,
interim, end-of-life, other-distribution, or future unverified releases deny.

For hosted releases, dispatch only `release.yml`. Its reusable validation
workflows never receive release Environment secrets; the selected direct,
reviewer-protected top-level release job alone reads the channel signing key and
creates the GitHub release. Never add `secrets: inherit`, a repository signing
secret, or a second publication dispatch.

<!-- beta-operator-clean:start -->
### 1. Verify the beta host

For a clean beta host, require a clean dedicated x86-64 Ubuntu 24.04 or 26.04
VPS. In the VPS provider console's root shell, run this read-only preflight;
stop unless it reports `x86_64` and Ubuntu `24.04` or `26.04`:

```sh
uname -m && . /etc/os-release && printf '%s %s\n' "$ID" "$VERSION_ID"
```

### 2. Verify the SSH host key

The VPS provider console is the trust source for the SSH host-key fingerprint.
In that console, record the fingerprint:

```sh
ssh-keygen -l -f /etc/ssh/ssh_host_ed25519_key.pub
```

On the workstation run `ssh root@<HOST>`, compare the prompt's SHA256 ED25519
fingerprint with that console value, and accept it only when it matches. Bind
every later `scp` and `ssh` to that same `root@<HOST>` target. Never
trust or accept an ssh-keyscan result by itself, disable host-key checking, or
auto-accept an unknown key.

### 3. Configure one wildcard DNS record

Create one `*.<DOMAIN>` wildcard record using `A`/`AAAA` or `CNAME` as the DNS provider supports. It covers the derived `admin.<DOMAIN>` dashboard and every
synthetic one-label app host. Do not ask for or require a separate `admin`
record. Before certificates, require nonempty resolution for both derived hosts:

```sh
getent ahosts admin.<DOMAIN> >/dev/null
getent ahosts onboarding-check.<DOMAIN> >/dev/null
```

Read the clean VPS public IPv4 from the provider console and point the wildcard
`A` record exactly there. Add `AAAA` only for a confirmed reachable public IPv6;
use CNAME only for a provider-supplied stable hostname. Never guess a target.
The operator needs DNS control but supplies no extra certificate input: setup
derives the ACME contact from the normalized operator email. `getent` proves
only nonempty resolution; compare the intended record target in the provider UI.

### 4. Install v0.1.6

For a clean host, first probe the exact immutable installer from that VPS's
root shell. HTTPS success permits installation and any failure stops:

```sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 --head --location --fail --silent --show-error --max-time 15 -o /dev/null https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v0.1.6
curl --proto '=https' --proto-redir '=https' --tlsv1.2 --location --fail --silent --show-error --max-time 15 --max-filesize 32768 -o /dev/null https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-host.sh
```

Then install once:

```sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-host.sh | sh
```

The released host installer embeds that immutable release directory; do not add
an origin, `latest` selector, provider credential, or signing material. The
copy/paste installer verifies checksums and a pinned Ed25519 signature before
installation. Inspect the [exact immutable beta
release](https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v0.1.6)
when useful.
If the exact `v0.1.6` release or the host installer asset is unavailable,
stop: do not install, deploy, or substitute another version. Minimal HTTPS
release readiness is the exact tag page plus the host installer asset URL
returning HTTPS success; the installer remains the checksum/signature authority.
The installer remains the
cryptographic authority, and any later exact-version proof failure stops.
HTTPS release-asset redirects are transport-only. Repository/development
verification and the optional workstation flow retain explicit `--release-base`
support where their contracts require it.

### 5. Reuse or transfer the Resend credential

Keep the provider credential out of chat, argv, and ordinary config. If the
credential is already on the VPS, reuse it after the exact root-only check:
`test -f /root/.config/tinkercloud/resend-api-key && test ! -L /root/.config/tinkercloud/resend-api-key && test "$(stat -c '%U:%G %a' /root/.config/tinkercloud/resend-api-key)" = 'root:root 600'`.
Transfer only when the credential is workstation-local or its location is
ambiguous. Only then ask for the SSH target; from the workstation, use the
already verified SSH host key for the supplied target; do not weaken SSH
verification. Substitute <HOST> and
<LOCAL_CREDENTIAL_FILE> with the supplied values; do not ask for them again.
On the VPS, create the credential directory, transfer only the file, then
verify the final destination is a root-owned regular mode-`0600` file:

```sh
ssh root@<HOST> 'install -d -o root -g root -m 0700 /root/.config/tinkercloud'
scp -p -- "<LOCAL_CREDENTIAL_FILE>" "root@<HOST>:/root/.config/tinkercloud/resend-api-key"
ssh root@<HOST> 'test -f /root/.config/tinkercloud/resend-api-key && test ! -L /root/.config/tinkercloud/resend-api-key && chown root:root /root/.config/tinkercloud/resend-api-key && chmod 0600 /root/.config/tinkercloud/resend-api-key'
```

For an alternate VPS-local path, validate that supplied exact path with the
same root-owned, non-symlink regular mode-`0600` check and pass that validated
path directly to setup. `/root/.config/tinkercloud/resend-api-key` is only the
workstation-transfer destination, never a reason to copy an already-safe file.

### 6. Run setup

Run the implemented resumable setup path after the credential boundary is
ready:

```sh
sudo tinkercloud setup
```

It asks only for the controlled base domain, initial operator email, verified
Resend sender, and root-readable Resend credential path when they cannot be
derived. This guided beta path explicitly selects and persists `resend` from
the verified sender and root credential path before initialization continues.
It generates private HMAC material and delegates to the strict initialization
state machine. Keep `init --non-interactive` for deterministic automation and
explicit Resend, Postmark, SendGrid, or SMTP selection. That separate
noninteractive/other-provider flow selects by environment presence in fixed order:
`RESEND_API_KEY`, `POSTMARK_SERVER_TOKEN`, `SENDGRID_API_KEY`, then the complete
`TINKERCLOUD_SMTP_*` set. The first present provider wins; do not invent a
selector variable or failure-based failover. SMTP is send-only, authenticated,
and requires certificate-verified `starttls` or implicit `tls`. The external
mail guide is optional troubleshooting, not required for the basic Resend beta
path; use `docs/operations/mail-setup.md` only for preparation, switching, or
diagnostics beyond that path. Pause for one exact DNS or selected email-provider
action when external state is incomplete; resume without re-asking verified
answers.

Give the exact validated VPS-local credential path to setup. The canonical
`/root/.config/tinkercloud/resend-api-key` is only the workstation-transfer
destination. Normalize every email by trimming outer whitespace, preserving
local-part case, lowercasing only the domain, requiring exactly one `@`,
nonempty local/domain, a dotted domain, and no whitespace/control characters;
never plus/dot rewrite. When values are
missing, setup prompts in this exact order and with these exact labels:

1. `Base domain:`
2. `Operator email:`
3. `Verified Resend sender email:`
4. `Root-readable Resend API key file:`

### 7. Authorize the exact deployer set

Setup derives `https://admin.<domain>/login`; use that exact dashboard URL. A
valid browser identity is reused. Otherwise, this bounded beta path permits one human browser OTP. In the dashboard select **Deployers**, then **Active deployer allowlist**; enter the reviewed normalized set in **Allowed deployer emails**.
When adding an email, check **I confirm that adding any email grants deployment authority.**, then select **Save active deployers**. Replace the reviewed
allowlist rather than applying a partial edit. Replace the initial active
deployer allowlist with exactly the intended normalized deployer email set and
no other addresses; the operator email alone is sufficient only when that is
the exact intended set. Operator setup permits at most one human browser OTP,
uses zero when a reusable browser identity is valid, and uses no CLI deployer
OTP. One browser OTP maximum for the operator and one CLI OTP maximum for the
deployer; the two-role total is at most two and never permits two OTPs for
either role. Terminal operator browser authentication rule: immediately after
an operator browser OTP attempt fails, is malformed, times out, or is denied,
stop operator onboarding. Do not retry or request another code, switch operator
identity or mailbox, clear browser cookies, or create another OTP path. A valid
exact browser identity uses zero OTP. This does not alter unattended machine OTP
acceptance.

A fully fresh successful human onboarding with neither a reusable browser
identity nor a saved CLI bearer requests exactly two codes total: exactly one
operator browser code and exactly one deployer CLI code. A reusable identity
reduces the relevant lane to zero.

### 8. Verify operator completion

After setup and deployer authorization, run the local checks and record the
direct public version proof:

```sh
sudo tinkercloud status
sudo tinkercloud doctor
curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https://admin.<domain>/api/v1/version
```

The HTTPS request must not follow a redirect. Completion requires HTTP `200`,
an `application/json` media type, and exact bounded body `{"api_version":1}`
with no extra or error fields; record included gateway headers. Protected-app
anonymous denial belongs to deployer deployment completion once an app exists;
do not fabricate it during empty operator setup.
<!-- beta-operator-clean:end -->

The workstation `tinker host install|status|doctor|update|uninstall` commands may perform
the same root-local flow over an explicit `root@HOST` using normal OpenSSH
host-key verification. They are not a remote control API: never add SSH
options, arbitrary remote commands, automatic host-key acceptance, or a
deployer bearer to that path. `tinker host uninstall root@HOST` removes the
canonical Tinkercloud service, credentials, application state, and binary only
after an exact local confirmation; use its explicit `--yes` only in a reviewed
non-interactive operator workflow. It preserves `/var/lib/tinkercloud-acme` by
default so reinstalling does not needlessly request certificates. Never add a
path, remote-command, or certificate-deletion option to this workflow.
The root command rejects owned mount points before stopping the service; after
uninstall, the next canonical setup recreates the service identity and safely
re-owns the preserved ACME cache.

A released `tinker host install root@HOST` derives its pinned GitHub release
directory from the installed CLI version. Development builds must receive an
explicit `--release-base`; installing artifacts and initialization are
intentionally separate. Follow the install with `sudo tinkercloud setup`; use
resumable `init --non-interactive` only for deterministic automation.

Keep provider and Tinkercloud secrets in root-owned mode-0600 credential files or
systemd credentials. Never accept them in argv, ordinary YAML, chat, browser
state, shell history, logs, or audit. Run the gateway as the unprivileged
`tinkercloud` service identity with only the narrow bind capability for 80/443.
During initialization, create and seed the control SQLite database only in a
fixed child that permanently drops to that identity before opening SQLite; do
not let the root init parent create database, WAL, or SHM artifacts. A resumable
legacy install may hand off only those exact validated root-owned artifacts,
never recursively change the data directory or widen modes.

If offering `llm.chat`, configure it as an operator-owned capability adapter:
use the dashboard **API keys** section to choose Anthropic or Gemini and enter
one write-only provider key. Tinkercloud derives its safe label and opaque ID;
never ask for a name, ID, URL, or arbitrary secret. If the dashboard says key
management is unavailable, run the root-only `tinkercloud llm enable` and restart
before entering a key; never expose or copy the encryption root. Define a fixed
approved model/profile and bounded quotas in **LLM chat**, then grant that
profile explicitly to selected apps. The browser receives only Tinkercloud's
same-origin response; it never receives the provider key, connection ID,
provider endpoint, raw provider error, arbitrary model choice, or an outbound
proxy. Removing a grant or connection must deny the next request. The local
`tinker dev` emulator deliberately excludes LLM capability calls, so verify the
real adapter through its normal protected gateway tests.

Use `tinkercloud status` for offline state and `sudo tinkercloud doctor` for checks
that may read root credentials. Initialization is complete only after the exact
platform HTTPS version proof, route classification, socket confinement, safe
unknown-app denial, database, permissions, DNS, TLS, and email checks succeed.
Do not replace public evidence with localhost, a browser page, a redirect, or
insecure TLS.

Record both local commands and this no-redirect public proof before claiming
setup success: `https://admin.<domain>/api/v1/version` must return bounded
API-version JSON with gateway headers. Its bounded onboarding OTP ceilings are
nonfungible: one browser OTP maximum for the operator and one CLI OTP maximum
for the deployer; the two-role total never permits two OTPs for either role.
Before issuing, relaying, requesting, or suggesting a third human OTP, stop
immediately; do not switch accounts, clear credentials, create a viewer
session, or use another mailbox to evade it.

Manage deployer authority as one revision-protected exact normalized email
allowlist. Show the complete resulting list. Require explicit confirmation only
when adding deployment authority. Removal immediately revokes that deployer's
CLI tokens and pending CLI OTPs while removing dashboard authority on the next
request. It preserves their global browser identity and independently
authorized app access.

For root recovery with `tinkercloud deployers authorize|suspend|revoke`, the
database mutation runs in a fixed child that drops permanently to `tinkercloud`.
After that child has committed and closed, Tinkercloud refreshes only an already
active service and verifies it remains active; it never starts an inactive
service. `deployer_applied_service_refresh_failed` means the authority mutation
is durable but the platform must be checked with `sudo tinkercloud doctor` before
the changed deployer is asked to log in.

Install only artifacts verified against signed release metadata. A checksum
alone is not a trust root. An update verifies both the server artifact and the
signed compatibility manifest before snapshot; air-gapped updates require both
triplets and never mix local with remote inputs. `tinkercloud update` must retain rollback state until
the restarted candidate passes doctor, listener readiness, platform health,
and anonymous protected-app denial. Any ambiguous or failed gate restores the
prior healthy state.

The updater derives its probe from installed server state and selects a locally
verified active app itself. Never ask for or pass an app ID, slug, probe host,
path, or URL. If a failed update did not complete automatic restoration, use
the installed root-local recovery path:

```sh
sudo tinkercloud update --rollback
tinkercloud status
sudo tinkercloud doctor
```

Do not invent another rollback command or delete rollback state by hand.

Automatic server updates are disabled until the operator explicitly enables
one official channel with `sudo tinkercloud updates enable --channel beta` or
`sudo tinkercloud updates enable --channel stable`. The fixed timer accepts only
a strictly newer same-schema release, then uses the exact signed update and
rollback gates above. It must preserve configuration, root credentials, apps,
app databases/blobs/releases, browser sessions, deployer bearers, and ACME
state. `sudo tinkercloud updates disable` stops future checks without changing
that durable state.

Local app insights default enabled. To stop or resume new local tracking, use
the root-local `tinkercloud insights disable` or `tinkercloud insights enable`.
This command delegates the durable SQLite mutation to the `tinkercloud` service
identity, then refreshes only an already active service after that child closes;
it never starts an inactive service or creates a browser/remote mutation path.
Dashboard aggregates remain owner/operator-only, and a disabled or unavailable
read must be shown as unavailable rather than zero.

Root access to the dedicated VPS is the recovery authority. If email is
unavailable, use root-only operator recovery and revoke affected global
identity families and CLI credentials; never create a remote HTTP recovery
bypass.

The accepted post-V1 public-static gate remains disabled unless an operator
explicitly changes it with the root-local `tinkercloud public enable` command.
That command must be revisioned and audited; it changes no app policy. Before
enabling it, verify the intended scope and explain that only independently
acknowledged, capability-free public releases can become anonymous. Use
`tinkercloud public disable` to revoke anonymous static access; the next
anonymous request must deny while normal private owner/viewer access remains.
The child closes its durable SQLite mutation before success and does not query,
restart, or start systemd: the gateway reads the gate on every request, so a
service refresh would add downtime without improving revocation.
Never add a public listener, proxy, file server, public capability endpoint, or
remote gate mutation.

At disk warning, diagnose and clean only through database-led Tinkercloud
operations. At the write-stop watermark, new deployments, KV mutations, and
blob uploads deny while safe reads, deletions, and revocations continue.
Never expose raw storage paths.

V1 has no operator backup or disaster-recovery feature. State plainly that
loss of the VPS can lose releases, KV, and blobs. Do not describe utility-grade
local state or realtime as durable.

For every consequential operation, report the exact target, command or
dashboard action, observed result, and remaining verification. Never claim
setup, update, recovery, or security completion from the happy path alone.
<!-- shared:operator:end -->
