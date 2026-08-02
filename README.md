# Tinkercloud

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

![Tinkercloud turns a folder into a protected team app](docs/assets/readme-deploy-flow.webp)

## Overview

Tinkercloud is a self-hosted private micro-app platform: one operator runs one
gateway, deployers publish small apps, and viewers authenticate by email.

For an operator on a fresh supported VPS root shell, installation is one exact
versioned command:

```sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/vVERSION/install-host.sh | sh
```

The installer downloads only its baked exact release, follows HTTPS-only asset
redirects, and verifies checksums plus Ed25519 signatures before it changes the
host. Replace `VERSION` with a published version. Run it only as root on a
clean Ubuntu 24.04 LTS or 26.04 LTS x86-64 VPS.

For a deployer, the shortest useful path is preview, then publish the current
project directory:

```sh
tinker dev
tinker deploy .
```

The first `tinker deploy .` asks only for required information it cannot safely
derive, including optional viewer emails or domains. With no viewer rule, the
app remains private to its owner.

The defining guarantee is stronger than “apps include authentication”:

> No private app file, platform API, WebSocket, blob, or backend request is
> reachable until Tinkercloud has authenticated and authorized the request.

Tinkercloud includes a Go gateway, deployer CLI, SQLite-backed capabilities,
browser SDK, and local operational commands. It is pre-release software: use it
for replaceable toy, prototype, and utility apps—not business-critical data.

## Install the Tinker CLI

Tinker is installed directly from an exact GitHub release. Replace `VERSION`
with a published beta version; beta installation never follows a mutable
`latest` channel.

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/vVERSION/install-client.sh | sh
```

The installer is bound to that exact immutable release. It selects the macOS or
Linux binary for the local architecture, verifies its checksum and Ed25519
signature, and installs `tinker` into a safe per-user directory. It adds that
directory to a supported shell profile only when needed. Run it as the current
user, never with `sudo`, then open a new terminal. If it reports an unsupported
shell profile, add the printed directory to `PATH` yourself:

```sh
tinker version
```

After the npm beta channel is published, package-manager users may install the
same reviewed native CLI matrix with one command:

```sh
npm install --global @tinkercloud/cli@next
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
tinker host update root@HOST --release-base https://github.com/ChrisMarxDev/tinkercloud/releases/download/vVERSION/
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

### Install the TypeScript SDK beta

Apps served by Tinkercloud use one browser-first ESM package for viewer and app
identity, capability discovery, KV, collections, private blobs, and ephemeral
realtime. During prerelease, opt in explicitly with the `beta` tag using the
package manager already used by the app:

```sh
npm install @tinkercloud/sdk@beta
pnpm add @tinkercloud/sdk@beta
yarn add @tinkercloud/sdk@beta
bun add @tinkercloud/sdk@beta
deno add npm:@tinkercloud/sdk@beta
```

All five commands consume the same reviewed npm artifact; there is no separate
package-manager build. If npm reports that the package is unavailable, the
first registry beta has not been published yet. Install an exact SDK from its
signed GitHub prerelease instead, replacing both `VERSION` values:

```sh
npm install https://github.com/ChrisMarxDev/tinkercloud/releases/download/vVERSION/tinkercloud-sdk-VERSION.tgz
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
an exact numeric version instead of `beta` when a build must remain
reproducible. See the [client SDK guide](docs/architecture/client-sdk.md) for
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
