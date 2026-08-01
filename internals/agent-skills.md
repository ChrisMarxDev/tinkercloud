# Coding-Agent Skill System

Agent skills are a first-class product surface. They make the SDK, security
model, deployment workflow, and verification rules available to coding agents
without relying on a long prompt or scattered documentation.

This follows the [Shopify Quick north star](../docs/product/north-star-quick.md):
agents receive the platform skills out of the box rather than expecting staff
to read and relay API documentation.

## Planned skill packages

```text
skills/
├── tinkercloud-platform/                 # authored first; shared canonical content
│   └── SKILL.md
├── tinkercloud-deployer/
│   └── SKILL.md
├── tinkercloud-operator/
│   └── SKILL.md
├── distribute-tinker-cli/           # maintainer CLI release workflow
│   └── SKILL.md
└── distribute-tinkercloud-sdk/           # maintainer SDK registry workflow
    └── SKILL.md
```

## `tinkercloud-deployer`

Teaches an agent to:

1. inspect the app goal, current project, and verified CLI state before asking;
2. use `@tinkercloud/sdk` instead of inventing backend/auth/storage code;
3. propose owner-only, exact-email, and exact-domain access choices;
4. discover and deliberately enable capabilities;
5. create or validate a secure `tinker.yaml`;
6. handle typed SDK errors, cancellation, quotas, and realtime recovery;
7. build a static artifact with the project's existing toolchain;
8. authenticate without moving credentials through chat;
9. deploy an immutable release; and
10. independently verify anonymous HTML, asset, API, and socket denial.

It includes KV state-recovery, lightweight blob, and ephemeral realtime
guidance: socket events are hints, current KV state is authoritative, and blobs
use opaque IDs plus bounded app-shared local storage rather than paths, mounts,
buckets, public URLs, or browser credentials.

The deployer receives one complete app lifecycle skill that can be pasted or
referenced by raw URL. It must never turn a failed denial probe into a warning.

## `tinkercloud-operator`

Teaches an agent to:

- initialize the supported Hetzner host;
- interpret `tinkercloud doctor`;
- authorize deployers and capability connections;
- apply signed updates;
- use root-only recovery;
- avoid exposing secrets in commands, output, or logs.

Consequential operator actions remain human-confirmed.

## Maintainer distribution skills

`distribute-tinker-cli` prepares and, only with explicit authorization, publishes
the signed native CLI through the reviewed installer, one npm-family package,
and Homebrew. `distribute-tinkercloud-sdk` keeps npm, JSR, exported versions,
compatibility ranges, examples, and the signed SDK tarball aligned.

Both distinguish read-only inspection, local preparation, and external
publication. Locked names and placeholder release origins allow rehearsal but
do not claim registry or tap availability. One explicit path permits a GitHub
beta through the protected `beta-release` workflow; it publishes the complete
signed prerelease but never npm, JSR, Homebrew, or stable/latest state. Neither
skill embeds an alternate signer or publisher; repository release tooling and
contracts remain authoritative.

## Generic-first, standalone rule

`tinkercloud-platform` is authored first. Each role skill then copies the relevant
common and role-specific sections so it remains useful when its `SKILL.md` is
the only Tinkercloud document available. Shared blocks carry stable markers, and a
check command makes CI fail on drift.

All skills are generic Markdown plus scripts. Codex-compatible `SKILL.md`
packaging is delivered first, without coupling the content to one agent vendor.

Copied content and generated references are checked against:

- SDK exported types and examples;
- `tinker.yaml` schema;
- HTTP/capability contracts;
- CLI help and machine-readable output schema;
- security test matrix;
- current server/client compatibility policy.

CI fails when documented skill commands, SDK examples, capability names, or
manifest fields drift from their contracts.

## Skill release policy

- Version skills with the compatible Tinkercloud API/SDK release.
- Embed a compatible copy in server docs and publish installable copies.
- Keep each role `SKILL.md` compact enough to paste while retaining all
  standalone safety and workflow knowledge.
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
