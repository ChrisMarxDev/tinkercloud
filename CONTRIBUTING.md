# Contributing to Tinkercloud

Thanks for helping improve Tinkercloud. The project welcomes bug reports,
documentation fixes, tests, and focused implementation changes.

## Before opening an issue

- Report security vulnerabilities privately according to [SECURITY.md](SECURITY.md).
- Use the bug form for reproducible defects.
- Use the feature form for product proposals.
- Use the question form for setup and usage help.
- Search existing issues before opening a duplicate.

New issue-form submissions enter the `inbox` state for maintainer/agent
triage. Refinement to `open` means the issue is ready, not that implementation
is authorized. Only a trusted maintainer may apply `implement`; `pending`
always blocks work. Approved agent work is proposed through a draft pull
request and remains subject to normal review and CI.

Tinkercloud is pre-release and V1 scope is intentionally narrow. A proposal that
adds public apps, backend runtimes, a second public listener, a second storage
authority, generic secret injection, or a backup product is outside the current
roadmap.

## Source-of-truth order

Read these before a substantial contribution:

1. [Core principles](PRINCIPLES.md)
2. [Product requirements](PRD.md)
3. Contracts in [`specs/`](specs/)
4. Accepted decisions in [`docs/decisions/`](docs/decisions/)
5. Topic documentation in [`docs/`](docs/)

Principles override the PRD; the PRD overrides lower-level documents.

## Development setup

You need Go 1.25.12, Node.js 22, npm, a POSIX shell, OpenSSL, and Python 3.

```sh
go test ./...
go vet ./...

cd sdk/typescript
npm ci
npm test
```

Run the security and repository invariants from the repository root:

```sh
./scripts/ci-security-gates.sh
./scripts/check-skill-drift
```

The release workflow additionally runs race tests, `govulncheck`, installer
tests, reproducibility checks, and signed-artifact verification. VPS acceptance
is separate evidence and requires an explicitly configured disposable host.

## Change design

For each implementation slice:

1. Name the smallest user-observable outcome and its M0–M5 milestone.
2. State the trust boundary and data ownership affected.
3. Update or add the technology-neutral contract in `specs/`.
4. Write the deny-path test charter before the happy path.
5. Add an ADR when deployment, trust, persistence, or a public interface changes.
6. Update affected docs and Tinkercloud coding-agent skills.
7. Implement one vertical path and exercise dependency failures.

Protected paths must deny anonymous, wrong-app, revoked, suspended, malformed,
and unavailable-state cases. A passing happy path is not sufficient security
evidence.

## Pull requests

- Keep changes focused; do not combine an unrelated refactor with a behavior change.
- Use conventional commit subjects, such as `fix: deny stale app sessions`.
- Run formatting and the relevant tests before requesting review.
- Fill in the pull request template, including explicit non-goals.
- Update docs, contracts, examples, migrations, and skills in the same change
  when their behavior changes.
- Do not include credentials, private keys, personal data, generated runtime
  state, or production logs.

Maintainers may ask for a smaller slice or additional deny-path evidence before
reviewing the implementation.

The autonomous issue-loop contract and local operation are documented in
[`specs/delivery/github-issue-loop-contract.md`](specs/delivery/github-issue-loop-contract.md)
and [`loop/README.md`](loop/README.md).

## Licensing

By intentionally submitting a contribution for inclusion, you agree that it is
licensed under the repository's [Apache License 2.0](LICENSE), consistent with
the contribution terms in that license. Clearly identify third-party material
and its license in the pull request.
