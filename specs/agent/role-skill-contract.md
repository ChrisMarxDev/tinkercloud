# TinyHost role-skill contract

## Outcome

TinyHost ships two self-contained role-facing coding-agent skills:

- `tiny-deployer` for understanding, building, configuring, deploying, and
  verifying a deployer's private static app; and
- `tiny-operator` for installing, configuring, diagnosing, updating, and
  recovering a TinyHost server.

`tiny-platform` remains the canonical authoring source for copied role guidance.
It is not a runtime dependency and does not create a third human role workflow.
Internal acceptance-test, TinyHost-owned UI, and opt-in local
deployer-workstation test skills may remain separate. The latter may drive the
normal CLI OTP flow only with an exact deployer's controlled local Resend test
mailbox; it is not production CI/noninteractive deployment-agent
authentication. A production deployment agent uses a separately provisioned
app-scoped deployer token and never an implicit credential write or prompt.
Maintainer-facing release work is also separate: the
[`distribution-skill contract`](distribution-skill-contract.md) governs the
CLI and client-package distribution skills without changing the two human role
workflows.

## Standalone distribution

Each role skill MUST remain useful when its `SKILL.md` is the only TinyHost
document an agent receives. It MUST NOT require a sibling skill, repository
checkout, hidden prompt, or remembered TinyHost knowledge.

Each role skill MUST:

- identify its actor and refuse to transfer authority from another role;
- state the V1 private-only and utility-grade persistence boundaries;
- route work through the supported `tiny` or `tinyhost` command surface;
- keep secrets out of chat, argv, app code, manifests, logs, and browser state;
- fail closed on missing, malformed, redirected, or ambiguous security state;
- report completion only after the role's required verification succeeds; and
- carry marked canonical blocks checked for drift.

## Deployer interaction

The deployer skill MUST start from the requested app outcome and inspect the
working project before asking questions. It asks only for a value or decision
that is required, unknown, undiscoverable, and unsafe to default.

The skill MUST:

1. discover or ask for the normalized HTTPS TinyHost platform URL;
2. distinguish app-building work from deployment-only work;
3. propose a concrete access policy and explain owner-only, exact-email, and
   exact-domain choices without inferring broader access;
4. combine unresolved optional description, access, capability, and SPA choices
   into one review step;
5. use `@tinyhost/sdk` as the app platform for current viewer/app information,
   capability discovery, KV, blobs, and realtime;
6. generate or validate a strict `tiny.yaml` receipt;
7. use the existing project-owned build action without making `tiny deploy`
   execute arbitrary builds;
8. authenticate through the CLI without requesting a bearer or OTP in chat;
9. run `tiny deploy [DIR]`; and
10. accept success only when the CLI's fresh anonymous gateway-denial proof
    succeeds.

When an authenticated deployer asks to inspect or repair an owned app's managed
KV/documents, the skill MUST use the bounded `tiny data` command family and
reuse the saved CLI login. It MUST explain the optional KV and required
destructive/document optimistic version checks plus exact destructive
confirmation, preserve deterministic `--json` behavior, and refuse
database URLs/files, raw SQL, schema/migration commands, exports/imports,
viewer impersonation, or another deployer's app. The app slug is an owned
control target only; TinyHost derives the private data scope.

The skill MAY continue safe local inspection or implementation while waiting
for a non-secret answer. It MUST stop before a deployment or access broadening
that still needs the deployer's decision.

## SDK knowledge

The deployer skill MUST document the current public SDK surface accurately
enough to build an app without repository documentation:

- same-origin, configuration-free `tiny`;
- `tiny.user.current`, `tiny.app.info`, and `tiny.capabilities.list`;
- versioned JSON KV get/set/delete/prefix-list;
- opaque app-shared blob upload/get/list/delete;
- app-scoped live channels and KV change hints;
- cancellation and typed failures; and
- KV reread after live connect/reconnect because realtime is ephemeral.

It MUST prohibit app IDs, API origins, deployer tokens, database credentials,
provider secrets, public blob URLs, durable event claims, and client-side auth
reimplementation.

## Operator interaction

The operator skill MAY point to the repository for implementation detail, but
its `SKILL.md` MUST still contain the safe setup, diagnostics, update, recovery,
deployer-allowlist, secret, listener, and V1 durability boundaries needed to
avoid dangerous guesses.

Consequential changes require the operator's explicit target and confirmation.
Root access remains the recovery authority; no remote recovery bypass is
introduced.

## Drift and packaging

The canonical file MUST expose separately marked common, deployer, and operator
blocks. The drift check MUST compare:

- common + deployer blocks with `tiny-deployer`;
- common + operator blocks with `tiny-operator`; and
- the common block with internal full-stack acceptance guidance.

Role skills SHOULD include Codex-compatible `agents/openai.yaml` metadata, but
their Markdown remains portable to agents that consume only a raw file or URL.
