# Tinkercloud

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Tinkercloud is a self-hosted platform for deploying small private web apps. An
operator runs one gateway, deployers publish useful apps, and viewers
authenticate by email.

> No private app file, platform API, WebSocket, blob, or backend request is
> reachable until Tinkercloud has authenticated and authorized the request.

Tinkercloud is pre-release software. Interfaces and operational procedures may
change, and V1 intentionally has no operator backup system. Use it only for
replaceable toy, prototype, and utility apps—not business-critical data.

## Operator first

The operator owns the VPS, platform configuration, deployer access, updates,
and recovery. Start here:

1. [Host the first private app](docs/getting-started/first-app.md)
2. [Hetzner deployment](docs/operations/hetzner-deployment.md)
3. [Operator flow](concept/flows/operator.html)
4. [Operations and recovery](docs/operations/)
5. [Security model](docs/security/threat-model.md)
6. [Release and update behavior](docs/operations/release-pipeline.md)

Tinkercloud is designed around one understandable production shape: one
self-contained server binary, one embedded SQLite engine, one data directory,
and one service on a dedicated VPS. The gateway owns TLS, hostname routing,
viewer authentication, policy, app files, capabilities, and deployment
activation.

## Deployer second

Deployers publish and manage their own private apps without implementing the
platform security boundary:

```sh
tinker deploy .
```

The first run establishes or reuses the verified platform connection, inspects
the project, creates a reviewed `tinker.yaml` receipt when needed, and returns
a protected URL after activation succeeds.

Read the [deployer flow](concept/flows/deployer.html), [local app development
guide](docs/getting-started/local-emulator.md), and [client SDK
guide](docs/architecture/client-sdk.md).

## What apps can use

The typed browser SDK provides a small, app-scoped capability surface:

- current viewer and app identity;
- bounded JSON KV and reactive collections;
- lightweight private blobs with server-enforced limits;
- ephemeral realtime notifications;
- capability discovery; and
- operator-governed, bounded LLM chat where enabled.

Capabilities are server-derived and authorized for every request. Operator
credentials never enter deployed browser code. Public object URLs, arbitrary
backend processes, external databases, multi-node clustering, and durable
realtime are outside the V1 boundary.

See the [SDK documentation](docs/architecture/client-sdk.md), [capability
contracts](specs/capabilities/), and [feature overview](concept/features/index.html).

## Security promise

Tinkercloud fails closed when state is missing, stale, malformed, unavailable,
or ambiguous. App identity comes from the validated hostname; viewer identity
comes from an opaque server-side session. App code cannot select either
identity, bypass the gateway, or receive provider credentials.

Read the [security model](docs/security/threat-model.md) and [core
principles](PRINCIPLES.md).

## Vocabulary

- **Operator** — hosts and operates Tinkercloud.
- **Deployer** — publishes and manages their own apps.
- **Viewer** — authenticates and uses explicitly allowed apps.
- **Gateway** — the only public request entry point.
- **Release** — an immutable uploaded app bundle.
- **Activation** — the atomic selection of one release as current.

## Support and project information

- [Support guide](SUPPORT.md)
- [Security policy](SECURITY.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Governance model](GOVERNANCE.md)
- [License](LICENSE)

Contributor setup, repository structure, agent workflows, CI details, and other
internal material live in [`internals/`](internals/README.md) and the relevant
[`skills/`](skills/). They are intentionally not part of this user guide.
