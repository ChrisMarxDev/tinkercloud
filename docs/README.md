# Tinkercloud user documentation

These documents explain Tinkercloud to operators, deployers, and viewers.
Operators are the primary audience; deployer and viewer guidance follows.
Contributor workflows, agent instructions, repository maps, and CI mechanics
belong in [`../internals/`](../internals/README.md) or a focused skill.

Canonical product scope: [`../PRD.md`](../PRD.md). The documents below expand
user-facing product and operational behavior without overriding it.

## Get started

- [Human setup: host your first private app](getting-started/first-app.md)
- [Tinker Ritual starter app](../examples/starter-app/)

## Product

- [V1 scope](product/v1-scope.md)
- [Actors and journeys](product/actors-and-journeys.md)
- [Shopify Quick north star](product/north-star-quick.md)
- [Future capability broker](product/capability-broker.md)
- [Native web design system](product/web-design-system.md)

## Architecture

- [System architecture](architecture/system.md)
- [Component map](architecture/components.md)
- [Request authorization pipeline](architecture/request-pipeline.md)
- [State machines](architecture/state-machines.md)
- [Data model](architecture/data-model.md)
- [Client SDK](architecture/client-sdk.md)

## Security

- [Threat model](security/threat-model.md)
- [Security test matrix](security/test-matrix.md)

## Delivery

- [Roadmap](delivery/roadmap.md)
- [Definition of done](delivery/definition-of-done.md)
- [Hetzner-first deployment](operations/hetzner-deployment.md)
- [Common setup scenarios](operations/setup-scenarios.md)
- [External VPS smoke acceptance](operations/vps-e2e.md)
- [Signed release pipeline](operations/release-pipeline.md)
- [SDK distribution](operations/sdk-distribution.md)


## Decisions and contracts

- [Architecture decision log](decisions/README.md)
- [HTTP contract](../specs/api/http-contract.md)
- [Unified browser identity and app-bound handoff contract](../specs/api/browser-identity-handoff-contract.md)
- [Event vocabulary](../specs/events/event-catalog.md)
- [Manifest contract](../specs/manifest/tinker-yaml.md)
- [Native web UI contract](../specs/ui/native-web-system.md)

For contributor and agent material, start with the
[internal repository guide](../internals/README.md).
