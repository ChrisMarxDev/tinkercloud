# ADR 0042: Two role-facing agent skills

**Status:** Accepted for V1

## Context

The prior specialized packaging separated app development from deployment.
That split makes a deployer or deployment agent discover and combine two skills
before it can turn an app idea into a verified TinyHost URL. It also makes the
most important path less suitable for pasting one Markdown file or referencing
one URL in a fresh agent conversation.

## Decision

Keep `tiny-platform` as the canonical shared authoring source and ship two
self-contained role-facing skills:

- `tiny-deployer`, combining app understanding, `@tinyhost/sdk` development,
  manifest/access decisions, build, deployment, and denial verification; and
- `tiny-operator`, covering host installation and operation.

Remove the separate `tiny-app-development` and `tiny-deploy` role packages.
Internal full-stack-test and TinyHost-native-UI skills remain task-specific
engineering tools rather than human role packages.

The canonical source uses independently checked common, deployer, and operator
copy markers. Each role file must work from raw Markdown alone.

## Consequences

- A deployer can give an agent one URL or pasted file for the complete app
  lifecycle.
- The deployer skill can gather missing platform and access decisions in one
  coherent interaction.
- SDK guidance and deployment verification evolve together.
- The operator skill stays intentionally smaller and may route technical depth
  to this repository without losing its safety boundaries.
- Existing references to the two removed skill names must migrate to
  `tiny-deployer`.
