# TinyHost

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

TinyHost is a self-hosted private micro-app platform: one operator runs one
gateway, deployers publish small apps, and viewers authenticate by email.

The defining guarantee is stronger than “apps include authentication”:

> No private app file, platform API, WebSocket, blob, or backend request is
> reachable until TinyHost has authenticated and authorized the request.

TinyHost includes a Go gateway, deployer CLI, persistent SQLite-backed
capabilities, browser SDK, and local operational commands. The documents define
the security boundaries and release evidence required for V1 operation.

`PRD.md` is the canonical implementation scope. Topic documents provide deeper
detail but must not silently expand or contradict it.

> [!WARNING]
> TinyHost is pre-release software. Interfaces and operational procedures may
> change, and V1 intentionally has no operator backup system. Use it only for
> replaceable toy, prototype, and utility apps—not business-critical data.

## Development quick start

The repository currently expects Go 1.25.12, Node.js 22, npm, a POSIX shell,
OpenSSL, and Python 3. On macOS, release scripts also require GNU
`sha256sum`.

```sh
git clone https://github.com/ChrisMarxDev/tiny.git
cd tiny

go test ./...
go vet ./...

cd sdk/typescript
npm ci
npm test
```

Run the offline security gates from the repository root:

```sh
./scripts/ci-security-gates.sh
```

Build the two Go entry points locally:

```sh
mkdir -p bin
go build -o bin/tinyhost ./cmd/tinyhost
go build -o bin/tiny ./cmd/tiny
```

These commands are development checks, not production deployment evidence.
The supported server workflow and its prerequisites are documented in
[Hetzner deployment](docs/operations/hetzner-deployment.md).

## Start here

1. [Human setup: host your first private app](docs/getting-started/first-app.md)
2. [Core principles](PRINCIPLES.md)
3. [Canonical implementation PRD](PRD.md)
4. [First-release definition](docs/product/v1-scope.md)
5. [System architecture](docs/architecture/system.md)
6. [Component map](docs/architecture/components.md)
7. [Quick north star](docs/product/north-star-quick.md)
8. [Security model](docs/security/threat-model.md)
9. [Delivery roadmap](docs/delivery/roadmap.md)
10. [AI development loop](docs/delivery/ai-development-loop.md)
11. [Client SDK](docs/architecture/client-sdk.md)
12. [Future capability broker](docs/product/capability-broker.md)
13. [Agent skill system](docs/delivery/agent-skills.md)
14. [Technology decisions](docs/decisions/README.md)
15. [Browsable concept](concept/index.html)

## Repository shape

The runtime is implemented as a Go gateway and local operator CLI. Runtime
directories are created by `tinyhost init`; they are not committed to the
repository.

```text
tiny/
├── AGENTS.md
├── PRD.md
├── PRINCIPLES.md
├── README.md
├── cmd/                    # tinyhost and tiny entry points
├── internal/               # private gateway and capability packages
├── sdk/                    # browser SDK
├── docs/
│   ├── architecture/
│   ├── decisions/
│   ├── delivery/
│   ├── product/
│   └── security/
├── specs/                  # technology-neutral contracts
├── test/                   # test charters before test implementation
└── concept/                # offline product concept and feature ranking
```

## Local operator workflow

`tinyhost init --config … --operator-email …` initializes private state and
the first operator. `tinyhost status` is safe for offline recovery and reports
local, redacted health (SQLite integrity, disk, permissions, clock, service,
listeners, version, and update rollback state). `sudo tinyhost doctor`
additionally performs bounded DNS, TLS, and read-only Resend credential checks
using the root-only systemd credential file; it never sends mail or prints
secrets, credential paths/references, or provider response bodies. Updates require a pinned signed artifact plus explicit public-health
and anonymous-denial probe URLs; the service is restarted after installation
and restored automatically if any gate fails.

## Working vocabulary

- **Operator** — owns the VPS, platform configuration, deployer access, and
  recovery.
- **Deployer** — publishes and manages apps, releases, and app access policy.
- **Viewer** — authenticates by email and can use only explicitly allowed apps.
- **Gateway** — the only public request entry point.
- **Control plane** — platform/admin/deployment operations.
- **App plane** — protected requests to app content and app-scoped capabilities.
- **Release** — an immutable uploaded bundle.
- **Activation** — the atomic selection of one release as current.

## Current decisions

- [Shopify Quick](https://shopify.engineering/quick) is the usability and
  capability-model north star, while TinyHost deliberately uses a stricter
  self-hosted, per-app authorization model.
- V1 is a modular Go monolith distributed as one self-contained `tinyhost`
  server binary plus a separate small `tiny` deployer CLI.
- V1 supports static apps, current-user/capability APIs, a deliberately small
  JSON key-value store, and ephemeral app-scoped realtime channels.
- The TypeScript client SDK is a first-class V1 product surface, not an optional
  wrapper around raw HTTP.
- Blob storage and durable or multi-node realtime are post-V1.
- Backend processes, arbitrary containers, custom app domains, and clustering
  are not V1 work.
- The first deployment targets are clean, dedicated Hetzner Cloud Ubuntu 24.04
  LTS and Ubuntu 26.04 LTS x86-64 VPS instances.
- SQLite, the local filesystem, Resend, and automatic per-host TLS are the
  default operational dependencies.
- App-scoped opaque sessions are preferred over shared parent-domain cookies.
- Backups are an advanced feature, not V1 scope. Signed self-updates still keep
  a narrow local rollback snapshot for upgrade recovery.
- Future LLM and internal-service integrations are server-side capability
  adapters. Operator secrets never enter deployed browser code.

## License

Apache-2.0. See [LICENSE](LICENSE).

## Contributing and community

Read [CONTRIBUTING.md](CONTRIBUTING.md) before proposing a change. The
[Code of Conduct](CODE_OF_CONDUCT.md), [security policy](SECURITY.md),
[support guide](SUPPORT.md), and [governance model](GOVERNANCE.md) describe
how the project is run.

The repository is still being prepared for public launch. The current audit,
remaining blockers, and GitHub settings checklist are tracked in
[open-source readiness](docs/delivery/open-source-readiness.md).
