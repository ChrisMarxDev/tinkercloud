# Coding-Agent Skill System

Agent skills are a first-class product surface. They make the SDK, security
model, deployment workflow, and verification rules available to coding agents
without relying on a long prompt or scattered documentation.

This follows the [Shopify Quick north star](../product/north-star-quick.md):
agents receive the platform skills out of the box rather than expecting staff
to read and relay API documentation.

## Planned skill packages

```text
skills/
├── tiny-platform/                 # authored first; shared canonical content
│   ├── SKILL.md
│   └── references/
│       ├── platform-model.md
│       ├── security-boundary.md
│       └── compatibility.md
├── tiny-app-development/
│   ├── SKILL.md
│   ├── references/
│   │   ├── sdk.md
│   │   ├── manifest.md
│   │   ├── capabilities.md
│   │   ├── security.md
│   │   └── errors.md
│   └── scripts/
│       └── verify-app
├── tiny-deploy/
│   ├── SKILL.md
│   └── references/
│       ├── authentication.md
│       ├── access-policy.md
│       └── troubleshooting.md
└── tiny-operator/
    ├── SKILL.md
    └── references/
        ├── hetzner-install.md
        ├── update-recovery.md
        └── security-diagnostics.md
```

## `tiny-app-development`

Teaches an agent to:

1. inspect the app goal and data sensitivity;
2. use `@tinyhost/sdk` instead of inventing backend/auth/storage code;
3. discover enabled capabilities;
4. create a secure `tiny.yaml`;
5. handle typed SDK errors and quotas;
6. never embed secrets, app IDs, or deployer tokens;
7. build a static artifact;
8. run local contract checks.

It includes KV state-recovery, lightweight blob, and ephemeral realtime
guidance: socket events are hints, current KV state is authoritative, and blobs
use opaque IDs plus bounded app-shared local storage rather than paths, mounts,
buckets, public URLs, or browser credentials.

## `tiny-deploy`

Teaches an agent to:

1. identify the build output;
2. authenticate using a scoped non-interactive token;
3. preview manifest, access rules, and capability grants;
4. deploy an immutable release;
5. wait for terminal activation/TLS state;
6. independently verify anonymous HTML, asset, API, and socket denial;
7. verify the expected authenticated path;
8. return app URL, deployment ID, policy summary, and known limits.

The skill must never turn a failed denial probe into a warning.

## `tiny-operator`

Teaches an agent to:

- initialize the supported Hetzner host;
- interpret `tinyhost doctor`;
- authorize deployers and capability connections;
- apply signed updates;
- use root-only recovery;
- avoid exposing secrets in commands, output, or logs.

Consequential operator actions remain human-confirmed.

## Generic-first, standalone rule

`tiny-platform` is authored first. Each specialized skill then copies the
relevant shared sections so it remains useful when installed alone; it must not
depend on a sibling skill being present. Shared blocks carry stable markers, and
a generator/check command refreshes them and makes CI fail on drift.

All skills are generic Markdown plus scripts. Codex-compatible `SKILL.md`
packaging is delivered first, without coupling the content to one agent vendor.

Copied content and generated references are checked against:

- SDK exported types and examples;
- `tiny.yaml` schema;
- HTTP/capability contracts;
- CLI help and machine-readable output schema;
- security test matrix;
- current server/client compatibility policy.

CI fails when documented skill commands, SDK examples, capability names, or
manifest fields drift from their contracts.

## Skill release policy

- Version skills with the compatible TinyHost API/SDK release.
- Embed a compatible copy in server docs and publish installable copies.
- Keep the core `SKILL.md` concise; route detailed material into references.
- Include deterministic verification scripts rather than prose-only checks.
- Test skills through representative agent tasks before release.

## Definition of complete

A capability is not agent-ready until:

- SDK method and types exist;
- capability discovery describes it;
- manifest/grant behavior is documented;
- security and quota behavior is explicit;
- runnable examples pass;
- relevant skill references and verification are updated.
