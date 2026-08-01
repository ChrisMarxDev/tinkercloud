# ADR 0056: Configure the local automation recipient domain

**Status:** Accepted

## Context

The opt-in local Resend reader and deployer-workstation wrapper must restrict
automation to identities controlled by the operator running the tests. A
personal domain hard-coded in public source describes one private checkout, not
a Tinkercloud product policy. Removing the restriction or accepting arbitrary
recipients would broaden access to a powerful local mail-reader credential.

## Decision

Require `TINKERCLOUD_AUTOMATION_RECIPIENT_DOMAIN` for every local OTP automation
path. It is a non-secret, normalized DNS domain with no default. The unattended
wrapper, Resend reader, and deployer-workstation wrapper use the same value and
require the requested identity's normalized domain to match it exactly.

Missing, empty, whitespace-padded, multiline, or malformed configuration fails
closed before local path inspection, saved-credential inspection, key or ledger
access, or network action. Subdomains and suffix lookalikes do not match. This
configuration affects only opt-in local test automation; it does not change
normal CLI login, direct browser login, deployer authorization, or production
deployment-agent credentials.

## Consequences

- A public checkout carries no maintainer-specific recipient policy.
- Operators must make the trusted recipient-domain boundary explicit before
  running unattended or deployment-wrapper tests.
- The wrappers remain unusable by default, and a typo cannot silently select a
  broader or different recipient domain.
- Tests use reserved example domains and prove missing, malformed, subdomain,
  and lookalike denials without reading credentials or making network calls.
