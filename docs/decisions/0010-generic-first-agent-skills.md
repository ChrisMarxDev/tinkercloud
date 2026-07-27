# ADR 0010: Generic-first, self-contained agent skills

**Status:** Accepted for V1

## Context

Coding agents are a primary TinyHost development audience. Specialized skills
must be easy to install independently without letting shared security guidance
drift.

## Decision

Author `tiny-platform` first. Build `tiny-app-development`, `tiny-deploy`, and
`tiny-operator` by copying the relevant marked shared sections into each
package. Every specialized skill is standalone and never requires another
skill at runtime.

Use portable Markdown and scripts, packaged as Codex-compatible `SKILL.md`
skills first. A generator/check command owns copied blocks, and CI fails when
they drift.

## Consequences

- One installed skill contains all context needed for its task.
- Shared rules are intentionally duplicated in release artifacts.
- The source markers and drift check become release-critical tooling.
- Other agent formats can be added without redesigning platform guidance.
