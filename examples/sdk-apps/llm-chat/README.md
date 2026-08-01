# Private non-streaming chat

Shows disclosure, capability discovery, pending/cancel behavior, and typed
safe errors for an operator-approved `llm.chat` grant. It keeps Send disabled
until `tinker.capabilities.list()` contains `llm.chat`; an absent or revoked
grant is a safe durable unavailable state. It retains only the latest complete
turns in memory for this open browser tab; it never persists messages or
selects a provider, connection, model, endpoint, or credential.

Build this example from `examples/sdk-apps` with `npm run build`, then deploy
this directory with `tinker deploy .`. The operator must separately create a
bounded profile and explicitly approve the app grant. See the live, one-call
acceptance procedure in
[LLM live smoke](../../../docs/operations/llm-live-smoke.md).
