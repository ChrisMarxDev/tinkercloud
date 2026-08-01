# ADR 0043: Workstation CLI coordinates a fixed SSH host grammar

Status: Accepted for distribution preparation

## Context

Operators want the small `tinker` CLI to install and administer `tinkercloud`.
Placing root host controls on the public control API would add a remote
authority and weaken the gateway boundary. Requiring unrelated handwritten SSH
commands would also make verified installation and update behavior inconsistent.

## Decision

Add `tinker host install|status|doctor|update|uninstall` as a workstation-side OpenSSH
adapter. It requires an explicit `root@HOST`, keeps normal host-key checking,
and permits only fixed remote commands. Install sends a reviewed bootstrap
embedded in the CLI; status, doctor, and update invoke the local `tinkercloud`
binary. Deployer HTTPS commands and their bearer storage remain separate.

The CLI is not a daemon, opens no listener, stores no root credential, and has
no arbitrary remote-exec command. `uninstall` is the sole destructive host
operation: it asks for a workstation-local acknowledgement of the exact host
(or requires explicit `--yes` for deterministic automation) and invokes only
the fixed root-local `tinkercloud uninstall --confirm-uninstall` command. The
root command removes only canonical Tinkercloud installation paths and retains
the ACME cache by default, avoiding certificate churn on a later reinstall.
It preflights Linux mount boundaries before stopping the service so recursive
removal cannot cross one. Removing the fixed service identity is intentional:
the next canonical setup recreates it and re-owns the preserved ACME cache.

A released `tinker host install root@HOST` derives its immutable GitHub release
directory from the strict build version. Development builds must use an
explicit validated release directory. Installation and initialization remain
separate: the former places signed server artifacts; implemented automation
uses resumable `init --non-interactive`, while the minimum-question
`tinkercloud setup` assistant remains planned.

## Consequences

Operators get one tool and a testable command grammar while the server remains
one gateway binary. SSH authentication and host-key ownership remain operator
responsibilities. More advanced fleet management is outside V1.
