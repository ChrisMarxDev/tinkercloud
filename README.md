# Tinkercloud

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Tinkercloud is a self-hosted private micro-app platform: one operator runs one
gateway, deployers publish small apps, and viewers authenticate by email.

The defining guarantee is stronger than “apps include authentication”:

> No private app file, platform API, WebSocket, blob, or backend request is
> reachable until Tinkercloud has authenticated and authorized the request.

Tinkercloud includes a Go gateway, deployer CLI, SQLite-backed capabilities,
browser SDK, and local operational commands. It is pre-release software: use it
for replaceable toy, prototype, and utility apps—not business-critical data.

## Operator first: run the platform

The target operator path is `sudo tinkercloud setup`: discover the host, ask
only for the controlled base domain, initial operator email, and necessary
protected provider credential source, then generate configuration and resume
across external DNS/email checkpoints.

```sh
sudo tinkercloud setup
sudo tinkercloud status
sudo tinkercloud doctor
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

Apache-2.0. See [LICENSE](LICENSE).

Read [CONTRIBUTING.md](CONTRIBUTING.md), the [Code of Conduct](CODE_OF_CONDUCT.md),
[security policy](SECURITY.md), [support guide](SUPPORT.md), and
[governance model](GOVERNANCE.md) before contributing or requesting support.
