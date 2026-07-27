## Outcome

<!-- Smallest user-observable outcome and the M0–M5 milestone. -->

## Trust boundary and data ownership

<!-- What authority, tenant data, credentials, or public interface changes? -->

## Non-goals

<!-- Explicitly list nearby work this pull request does not add. -->

## Contract and deny paths

- Contract/spec:
- Deny-path charter:
- Failure injection:

## Verification

<!-- List exact commands and meaningful results. Do not paste secrets or personal data. -->

## Checklist

- [ ] I read `PRINCIPLES.md` and the relevant PRD milestone.
- [ ] Anonymous, cross-app, revoked, malformed, and unavailable-state cases deny where applicable.
- [ ] Tests cover the changed success and failure paths.
- [ ] Documentation, examples, migrations, and skills match the behavior.
- [ ] I added or updated an ADR for a deployment, trust, persistence, dependency, or public-interface decision.
- [ ] I ran `./scripts/scan-secrets.sh`.
- [ ] I included no credentials, private keys, personal data, or generated runtime state.
