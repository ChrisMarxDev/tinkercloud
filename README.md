# Tinkercloud

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Tinkercloud is a self-hosted private micro-app platform: one operator runs one
gateway, deployers publish small apps, and viewers authenticate by email.

The defining guarantee is stronger than “apps include authentication”:

> No private app file, platform API, WebSocket, blob, or backend request is
> reachable until Tinkercloud has authenticated and authorized the request.

Tinkercloud includes a Go gateway, deployer CLI, persistent SQLite-backed
capabilities, browser SDK, and local operational commands. The documents define
the security boundaries and release evidence required for V1 operation.

`PRD.md` is the canonical implementation scope. Topic documents provide deeper
detail but must not silently expand or contradict it.

> [!WARNING]
> Tinkercloud is pre-release software. Interfaces and operational procedures may
> change, and V1 intentionally has no operator backup system. Use it only for
> replaceable toy, prototype, and utility apps—not business-critical data.

## Development quick start

The repository currently expects Task 3.38+, Go 1.25.12, Node.js 22, npm, a
POSIX shell, OpenSSL, and Python 3. On macOS, release scripts also require GNU
`sha256sum`.

```sh
git clone https://github.com/ChrisMarxDev/tinkercloud.git
cd tinkercloud

task setup
task -l
task build
task check
```

Use `task -l` to see all maintainer workflows. The core commands are `task
build` (local server, deployer CLI, SDK, and examples) and `task check` (the
local CI-equivalent verification suite). Focused tasks such as `task test`,
`task lint`, `task security`, and `task release:check` are also available.
These are local evidence commands; none deploy, publish, or install production
software. Signed artifact creation requires explicit `VERSION`, `OUTPUT`, and
`TINKERCLOUD_RELEASE_SIGNING_KEY` inputs:

```sh
task release:build VERSION=0.1.0 OUTPUT=./dist/0.1.0
```

The explicitly gated `task vps:e2e` mutates a disposable VPS only after its
acknowledgement and pinned host-key inputs are present. Read
[VPS E2E](docs/operations/vps-e2e.md) before using it. Local build and test
commands are development checks, not production deployment evidence.
The supported server workflow and its prerequisites are documented in
[Hetzner deployment](docs/operations/hetzner-deployment.md).
Stable CLI publication and its currently gated one-time setup are documented
in [CLI distribution](docs/operations/cli-distribution.md). Until those gates
are completed and the public channels are anonymously verified, install
commands remain maintainer templates rather than release claims.

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
16. [Complete operator flow](concept/flows/operator.html)
17. [Complete deployer flow](concept/flows/deployer.html)
18. [Conversation decision compact](docs/product/conversation-decisions-2026-07.md)
19. [Flow necessity audit](docs/product/flow-necessity-audit-2026-07.md)
20. [Local app development with `tinker dev`](docs/getting-started/local-emulator.md)
21. [Implemented post-V1 LLM chat capability](docs/product/llm-chat-capability-plan.md)

## Deployer quick start

From a static app project, deploy the project directory (not just its output
folder). Tinker reuses valid existing output; when none exists it stops with the
project-owned build action rather than running it:

```sh
tinker deploy .
```

On its first human run, Tinker asks for the HTTPS platform URL only when it has no
verified default, reuses or establishes the deployer identity, inspects the
project, asks only about ambiguous required state, and creates a missing
`tinker.yaml` as a reviewed receipt. Use `tinker init .` to create the manifest
ahead of time. Later commands reuse the saved default server and verified CLI
bearer; `tinker logout` revokes and removes that local bearer.

## Local app development

Run `tinker dev` from a static app project for a loopback-only development
server with project-local SQLite state:

```sh
tinker dev
```

The supported local subset includes current viewer/app information, KV,
bounded document collections, collection snapshot recovery, and live KV and
collection change hints. It deliberately excludes production login, policy,
deployment, blobs, TLS/protection proof, and LLM/provider capabilities. Local
success is development evidence only; a real deployment must still pass the
gateway, activation, and anonymous-denial gates.

## Repository shape

The runtime is implemented as a Go gateway and local operator CLI. Runtime
directories are created by `tinkercloud init`; they are not committed to the
repository.

```text
tinker/
├── AGENTS.md
├── PRD.md
├── PRINCIPLES.md
├── README.md
├── cmd/                    # tinkercloud and tinker entry points
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

The target human path is `sudo tinkercloud setup`: discover the host, ask only for
the controlled base domain, initial operator email, and necessary protected
provider credential source, then generate the config and resume across external
DNS/email checkpoints. Strict `tinkercloud init --non-interactive` remains the
automation contract. `tinkercloud status` is safe for offline recovery and reports
local, redacted health (SQLite integrity, disk, permissions, clock, service,
listeners, version, and update rollback state). `sudo tinkercloud doctor`
additionally performs bounded DNS, TLS, and read-only Resend credential checks
using the root-only systemd credential file; it never sends mail or prints
secrets, credential paths/references, or provider response bodies. Updates use
the configured signed release source, derive public-health and
anonymous-denial evidence from installed state, restart the service, and
restore the prior version automatically if any gate fails. An alternate release
source is an explicit advanced override.

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
  capability-model north star, while Tinkercloud deliberately uses a stricter
  self-hosted, per-app authorization model.
- V1 is a modular Go monolith distributed as one self-contained `tinkercloud`
  server binary plus a separate small `tinker` deployer CLI.
- V1 supports static apps, current-user/capability APIs, a deliberately small
  JSON key-value store, bounded reactive JSON document collections, lightweight
  app-shared local blobs, and ephemeral app-scoped realtime channels.
- The TypeScript client SDK is a first-class V1 product surface, not an optional
  wrapper around raw HTTP.
- Remote blob backends, public object URLs, and durable or multi-node realtime
  are post-V1. V1 blobs stay private, local, bounded, and gateway-authorized.
- Backend processes, arbitrary containers, custom app domains, and clustering
  are not V1 work.
- The first deployment targets are clean, dedicated Hetzner Cloud Ubuntu 24.04
  LTS and Ubuntu 26.04 LTS x86-64 VPS instances.
- Normal embedded SQLite, the local filesystem, Resend, and automatic per-host
  TLS are the default operational dependencies. One control database owns
  authorization and platform state; each app has a physically isolated
  `apps/{immutable-app-id}/data.db` for KV and document collections.
- App-scoped opaque sessions are preferred over shared parent-domain cookies.
- Backups are an advanced feature, not V1 scope. Signed self-updates still keep
  a narrow local rollback snapshot for upgrade recovery.
- The first operator-governed LLM chat capability is an implemented post-V1
  extension: encrypted write-only Anthropic/Gemini connections, app grants,
  bounded non-streaming `tinker.llm.chat.complete`, quotas, audit, and SDK
  support. It does not expand locked V1; streaming and broader provider
  integrations remain deferred. Operator secrets never enter deployed browser
  code.

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
