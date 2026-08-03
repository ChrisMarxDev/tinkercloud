# Tinkercloud

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

![Tinkercloud turns a folder into a protected team app](docs/assets/readme-deploy-flow.webp)

## Start the beta

Tinkercloud is a self-hosted private micro-app platform: one operator runs one
gateway, deployers publish small apps, and viewers authenticate by email. The
two role skills are independently usable as current standalone skill content:
[operator](https://raw.githubusercontent.com/ChrisMarxDev/tinkercloud/main/skills/tinkercloud-operator/SKILL.md)
and [deployer](https://raw.githubusercontent.com/ChrisMarxDev/tinkercloud/main/skills/tinkercloud-deployer/SKILL.md).

This page prepares the next exact beta, `v0.1.6`; its install and package
commands become runnable only after that prerelease is published.

<!-- beta-operator-readme:start -->
### 1. Operator: create the private host

You need a clean dedicated x86-64 Ubuntu 24.04 or 26.04 VPS with root SSH, DNS
control for the base domain, an operator email, a verified sender, a
root-readable protected provider-credential file, and the deployer emails to
authorize. In the VPS provider console's root shell, first run the read-only
preflight:

```sh
uname -m && . /etc/os-release && printf '%s %s\n' "$ID" "$VERSION_ID"
```

Continue only for `x86_64` Ubuntu 24.04/26.04. The VPS provider console is the
trust source for the SSH host-key fingerprint: run
`ssh-keygen -l -f /etc/ssh/ssh_host_ed25519_key.pub` there, compare it with the
first normal workstation SSH connection, and accept only a match. Never trust
or accept an `ssh-keyscan` result by itself.

Create one `*.<DOMAIN>` wildcard record using `A`/`AAAA` or `CNAME` as the DNS provider supports. It covers both the derived dashboard and app hosts; do not
create a separate `admin` record. Prove nonempty DNS resolution before
certificates without comparing answers to a public IP:

```sh
(dig +short A admin.<DOMAIN>; dig +short AAAA admin.<DOMAIN>) | grep -q .
(dig +short A onboarding-check.<DOMAIN>; dig +short AAAA onboarding-check.<DOMAIN>) | grep -q .
```

The operator supplies no extra certificate input. From the VPS root shell:

```sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-host.sh | sh
```

The copy/paste installer verifies checksums and a pinned Ed25519 signature
before installation. It downloads only its baked exact release and follows
HTTPS-only asset redirects. [Inspect the exact immutable beta
release](https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v0.1.6)
after it is published and before running if preferred. Transfer the credential only through verified root SSH to
`/root/.config/tinkercloud/resend-api-key`; make it a root-owned regular file at
mode `0600`, then give setup only that path. After the credential is ready, run:

```sh
sudo tinkercloud setup
```

Setup asks, in order, for the base
domain, operator email, verified Resend sender email, and that key-file path.
Setup derives `https://admin.<domain>/login`. Sign in there with one human browser
OTP only when there is no reusable browser identity, then use **Deployers** →
**Active deployer allowlist** to enter the reviewed normalized emails in
**Allowed deployer emails**. Check **I confirm that adding any email grants
deployment authority.**, then select **Save active deployers**. Replace the initial active deployer allowlist
with exactly the intended normalized deployer email set and no other addresses;
the operator email alone is sufficient only when that is the exact intended
set. Operator setup permits at most one human browser OTP, uses zero when a
reusable browser identity is valid, and uses no CLI deployer OTP. After saving
the exact deployer set, verify operator completion:

```sh
sudo tinkercloud status
sudo tinkercloud doctor
curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https://admin.<domain>/api/v1/version
```

The final HTTPS request must not follow a redirect; record its included gateway headers and a response body bounded to 32768 bytes containing bounded API-version JSON and record the no-redirect `https://admin.<domain>/api/v1/version` gateway proof. Protected-app anonymous denial belongs to deployer
deployment completion once an app exists; do not fabricate it during empty
operator setup. The external mail guide
is optional troubleshooting—not required for this basic Resend beta path.

<!-- beta-operator-readme:end -->
### 2. Deployer: publish one owner-only app

You need a macOS/Linux workstation, the static project directory, and the
operator-authorized deployer email. From the selected static project root,
install the exact client and deploy:

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-client.sh | sh
tinker version
tinker deploy .
```

The deploy wizard discovers or reuses the verified endpoint and deployer email;
it requests one human CLI OTP only when the saved bearer is missing,
unauthorized, or expired. The first deployment stays owner-only by default.
If that CLI authentication or OTP attempt fails, is malformed, times out, or is
denied, stop that deploy attempt: do not retry login, use `--force`, switch
identity, log out, clear credentials, or seek another OTP path. A valid exact
server-scoped saved identity uses zero OTP, and any eligible OTP is entered only
in the CLI, never chat.
After final review, invoke `tinker deploy .` exactly once for that deploy
attempt. Any outcome ends the invocation: do not rerun deploy, upload another
release, or retry from chat. The CLI's bounded readiness retries stay inside
that invocation; `active_but_unverified` permits only its independent exact-URL
recheck. A later attempt needs an explicit new human request after the cause is
addressed.
Success records the immutable deployment ID, protected exact app origin,
authenticated platform-health success, and anonymous HTML, asset, and reserved
API denial with no app bytes. Across both tracks there are at most two human OTP
requests in total: one human browser OTP and one human CLI OTP. Before issuing,
relaying, requesting, or suggesting a third human OTP, stop immediately.

Optional: add SDK capabilities only when the app needs current viewer/app
information, KV, blobs, or realtime:

```sh
npm install @tinkercloud/sdk@0.1.6
```

The defining guarantee is stronger than “apps include authentication”:

> No private app file, platform API, WebSocket, blob, or backend request is
> reachable until Tinkercloud has authenticated and authorized the request.

Tinkercloud includes a Go gateway, deployer CLI, SQLite-backed capabilities,
browser SDK, and local operational commands. It is pre-release software: use it
for replaceable toy, prototype, and utility apps—not business-critical data.

## Install the Tinker CLI

Tinker is installed directly from an exact GitHub release. Beta installation
never follows a mutable `latest` channel.

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-client.sh | sh
```

The installer is bound to that exact immutable release. It selects the macOS or
Linux binary for the local architecture, verifies its checksum and Ed25519
signature, and installs `tinker` into a safe per-user directory. It adds that
directory to a supported shell profile only when needed. Run it as the current
user, never with `sudo`, then open a new terminal. If it reports an unsupported
shell profile, add the printed directory to `PATH` yourself:

When `$HOME/.local/bin` is already on `PATH`, the `v0.1.6` installer preserves
the selected safe client install directory rather than replacing it through a
shell-variable collision.

```sh
tinker version
```

Package-manager users may install the same reviewed native CLI matrix with one
command:

```sh
npm install --global @tinkercloud/cli@0.1.6
```

## Operator first: run the platform

After the root-shell install, run the guided, resumable setup. It asks only for
the base domain, operator email, verified Resend sender, and a root-readable
file containing the Resend key. It creates the private HMAC material itself:

```sh
sudo tinkercloud setup
sudo tinkercloud status
sudo tinkercloud doctor
```

Installation places the verified server binary and systemd unit. Setup persists
the resulting state and can be rerun after an external DNS or email prerequisite
is fixed. For deterministic automation, `tinkercloud init --non-interactive`
remains available with explicit flags and protected secret files.
Use `--email-provider postmark`, `sendgrid`, or `smtp` when another outbound
adapter is required. Provider-specific variables in the root-owned systemd
environment file select the first configured adapter; generated YAML records
only the sender. The complete preparation, initialization, switching, and
troubleshooting workflow is in the
[outbound mail setup guide](docs/operations/mail-setup.md).

### Optional: operate from a workstation over SSH

The `tinker` CLI can also administer a VPS over the existing root SSH trust
boundary. This is optional; it is useful when the operator already has the CLI
installed on a workstation:

```sh
tinker host install root@HOST
tinker host status root@HOST
tinker host doctor root@HOST
tinker host update root@HOST
tinker host uninstall root@HOST
```

The released CLI derives the exact immutable GitHub release URL from its own
signed build version. A local development build instead requires the explicit
`--release-base` form.

`tinker host update` invokes the server's signed self-updater. The host verifies
the complete compatibility evidence, restarts the service, runs health and
anonymous-denial checks, and automatically restores the previous binary if a
gate fails. Tinker uses a fixed SSH command grammar; it stores no root
credential and exposes no arbitrary remote shell.

`tinker host uninstall` requires confirmation of the exact SSH target. It
permanently removes Tinkercloud configuration, credentials, apps, data,
service, binary, and service identity while preserving the ACME cache so a
manual reinstall does not request the same certificates again.

### Optional: safe automatic server updates

Automatic updates are off by default. An operator can opt into one signed
channel; the timer accepts only a strictly newer compatible release, verifies
the full signed evidence, health-checks it, and rolls back on failure. It never
replaces configuration, credentials, apps, app data, sessions, or ACME state:

```sh
sudo tinkercloud updates enable --channel beta
sudo tinkercloud updates status
# Disable future checks without changing installed state:
sudo tinkercloud updates disable
```

`status` reports redacted local health for SQLite, disk, permissions, clock,
service state, listeners, version, and rollback state. `doctor` performs bounded
DNS, TLS, and read-only provider checks without sending mail or printing
secrets. Updates use the configured signed release source, restart the service,
and restore the prior version automatically if a gate fails. Read the
[Hetzner deployment guide](docs/operations/hetzner-deployment.md),
[setup scenarios](docs/operations/setup-scenarios.md), and
[VPS smoke acceptance](docs/operations/vps-e2e.md) before operating a host.

## Deployer second: publish an app

From a static app project, deploy the project directory—not only its output
folder:

```sh
tinker deploy .
```

On its first human run, Tinker asks for the HTTPS platform URL only when it has
no verified default, reuses or establishes the deployer identity, inspects the
project, asks only about ambiguous required state, and creates a missing
`tinker.yaml` as a reviewed receipt. Use `tinker init .` to create the manifest
ahead of time. Later commands reuse the saved default server and verified CLI
bearer; `tinker logout` revokes and removes that local bearer.

For local app development:

```sh
tinker dev
```

The local subset includes current viewer/app information, KV, bounded document
collections, snapshot recovery, and live change hints. It deliberately excludes
production login, policy, deployment, blobs, TLS/protection proof, and
LLM/provider capabilities. Local success is development evidence only.

### Install the TypeScript SDK

Apps served by Tinkercloud use one browser-first ESM package for viewer and app
identity, capability discovery, KV, collections, private blobs, and ephemeral
realtime. Use the fixed public beta version with the package manager already
used by the app after `v0.1.6` is published:

```sh
npm install @tinkercloud/sdk@0.1.6
pnpm add @tinkercloud/sdk@0.1.6
yarn add @tinkercloud/sdk@0.1.6
bun add @tinkercloud/sdk@0.1.6
deno add npm:@tinkercloud/sdk@0.1.6
```

All five commands consume the same reviewed npm artifact; there is no separate
package-manager build. Install the current exact SDK from its signed GitHub
prerelease with:

```sh
npm install https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/tinkercloud-sdk-0.1.6.tgz
```

Use the SDK only inside an app served by Tinkercloud:

```ts
import { tinker } from "@tinkercloud/sdk";

const current = await tinker.user.current();
const capabilities = await tinker.capabilities.list();

console.log(current.identity.email);
console.log(current.app.slug);
console.log(capabilities);
```

The SDK uses the current app's same-origin browser session. It accepts no app
ID, deployer token, database credential, provider key, or endpoint secret. Pin
the exact numeric version when a build must remain reproducible. See the [client SDK guide](docs/architecture/client-sdk.md) for
the complete API and compatibility model.

## What Tinkercloud protects

- Every app has an isolated origin and storage namespace.
- Gateway authorization precedes app files, APIs, WebSockets, blobs, and backend
  requests.
- V1 blobs are private, local, bounded, and gateway-authorized.
- Operator-governed LLM access uses server-side provider credentials; secrets
  never enter deployed browser code.
- Revoked sessions, deployers, policies, and apps take effect on the next
  relevant request.

## User documentation

- [Host your first private app](docs/getting-started/first-app.md)
- [Client SDK](docs/architecture/client-sdk.md)
- [V1 scope](docs/product/v1-scope.md)
- [System architecture](docs/architecture/system.md)
- [Security model](docs/security/threat-model.md)
- [Technology decisions](docs/decisions/README.md)
- [Browsable product concept](concept/index.html)
- [Complete operator flow](concept/flows/operator.html)
- [Complete deployer flow](concept/flows/deployer.html)
- [Full documentation map](docs/README.md)

Governance and implementation scope remain canonical in
[PRINCIPLES.md](PRINCIPLES.md) and [PRD.md](PRD.md). Contributor and agent
maintenance material lives under [`internals/`](internals/README.md) and
[`skills/`](skills/).

## License and community

Apache-2.0. See [LICENSE](LICENSE) and the repository-owned
[asset provenance](ASSET_PROVENANCE.md).

Read [CONTRIBUTING.md](CONTRIBUTING.md), the [Code of Conduct](CODE_OF_CONDUCT.md),
[security policy](SECURITY.md), [support guide](SUPPORT.md), and
[governance model](GOVERNANCE.md) before contributing or requesting support.
