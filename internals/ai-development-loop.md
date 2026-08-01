# AI-Assisted Development Loop

The goal is not “let an agent build the whole platform.” The goal is to make
small changes inspectable, evidence-driven, and hard to accidentally declare
secure.

## Repository roles

Use separate agent roles even when one person operates them sequentially:

- **Planner** — selects one roadmap slice and writes/updates the contract.
- **Implementer** — changes only the named components and tests.
- **Adversary** — creates bypass, cross-app, malformed, concurrency, and failure
  cases without editing production code.
- **Reviewer** — checks principle compliance, boundary erosion, and dependency
  direction.
- **Verifier** — runs the complete relevant suite and captures evidence.

The implementer must not be the only source of its acceptance criteria.

## Loop for one change

### 1. Frame

Create a short change brief:

```text
Outcome:
In scope:
Out of scope:
Trust boundary touched:
Contracts changed:
Failure modes:
Required negative tests:
Rollback/recovery:
Open decision:
```

Reject changes that combine unrelated boundaries or cannot name a reversible
slice.

### 2. Contract first

Update the relevant HTTP, manifest, event, state-machine, or repository contract.
Write examples and reason codes. For a protected endpoint, add its row to the
surface matrix before adding the route.

### 3. Test charter

The adversarial pass produces cases for:

- anonymous;
- authenticated but unauthorized;
- wrong app;
- just revoked;
- missing/corrupt dependency state;
- maximum-size and malformed input;
- concurrent duplicate request;
- interruption around durable transitions.

These are reviewed before implementation begins.

### 4. Implement the thinnest vertical slice

The implementer receives an explicit file/component allowlist. No opportunistic
refactors, new dependencies, or adjacent features. TODOs may document deferred
behavior but may not weaken the deny path.

### 5. Local evidence ladder

Run in increasing cost:

```text
format/static analysis
→ focused unit tests
→ contract and migration tests
→ integration tests through real HTTP
→ negative security matrix
→ fuzz corpus / race detector
→ failure injection
→ browser or VPS smoke where relevant
```

Stop on the first failure and preserve the simplest reproduction.

### 6. Adversarial review

Ask the adversary to answer:

- Can I reach the handler without an authorization context?
- Can I choose app or identity input?
- Can I make an error path serve stale/partial data?
- Can I reuse an artifact across apps, sessions, or retries?
- Can I exhaust disk, memory, goroutines, database writes, or email?
- Can I inject secrets into logs or audit?

New findings become regression tests, not prose-only assurances.

### 7. Principle and decision review

Diff the change against `PRINCIPLES.md`. If it changes a trust/persistence/public
contract choice, update or add an ADR. If it expands scope, update the concept
ranking before merging.

### 8. Verify and record

The verifier records:

```text
commit/revision:
contract version:
tests run:
security cases run:
failure cases run:
known gaps:
evidence artifact:
```

“Complete” means the roadmap exit signal is demonstrated, not that files exist.

## Prompt templates

### Planner

```text
Read PRINCIPLES.md and the target roadmap milestone. Propose the smallest
vertical slice that creates [outcome]. Name affected contracts, components,
states, threat cases, and explicit non-goals. Do not implement.
```

### Implementer

```text
Implement only the approved slice and named components. Preserve all current
deny behavior. Add contract, unit, integration, and negative tests. Do not add a
dependency or public route without stopping for a decision.
```

### Adversary

```text
Assume the new happy path works. Try to bypass authorization, cross app
boundaries, replay state, exploit parser ambiguity, interrupt durable steps, and
exhaust bounded resources. Return minimal reproducible cases and expected
fail-closed behavior. Do not modify production code.
```

### Reviewer

```text
Review this diff against every principle, the component dependency direction,
the route registry, and relevant ADRs. Prioritize concrete authorization,
tenancy, recovery, and secret-handling failures over style.
```

## Branch and change hygiene

- One milestone slice per branch.
- Conventional commits that name behavior, not agents.
- Generated AI notes do not become source-of-truth unless folded into a contract,
  ADR, or test.
- Never ask an agent to “fix all security” or “finish the platform.”
- Human approval is required for migrations, cryptographic/session design,
  dependency additions, release signing, update rollback behavior, and public
  exposure.

## Unattended GitHub issue loop

The repository may run the recurring issue loop in [`loop/`](../loop/).
It uses deterministic GitHub prefetch so an empty queue causes no model call.
Issue text is untrusted intake, and `open` means refined rather than authorized.
Repository changes require a current `implement` label applied by a configured
trusted maintainer, with no `pending` label.

The loop carries approved work only to a feature branch and draft pull request.
It does not merge, bypass protected `main`, publish, release, or access
production/signing credentials. See the executable state and deny contract in
[`specs/delivery/github-issue-loop-contract.md`](../specs/delivery/github-issue-loop-contract.md).

## Development deployment loop for apps hosted on Tinkercloud

The later Tinkercloud agent skill should use:

```text
inspect → build → validate tinker.yaml → deploy with scoped token
→ poll terminal deployment state → independently probe anonymous denial
→ authenticate test viewer → probe expected content → report URL + release ID
```

It must never convert a failed protection probe into a warning.
