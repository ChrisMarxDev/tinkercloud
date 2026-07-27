# ADR 0005: Separate server and client binaries

**Status:** Accepted

## Context

TinyHost promises a single executable server, while deployers need only to
authenticate and upload apps. A universal binary would make the commands look
simple but would ship server-only concerns—SQLite, migrations, ACME, embedded
admin UI, recovery, and service management—to every deployer machine.

AI-assisted development lowers the maintenance cost of two composition roots,
but it does not reduce client artifact size, server attack surface, signing
scope, or the consequences of bundling unrelated privileged code.

## Decision

Build and distribute:

- `tinyhost`: the single self-contained server/operator executable installed on
  the dedicated VPS;
- `tiny`: the small deployer and automation client installed on laptops and CI.

Both live in one repository and share versioned contracts plus the generated or
handwritten API client. Server internals are not shared into the client.

## User experience

The split does not change the product workflow:

```text
# clean VPS
curl -fsSL https://tinyhost.example/install.sh | sudo sh
sudo tinyhost init

# deployer machine
tiny login
tiny deploy .
```

## Consequences

- Smaller client download and narrower supply-chain/signing surface.
- Server and client can later have different release cadence.
- Compatibility must be explicit: the client discovers the server API version
  and reports a useful upgrade requirement.
- Release automation produces and signs multiple OS/architecture artifacts.
- The “one binary” promise means one server binary, not one universal artifact.
