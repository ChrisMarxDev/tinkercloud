# Security Policy

Tinkercloud is a security-sensitive, pre-release project. Please disclose
vulnerabilities privately and allow time for a coordinated fix.

## Supported versions

| Version | Security support |
| --- | --- |
| Unreleased `main` | Best effort |
| `0.1.5` prerelease | Best effort |
| Earlier published prereleases | Unsupported; upgrade to `0.1.5` |

Prepared but unpublished versions, including `0.1.6`, are unsupported until
their exact GitHub prerelease is published. A version being listed does not turn
pre-release software into a durability or production-readiness guarantee.

## Reporting a vulnerability

Do not open a public issue, discussion, or pull request for a suspected
vulnerability.

Use GitHub's
[private vulnerability report](https://github.com/ChrisMarxDev/tinkercloud/security/advisories/new).
If that form is not yet available during the repository-publication window,
email `dev@christopher-marx.de` with the subject `Tinkercloud security report`.
Do not send live credentials, private keys, OTPs, session values, or production
data by email; arrange a safer transfer method first when such evidence is
essential.
Include:

- the affected commit or version;
- the entry point and required attacker access;
- minimal reproduction steps or a proof of concept;
- the expected and observed security boundary;
- potential impact and any known mitigations.

Do not include real user data or live credentials. We aim to acknowledge a
report within five business days and provide a status update within ten
business days. Remediation and disclosure timing depends on severity and
release availability.

## Security boundaries

The gateway is the only public request boundary. App identity and viewer
identity are server-derived, protected dispatchers require a typed
authorization context, and missing or stale state fails closed. The canonical
model is in [PRINCIPLES.md](PRINCIPLES.md) and
[docs/security/threat-model.md](docs/security/threat-model.md).

The file [`packaging/release-public-key.pem`](packaging/release-public-key.pem)
is intentionally public. It is the Ed25519 verification key pinned into release
artifacts. The corresponding signing private key must never be committed,
uploaded to a VPS, included in an artifact, or shared in a report.

Before publishing a fork or contribution, run:

```sh
./scripts/scan-secrets-self-test.sh
./scripts/scan-secrets.sh
```

If a real secret was ever committed, deleting the file is not remediation:
revoke or rotate the credential first, then remove it from every reachable Git
reference before publishing.
