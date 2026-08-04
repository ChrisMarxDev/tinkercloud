# Future Capability Broker

**Status:** Directional concept, explicitly post-V1.

## Opportunity

A technical operator often already holds useful organization credentials:

- LLM provider API keys;
- internal service tokens;
- Jira bot or integration accounts;
- document/search APIs;
- messaging or automation credentials.

Tinkercloud could let non-technical deployers build apps on those services without
ever receiving the underlying credentials. This turns Tinkercloud from private app
hosting into a small, operator-governed capability platform.

This extends the [Shopify Quick north star](north-star-quick.md), where database,
AI, files, realtime, and identity are client-side API calls backed by shared
server capabilities and server-held keys.

## Critical security distinction

A secret cannot be “passed down” to a static app without leaking it. Browser
JavaScript, network inspection, extensions, and the viewer all have access to
anything delivered to the app.

The safe model is invocation, not secret delivery:

```text
deployed app
  → Tinkercloud SDK
  → authenticated app capability endpoint
  → viewer/app policy + quota check
  → server-side adapter
  → operator-managed secret
  → external provider
```

The app receives only the bounded result.

## Operator and deployer model

Operator:

1. Creates a named connection, such as `company-openai` or `jira-automation`.
2. Stores the credential through a local or protected operator flow.
3. Chooses which adapters, operations, models/projects, budgets, and deployers
   may use it.

Deployer:

1. Sees permissions and external data implications.
2. Uses the typed SDK module.
3. Cannot read, export, replace, or arbitrarily forward the credential.

Example direction:

```yaml
capabilities:
  llm:
    connection: company-openai
    operations: [generate]
    model_policy: staff-default
    monthly_budget: 10 EUR

  jira:
    connection: jira-automation
    operations: [issues.read, issues.create]
    projects: [OPS]
```

This generic direction does not govern the implemented `llm.chat` capability:
LLM availability is reactive and has no manifest request or per-app grant.

## Adapter model

Adapters declare:

- versioned operations and input/output schemas;
- credential type and validation;
- allowed destination hosts;
- required app grants;
- external data disclosure;
- request/response size limits;
- cost/quota dimensions;
- redaction and audit fields;
- timeouts, retry, and idempotency behavior.

Avoid a generic “HTTP request with injected secret” capability in the early
design. It creates SSRF, arbitrary exfiltration, confused-deputy, and billing
risks. Prefer narrow LLM, Jira, and internal-API adapters.

An extensibility mechanism for operator-written adapters needs a separate
execution and trust decision. In-process plugins undermine the single-binary
security model; out-of-process or declarative adapters add operational
complexity. Do not choose incidentally.

## Secret storage

Directional requirements:

- encrypted at rest with a host-local root key;
- plaintext held only for the shortest provider-call window;
- never returned by any read API, log, audit event, SDK, or deployment command;
- rotation without redeploying apps;
- connection disable/revoke takes effect immediately;
- root-only recovery may replace but should not reveal a secret;
- provider usage correlated through safe connection/app IDs.

## Controls

- explicit operator approval for every app binding;
- operation and resource scoping;
- per-app/deployer/viewer rate and cost limits;
- outbound host allowlist;
- bounded request/response bodies;
- prompt/input logging off by default;
- audit of connection use without sensitive payloads;
- optional human confirmation for consequential operations.

## Non-goals for the first capability release

- arbitrary shell execution;
- arbitrary outbound HTTP proxying;
- raw secret/environment-variable delivery to static apps;
- deployer-created global credentials;
- unreviewed third-party adapter code in the Tinkercloud process;
- pretending LLM or SaaS provider calls are private to Tinkercloud.

## Open questions

- Should connections be grantable to a deployer, one app, or both?
- Which operations require operator approval per app versus policy templates?
- How are cost budgets represented across providers?
- Can organization-authenticated viewer identity be forwarded, and when?
- What adapter extension model preserves single-binary simplicity?
- Which first adapter proves value: one LLM provider or one internal HTTP schema?

## Recommendation

Build the SDK, current-user, KV, realtime, and capability discovery in V1.
Then design one narrow LLM adapter as the first brokered external capability.
Do not add generic secret injection.
