# ADR 0031: Trusted issue loop proposes changes through draft pull requests

**Status:** Accepted for repository delivery

## Context

Loozr uses a deterministic GitHub issue prefetch followed by an unattended
Hermes agent loop. That pattern avoids spending model work on an empty queue
and keeps task state in GitHub. Its current implementation also accepts normal
`open` issues for implementation and may commit directly to `main`.

Tinkercloud is a security-sensitive platform with a documented one-slice-per-
branch workflow, planned protected-`main` rules, and human maintainer authority
for merge and release decisions. GitHub issue text can become public untrusted
input. Copying direct-to-`main` behavior would let intake bypass the review and
security-evidence gate.

Tinkercloud does not yet have in-app feedback submission. Written GitHub issues
are the only intake source for this first loop slice.

## Decision

Adopt Loozr's deterministic prefetch and token-gated model invocation pattern,
with a narrower authority model defined by
[`specs/delivery/github-issue-loop-contract.md`](../../specs/delivery/github-issue-loop-contract.md).

All issue content is untrusted. The agent may triage `inbox` issues and produce
plans, but repository changes require a currently present `implement` label
whose latest applying actor is in the configured maintainer allowlist.
`pending` always blocks implementation, and `open` alone is not actionable.

Approved work receives one `feature/issue-*` branch and one draft pull request.
The agent may push that branch, comment evidence, and move the issue to
`pending` review. It may not merge, bypass a ruleset, publish a release or
package, access a protected environment, or push implementation directly to
`main`.

The runner uses a dedicated checkout and least-privilege GitHub credential. It
must be isolated from production, operator, provider, recovery, and signing
secrets. Runtime snapshots and prompts are disposable local state ignored by
Git.

The first slice does not port Loozr's autonomous failed-`main` CI fixer. Pull
request CI provides validation feedback without giving an unattended repair
loop direct authority over `main`. In-app feedback intake is also deferred
until Tinkercloud has a separately designed app capability for it.

## Consequences

- Maintainers can feed written issues into a recurring loop without making
  every issue an implementation instruction.
- A public reporter, compromised issue body, or triage mistake cannot
  intentionally authorize code merely by writing `implement` in text.
- Trusted approval is executable evidence from GitHub label history rather
  than a prompt convention.
- The agent can carry approved work to a reviewable PR, but branch protection,
  review, and CI remain the merge gate.
- The loop adds no Tinkercloud listener, runtime dependency, product credential,
  storage authority, or V1 feature.
- The unattended host and GitHub token remain security-sensitive operational
  components and must stay narrow, isolated, revocable, and observable.
- CI auto-repair may be reconsidered later only with a branch/PR-based contract
  and bounded retry evidence.

