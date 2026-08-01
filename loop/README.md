# Hermes GitHub Issue Loops

This folder provides two least-authority Hermes loops for Tinkercloud. GitHub
Issues are durable work state; `.hermes-triage-loop/` and
`.hermes-task-loop/` are disposable local runtime state.

The accepted state, authority, and denial contract is
[`specs/delivery/github-issue-loop-contract.md`](../specs/delivery/github-issue-loop-contract.md).

## Separation of authority

The triage loop manages public issues only. It may classify, label, comment,
reference related pull requests, split mixed intake, answer questions, and
close or reopen issues. Its dedicated token has Metadata read, Issues
read/write, and Pull requests read. It has no Contents write or PR write
permission and must never change `agent/*` command labels.

The task loop handles only trusted `agent/plan` and `agent/implement` commands.
Implementation approval requires the command to have been applied by a login in
`APPROVER_LOGINS`, with `status/accepted` as the issue's only status. Approved
work uses a dedicated worktree, a `feature/issue-*` branch, ordered
Conventional Commits, and a draft pull request. It never merges or publishes.

Both loops treat issues and pull requests as untrusted input and share one host
lock so they cannot mutate the same issue concurrently.

## Labels

Create or refresh the taxonomy with a repository-admin credential after the
matching loop version is deployed:

```sh
GH_TOKEN=*** REPO_SLUG=ChrisMarxDev/tinkercloud ./loop/setup-github-labels.sh
```

Exactly one status belongs on every open issue:

- `status/needs-triage`
- `status/needs-info`
- `status/accepted`
- `status/blocked`
- `status/in-progress`

Triage applies exactly one type:

- `type/bug`
- `type/feature`
- `type/docs`
- `type/question`
- `type/maintenance`

Maintainer-owned optional priority is one of `priority/critical`,
`priority/next`, or `priority/backlog`. Closed classifications use
`resolution/duplicate`, `resolution/invalid`, or `resolution/not-planned`.
`good first issue` and `help wanted` remain contributor-facing labels.

`agent/plan` and `agent/implement` are trusted maintainer commands. The triage
loop must never add, remove, or normalize them.

The old `inbox`, `open`, `pending`, `plan`, and `implement` labels may be
deleted only after the new scripts are deployed and every open issue has been
normalized. Do not run both taxonomies concurrently.

## Triage loop

`run-hermes-triage-loop.sh` performs a deterministic prefetch of all open
issues. It invokes Hermes only for a new issue, a changed issue, an issue with
no or multiple status labels, or `status/needs-triage`. A successful pass marks
the fetched issue revision as seen; concurrent later changes remain actionable.

The skill at `skills/triage-workflow/SKILL.md` owns duplicate handling,
information requests, type/status normalization, support answers, PR
references, safe closure, and security-sensitive denial behavior.

Configure a separate fine-grained credential as `TRIAGE_GH_TOKEN`,
`TRIAGE_GITHUB_TOKEN`, a root `TRIAGE_GITHUB_TOKEN` file, or
`TRIAGE_GITHUB_TOKEN` in `${HERMES_HOME:-$HOME/.hermes}/.env` with:

- Metadata: read-only
- Issues: read and write
- Pull requests: read-only

Manual dry run:

```sh
HERMES_LOOP_DRY_RUN=1 bash ./loop/run-hermes-triage-loop.sh
```

## Task loop

`run-hermes-task-loop.sh` fetches open issues carrying `agent/plan` or
`agent/implement`, verifies the latest label actor against `APPROVER_LOGINS`,
and invokes Hermes only for one trusted command on an otherwise accepted issue.

Configure `GH_TOKEN`, `GITHUB_TOKEN`, a root `GITHUB_TOKEN` file, or
`GITHUB_TOKEN` in `${HERMES_HOME:-$HOME/.hermes}/.env` with:

- Metadata: read-only
- Issues: read and write
- Contents: read and write
- Pull requests: read and write
- Actions and checks: read-only

Do not grant ruleset bypass, administration, environments, secrets, packages,
or release authority. `APPROVER_LOGINS` defaults to `ChrisMarxDev`.

Manual dry run:

```sh
HERMES_LOOP_DRY_RUN=1 bash ./loop/run-hermes-task-loop.sh
```

## Host isolation and scheduling

Run both loops in a dedicated checkout on an agent host, never a production
Tinkercloud VPS. The host must not contain provider, production,
operator-recovery, or release-signing secrets.

Schedule script-only jobs; deterministic prefetch decides whether a model call
is necessary:

```text
tinkercloud issue triage    every 15m    script: run-hermes-triage-loop.sh    no_agent: true
tinkercloud task loop       every 30m    script: run-hermes-task-loop.sh      no_agent: true
```

Each scheduler wrapper changes to the repository checkout and executes the
corresponding script. `no_agent: true` is intentional.

## Files

- `fetch-triage-issues.sh`: finds new, changed, or malformed open issues.
- `mark-triage-seen.py`: atomically records revisions processed by triage.
- `run-hermes-triage-loop.sh`: locked, token-gated triage entrypoint.
- `skills/triage-workflow/SKILL.md`: issue-only triage authority and behavior.
- `fetch-issues.sh`: verifies trusted task command events.
- `run-hermes-task-loop.sh`: locked planning/implementation entrypoint.
- `skills/task-workflow/SKILL.md`: worktree and draft-PR task rules.
- `setup-github-labels.sh`: idempotently creates or updates the taxonomy.
- `test.sh`: fake-GitHub denial and invocation tests.

## Validation

```sh
bash ./loop/test.sh
HERMES_LOOP_DRY_RUN=1 bash ./loop/run-hermes-triage-loop.sh
HERMES_LOOP_DRY_RUN=1 bash ./loop/run-hermes-task-loop.sh
```

The first command is self-contained. Dry runs require the corresponding token,
`gh`, `flock`, and Hermes on the loop host.
