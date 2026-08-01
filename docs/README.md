# Tinkercloud user documentation

This directory contains user, operator, deployer, product, architecture, and
security guidance. Start with the root [README](../README.md), then choose the
path that matches the work you need to do.

## Get started

- [Host your first private app](getting-started/first-app.md)
- [Tinker Ritual starter app](../examples/starter-app/)
- [Local app development](getting-started/local-emulator.md)

## Operate a host

- [Hetzner-first deployment](operations/hetzner-deployment.md)
- [Common setup scenarios](operations/setup-scenarios.md)
- [External VPS smoke acceptance](operations/vps-e2e.md)
- [Signed release pipeline](operations/release-pipeline.md)
- [CLI distribution](operations/cli-distribution.md)
- [SDK distribution](operations/sdk-distribution.md)

## Build and deploy apps

- [V1 scope](product/v1-scope.md)
- [Actors and journeys](product/actors-and-journeys.md)
- [Client SDK](architecture/client-sdk.md)
- [Future capability broker](product/capability-broker.md)
- [System architecture](architecture/system.md)
- [Component map](architecture/components.md)
- [Request authorization pipeline](architecture/request-pipeline.md)
- [Native web design system](product/web-design-system.md)

## Understand the security model

- [Threat model](security/threat-model.md)
- [Security test matrix](security/test-matrix.md)
- [Data model](architecture/data-model.md)
- [State machines](architecture/state-machines.md)
- [HTTP contract](../specs/api/http-contract.md)
- [Manifest contract](../specs/manifest/tinker-yaml.md)
- [Native web UI contract](../specs/ui/native-web-system.md)

## Product direction

- [Shopify Quick north star](product/north-star-quick.md)
- [Roadmap](delivery/roadmap.md)
- [Architecture decision log](decisions/README.md)
- [Event vocabulary](../specs/events/event-catalog.md)

Contributor and agent procedures are intentionally not part of this user
navigation. Find them in [`../internals/`](../internals/README.md) and
[`../skills/`](../skills/).
