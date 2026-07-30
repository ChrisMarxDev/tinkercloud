# Product North Star: Shopify Quick

Primary reference:
[“Quick: An internal hosting platform for the AI era”](https://shopify.engineering/quick),
Shopify Engineering, published June 10, 2026.

Quick is Tinkercloud's north star for staff usability, constrained capability
design, and agent-assisted creation. It is a reference, not an architecture to
copy literally.

## What Quick demonstrates

Quick validates a remarkably small product loop:

```text
drop in a folder of HTML/assets
→ receive a secure URL in seconds
→ call shared backend capabilities from browser code
→ give coding agents the skills to use the whole platform
```

Its reported capability set is intentionally compact:

- database;
- file uploads;
- AI;
- data warehouse;
- WebSockets;
- identity.

The platform keeps provider keys on the server and exposes pleasant client-side
methods. Quick also treats constraints as a product advantage: a small fixed set
of primitives remains easier to use, maintain, and combine creatively than
custom backends, jobs, and general infrastructure.

## What Tinkercloud should borrow

### Folder-to-URL immediacy

The normal path should remain:

```text
tinker deploy .
```

No framework, container, deployment pipeline, database provisioning, or
authentication code should be required.

The same rule applies to questions: derive project/server state and use secure
defaults before prompting. Generated config and manifests are receipts for
review and automation, not paperwork a human must prepare before the command.

### Capabilities as client API calls

Identity, KV, realtime, files, and later AI should feel like one coherent SDK:

```ts
const viewer = await tinker.user.current();
const posts = await tinker.kv.list({ prefix: "posts/" });
tinker.live.onKvChange({ prefix: "posts/" }, refreshPosts);
const answer = await tinker.llm.generate(...);
```

The deliberately small KV API and ephemeral live layer should make the platform
disappear without pretending to be a general database.

### Server-held provider credentials

Apps invoke capabilities; they do not receive API keys. Tinkercloud adds stronger
per-app grants, operation scopes, quotas, and audit because its trust environment
is broader than one company network.

### Skills included from day one

The platform ships coding-agent skills covering initialization, SDK APIs,
capabilities, manifest creation, deployment, and verification. Skills are tested
release artifacts rather than an afterthought.

### A small fixed capability set

New primitives must earn their complexity. Prefer combinations of current
capabilities over custom runtimes, cron jobs, or arbitrary backend execution.

### One small operational footprint

Quick's single-server success supports Tinkercloud's one-VPS target. Tinkercloud
should add quotas and rate limits early because even trusted staff tools can
accidentally create loops or excessive storage/provider usage.

## Where Tinkercloud deliberately differs

| Quick reference | Tinkercloud decision |
|---|---|
| All sites visible to all Shopify employees | Apps are private allowlist-only by default |
| No meaningful site ownership/permission model | Explicit deployer ownership and per-app policy |
| Shopify IAP supplies a trusted company identity | Tinkercloud owns email OTP, app sessions, and authorization |
| Google Cloud Storage and CloudSQL | One self-hosted VPS, SQLite, and private local releases |
| Internal company trust bubble | Treat app code, viewers, deployers, archives, and browser input as untrusted |
| Direct overwrite-style simplicity | Immutable releases, verified activation, and failed-activation preservation |
| Shopify service infrastructure and AI proxy | Operator-managed adapters and credentials behind Tinkercloud |

## Decision filter

When evaluating a Tinkercloud feature, ask:

1. Does it make folder-to-secure-URL faster or more reliable?
2. Does it remove a question, repeated value, prerequisite file, or manual
   ceremony that the system can safely derive?
3. Can it be a small typed SDK capability instead of infrastructure?
4. Will coding agents understand and verify it from shipped skills?
5. Can it remain server-authorized and secret-free in the browser?
6. Does it compose with existing primitives?
7. Is the capability worth permanently expanding the trusted platform surface?

If the answer is weak, defer it.
