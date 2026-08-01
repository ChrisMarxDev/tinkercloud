# ADR 0010: Generic-first, self-contained agent skills

**Status:** Accepted for V1; role-package split superseded by ADR 0042

## Context

Coding agents are a primary Tinkercloud development audience. Specialized skills
must be easy to install independently without letting shared security guidance
drift.

## Decision

Author `tinkercloud-platform` first. Copy relevant marked shared sections into
standalone role skills that never require another skill at runtime. ADR 0042
subsequently consolidates app development and deployment into `tinkercloud-deployer`
beside `tinkercloud-operator`.

Use portable Markdown and scripts, packaged as Codex-compatible `SKILL.md`
skills first. A generator/check command owns copied blocks, and CI fails when
they drift.

## Consequences

- One installed skill contains all context needed for its task.
- Shared rules are intentionally duplicated in release artifacts.
- The source markers and drift check become release-critical tooling.
- Other agent formats can be added without redesigning platform guidance.
