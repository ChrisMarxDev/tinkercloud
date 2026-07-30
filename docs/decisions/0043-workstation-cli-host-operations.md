# ADR 0043: Workstation CLI coordinates a fixed SSH host grammar

Status: Accepted for distribution preparation

## Context

Operators want the small `tinker` CLI to install and administer `tinkercloud`.
Placing root host controls on the public control API would add a remote
authority and weaken the gateway boundary. Requiring unrelated handwritten SSH
commands would also make verified installation and update behavior inconsistent.

## Decision

Add `tinker host install|status|doctor|update` as a workstation-side OpenSSH
adapter. It requires an explicit `root@HOST`, keeps normal host-key checking,
and permits only fixed remote commands. Install sends a reviewed bootstrap
embedded in the CLI; status, doctor, and update invoke the local `tinkercloud`
binary. Deployer HTTPS commands and their bearer storage remain separate.

The CLI is not a daemon, opens no listener, stores no root credential, and has
no arbitrary remote-exec command.

## Consequences

Operators get one tool and a testable command grammar while the server remains
one gateway binary. SSH authentication and host-key ownership remain operator
responsibilities. More advanced fleet management is outside V1.
