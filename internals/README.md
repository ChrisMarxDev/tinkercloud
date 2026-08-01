# Contributor and agent internals

This directory contains repository-maintenance material that is intentionally
separate from the user-facing README and `docs/`.

## Start here

1. Read [`../AGENTS.md`](../AGENTS.md), [`../PRINCIPLES.md`](../PRINCIPLES.md),
   and [`../PRD.md`](../PRD.md) before changing the product.
2. Read the relevant project skill under [`../skills/`](../skills/).
3. Use the smallest relevant verification command from
   [`development.md`](development.md).
4. Keep user-facing operator and deployer guidance in `README.md` or `docs/`.

## Internal references

- [Development and verification](development.md)
- [Repository map](repository-map.md)
- [Autonomous GitHub issue loop](../loop/README.md)
- [AI development loop](ai-development-loop.md)
- [Agent skill system](agent-skills.md)
- [Open-source readiness](open-source-readiness.md)
- [Canonical platform skill](../skills/tinkercloud-platform/SKILL.md)
- [Deployer skill](../skills/tinkercloud-deployer/SKILL.md)
- [Operator skill](../skills/tinkercloud-operator/SKILL.md)
- [Full-stack test skill](../skills/tinkercloud-full-stack-test/SKILL.md)

Contributor and agent instructions belong here or in a focused skill. Do not add
internal workflow notes, hidden implementation assumptions, or agent prompts to
the user-facing README or `docs/`.
