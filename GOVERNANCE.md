# Governance

TinyHost currently uses a maintainer-led governance model.

## Roles

- **Contributors** report issues, propose changes, review work, and submit pull
  requests.
- **Maintainers** triage work, merge changes, manage releases and security
  reports, and protect the project's trust boundaries.

The current lead maintainer is [@ChrisMarxDev](https://github.com/ChrisMarxDev).
Additional maintainers may be invited after sustained, constructive
contributions and demonstrated care with the security model.

## Decisions

Routine changes are decided through issues and pull request review. The
source-of-truth order is:

1. `PRINCIPLES.md`
2. `PRD.md`
3. `specs/`
4. accepted ADRs
5. topic documentation and examples

Changes to deployment, trust, persistence, public interfaces, or
security-critical dependencies require an ADR. Changes that conflict with a
core principle are rejected or require an explicit revision to the principles,
not a quiet implementation exception.

Maintainers have final merge and release authority. They should explain a
rejection or material scope reduction in the relevant issue or pull request.
No contributor or maintainer is entitled to a response or release deadline.

## Releases and security

Only maintainers with release authority may publish signed artifacts. Release
signing keys remain outside the repository and outside production VPS hosts.
Security reports are handled privately under [SECURITY.md](SECURITY.md).

## Conduct

Participation is governed by [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
