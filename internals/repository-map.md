# Repository map

This map is for contributors and agents; the user-facing entry point is the
root `README.md`.

```text
AGENTS.md                 agent operating boundary
PRINCIPLES.md             principle gates
PRD.md                    canonical product scope
README.md                 operator/deployer user guide
cmd/                      tinkercloud and tinker entry points
internal/                 private gateway and capability packages
sdk/                      browser SDK
landing/                  public landing app and Worker deployment
skills/                   agent procedures
loop/                     autonomous issue workflow
internals/                contributor and agent maintenance docs
docs/                     user, operator, deployer, product, and security docs
specs/                    technology-neutral contracts
test/                     test charters
concept/                  offline product concept and feature ranking
```

Runtime state is created by `tinkercloud init` or setup and is not committed to
this repository.
