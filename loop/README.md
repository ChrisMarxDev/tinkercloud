# Hermes GitHub Issue Loop

This folder adapts Loozr's deterministic-prefetch Hermes loop for Tinkercloud.
GitHub Issues are durable work state; `.hermes-task-loop/` is disposable local
runtime state.

## Safety model

Every issue is untrusted intake. Tinkercloud has no in-app feedback path yet, so
there is no separate `user-feedback` source or label.

The loop can autonomously:

- refine and classify `inbox`;
- answer or close support/duplicate/out-of-scope intake;
- post requested technical plans; and
- implement an issue only after trusted maintainer approval.

Implementation approval requires a current `implement` label applied by a
login in `APPROVER_LOGINS`, with no `pending` label. `open` alone does not
authorize code. Approved work goes to a `feature/issue-*` branch and draft pull
request. The loop never merges or pushes implementation directly to `main`.

The complete state and deny contract is
[`specs/delivery/github-issue-loop-contract.md`](../specs/delivery/github-issue-loop-contract.md).

## Loop pattern

Each scheduled tick:

1. obtains a non-overlapping host lock;
2. fetches issues with workflow labels;
3. verifies `implement` label actors against `APPROVER_LOGINS`;
4. writes a bounded snapshot to `.hermes-task-loop/issues.json`;
5. exits silently without a model call when nothing is actionable; or
6. invokes Hermes with the repository workflow skill and snapshot.

The agent re-fetches GitHub after each mutation. A snapshot never overrides
newer issue state.

## Labels

Run once with a repository-admin credential:

```sh
GH_TOKEN=*** REPO_SLUG=ChrisMarxDev/tinkercloud ./loop/setup-github-labels.sh
```

The labels are:

- `inbox`: untrusted intake awaiting refinement;
- `open`: refined and ready, but not implementation-authorized;
- `pending`: blocked or awaiting review/decision;
- `plan`: request a technical plan comment;
- `implement`: trusted maintainer command.

Issue forms apply `inbox` automatically. To approve a refined issue, a trusted
maintainer removes `pending` if present and adds `implement`. The loop itself
must never add `implement`.

## Authentication and isolation

The scripts accept `GH_TOKEN`, `GITHUB_TOKEN`, a root `GITHUB_TOKEN` file, or
`GITHUB_TOKEN` from `${HERMES_HOME:-$HOME/.hermes}/.env`. Do not commit any of
them.

Use a dedicated automation account or fine-grained token restricted to this
repository with:

- Metadata: read-only
- Issues: read and write
- Contents: read and write
- Pull requests: read and write
- Actions and checks: read-only

Do not grant administration, ruleset bypass, environments, secrets, packages,
or release authority. Run the loop in a dedicated checkout on an agent host,
not on a production Tinkercloud VPS. The host must not contain provider,
production, operator-recovery, or release-signing secrets.

`APPROVER_LOGINS` is a comma-separated allowlist and defaults to
`ChrisMarxDev`:

```sh
APPROVER_LOGINS=ChrisMarxDev,SecondMaintainer
```

## Run and schedule

Manual dry run:

```sh
HERMES_LOOP_DRY_RUN=1 bash ./loop/run-hermes-task-loop.sh
```

Hermes cron should run a script-only job every 30 minutes. Keep the scheduler
wrapper under Hermes home and the real logic in this repository:

```sh
#!/usr/bin/env sh
set -eu
cd /root/tinkercloud
exec bash ./loop/run-hermes-task-loop.sh
```

Desired scheduler shape:

```text
tinkercloud task loop    every 30m    script: run-hermes-task-loop.sh    no_agent: true
```

`no_agent: true` is intentional. The scheduled script performs cheap
deterministic prefetch and invokes Hermes only after it proves actionable work
exists.

## Files

- `fetch-issues.sh`: fetches labeled issues and verifies implementation label
  actors.
- `run-hermes-task-loop.sh`: locked, token-gated Hermes entrypoint.
- `setup-github-labels.sh`: idempotently creates/updates workflow labels.
- `skills/task-workflow/SKILL.md`: issue-state and implementation rules.
- `test.sh`: fake-GitHub deny-path tests with no network or real model call.

## Validation

```sh
bash ./loop/test.sh
HERMES_LOOP_DRY_RUN=1 bash ./loop/run-hermes-task-loop.sh
```

The first command is self-contained. The second uses the configured GitHub
repository and therefore requires `gh`, a token, `flock`, and Hermes on the
loop host.

