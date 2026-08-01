# Repository map

This map is for contributors and agents; the user-facing entry point is the
root `README.md`.

```text
AGENTS.md                 agent operating boundary
PRINCIPLES.md             product/security decision gates
PRD.md                    canonical implementation scope
cmd/                      tinkercloud server and tinker CLI entry points
internal/                 private Go gateway and capability packages
sdk/                      browser SDK source and package tests
web/                      server-owned native UI and templates
examples/                 user-facing SDK and starter app examples
concept/                  browsable product concept and feature ranking
docs/                     operator/deployer/product/security guidance
specs/                    technology-neutral contracts
skills/                   self-contained coding-agent workflows
loop/                     autonomous GitHub issue intake workflow
internals/                contributor and agent-only material
```

Runtime directories such as `data/`, `runtime/`, and `state/` are created by
`tinkercloud init` and must not be committed.
