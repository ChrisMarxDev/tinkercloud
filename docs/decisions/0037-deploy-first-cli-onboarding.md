# 0037: Deploy-first local manifest onboarding

**Status:** Accepted for V1

## Context

Tinkercloud's deployer workflow should start from a completed local app rather
than require deployers to learn a separate manifest ceremony. At the same time,
`tinker.yaml`, filesystem paths, CLI credentials, and server selection form a
local trust boundary: convenience cannot turn automation into hidden prompts,
write outside the project, or accept an unproven bearer.

## Decision

`tinker init [DIR]` is the explicit manifest creator. `tinker deploy [DIR]` uses
the current directory by default and, for a human invocation only, runs the
same bounded setup when `tinker.yaml` is absent. It atomically writes a strict,
deterministically generated V1 manifest without overwriting an existing file.
The wizard inspects the project first. It uses a valid directory-derived slug
and one unambiguous conventional output directory without asking. It asks for a
slug or output only when the safe inference is invalid or ambiguous. Owner-only
private access is the default. A single review/edit checkpoint keeps
description, combined email/domain access rules, capabilities, and fallback
optional rather than turning each field into a question. Access broadening
is named by the same final deploy action. Manifest creation is a consequence of
that action, not a second confirmation.

The generated manifest is a durable receipt and automation interface, not
prerequisite paperwork. Every prompt must unlock a required decision that
cannot be discovered or safely defaulted.

All output/fallback components are checked beneath a non-symlinked project with
`Lstat`; traversal, symlinks, wrong file types, and unsafe paths deny before
archive creation. JSON mode is fully non-interactive and leaves missing local
prerequisites untouched.

Before a human deployment uses a saved CLI bearer, it performs the existing
version-plus-`whoami` proof. Only a definite unauthorized result may fall back
to OTP, and only a newly verified bearer is stored. `--server` remains an
invocation-local override.

## Consequences

Deployers can build an app and run one deploy command, while CI/agents retain a
strict non-prompting contract. The CLI makes an additional authenticated check
before human deployments; this is intentional fail-closed evidence, not a
session cache. The manifest generator and parser remain one contract, so a
wizard cannot emit a form the server would not accept. Adding future questions
requires evidence that inference/defaulting would be unsafe.
