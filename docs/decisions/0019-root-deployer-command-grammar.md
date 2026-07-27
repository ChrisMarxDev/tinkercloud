# ADR 0019: Root deployer command grammar

**Status:** Accepted for V1

## Context

The VPS recovery authority includes a root-only command to authorize, suspend,
or revoke a deployer. Go flag parsing stops at the first positional argument,
so treating both the action and email as generic arguments made the documented
`tinyhost deployers authorize --config PATH EMAIL` form reject its config
flag. The command also did not use the operator-installed configuration path
by default.

## Decision

Use one explicit grammar:

```text
tinyhost deployers ACTION [--config PATH] EMAIL
```

`ACTION` is parsed before its flags and is one of `authorize`, `suspend`, or
`revoke`. `--config` defaults to `/etc/tinyhost/config.yaml`; flags after the
email and extra arguments are rejected. The command remains root-only and
passes the untrusted email unchanged to the persistence operation, which owns
normalization and validation. CLI errors are stable typed values and suppress
flag-parser diagnostics, paths, and secrets.

## Consequences

- The operational shorthand `tinyhost deployers authorize email@example.com`
  works on a standard installation.
- The VPS acceptance suite exercises that standard default path.
- Automation requiring a non-standard configuration must place `--config`
  between the action and email.
