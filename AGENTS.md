# Agent Guidance for TinyHost

Read `PRINCIPLES.md` and then `PRD.md` before planning, coding, or reviewing
changes. Principles override the PRD; the PRD overrides topic documents and
examples.

## Before implementation

1. Identify the smallest roadmap slice that produces a user-observable outcome.
   Confirm it belongs to PRD milestones M0–M5 rather than post-V1 direction.
2. Name the trust boundary and data ownership affected.
3. Locate or add the technology-neutral contract in `specs/`.
4. Write the deny-path test charter before the happy path.
5. Record a decision in `docs/decisions/` when a choice changes the deployment,
   trust, persistence, or public interface model.
6. Update the relevant TinyHost coding-agent skill whenever an SDK, manifest,
   capability, deployment, or verification workflow changes.
7. Treat `skills/tiny-platform` as the canonical shared skill source. Refresh
   marked copies in each standalone specialized skill and run the drift check.
8. For TinyHost-owned web UI, read and follow
   `skills/tiny-native-ui/SKILL.md`. Capture every durable component rule in
   the UI contract, design guidance, implementation, showcase, regression
   tests, and the skill in the same change.

## Implementation constraints

- Do not add a public listener outside the gateway.
- Do not expose app storage through a second file server or raw URL.
- Do not accept app IDs or viewer identity from client-controlled input.
- Do not call a protected dispatcher with an untyped boolean such as
  `authorized=true`; pass an authorization context produced by the gateway.
- Do not activate a release before policy and security verification succeed.
- Do not add backend runtimes before the static app path meets its exit gate.
- Do not add an operator-facing backup system to V1.
- Do not expose operator/provider secrets to the SDK or browser.
- Treat KV as realtime recovery state; never claim socket history, replay,
  ordering, or durability in V1.
- Authenticate before WebSocket upgrade, derive app/session identity on the
  server, and close affected connections on revocation.
- Keep dependencies narrow and justify security-critical ones in an ADR.

## Change loop

For each slice:

1. Update the contract and test charter.
2. Implement one vertical path.
3. Run unit, contract, integration, and negative security tests.
4. Exercise failure injection for changed dependencies.
5. Review diffs against `PRINCIPLES.md`.
6. Update the roadmap evidence and any affected ADR.
7. Run and update the relevant coding-agent skill examples and verification.

An agent must not report a deployment or security feature complete solely
because the happy path works.
