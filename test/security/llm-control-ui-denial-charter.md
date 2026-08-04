# LLM API-key dashboard denial charter

The admin-host API-key dashboard section is an operator-only, write-only
credential mutation surface. Tests must prove the following before its happy
path is accepted:

- Anonymous, deployer, cross-origin, missing-CSRF, malformed, oversized, and
  unsupported-provider create/rotate/disable attempts deny before validation or
  persistence.
- A create request carrying `display_name`, `id`, or `connection_id` is denied;
  only fixed Anthropic/Gemini provider selection and one bounded password API
  key are accepted. The service derives the connection ID and fixed display
  label, while previously stored labels and multiple same-provider connections
  remain readable as safe metadata.
- Rotation rejects any browser provider field and validates only against the
  stored fixed provider. Validation or persistence failure preserves the prior
  encrypted connection and status.
- Dashboard HTML, redirects, notices, errors, audit views, and source never
  contain an API key, envelope/ciphertext, encryption-root value/reference,
  raw provider response, or raw validation error.
- When the repository, envelope root, or validator is unavailable, the
  operator sees only a safe unavailable state with root-only `tinkercloud llm
  enable` plus restart guidance; no create, rotate, or disable form renders.
- Deployer dashboards omit API keys and LLM chat capability controls entirely.
- The operator model catalog contains entries only for active connections whose
  encrypted key decrypts and whose fixed provider returns a bounded valid model
  list. Disabled, missing-key, undecryptable, timed-out, provider-rejected,
  malformed, and oversized responses contribute no options and disclose no
  failure detail; one failed connection does not suppress successful entries.
- Catalog metadata never appears in deployer/viewer pages, app capability
  discovery, the SDK, redirects, notices, audit, or errors. The browser cannot
  supply a provider URL or use a malformed catalog selection to choose a
  connection or model.
- The explicit custom-model path accepts only the existing bounded single-line
  model identifier, still requires a current active connection, and never
  weakens provider, endpoint, profile, app-policy, quota, or invocation checks.
- The first active profile becomes default exactly once. Stale or malformed
  default-selection writes, inactive profiles, and browser attempts to select a
  provider/model indirectly are denied. An unavailable default never falls back.
- Host quota and app-policy forms are operator-only, same-origin, CSRF-bound,
  bounded, and revision checked. Invalid modes, inconsistent nullable limits,
  stale writes, cross-app slugs, and deployer submissions make no change.
- An app is enabled and inherits host policy without a row. Explicit disable,
  inherit, unlimited, and specific allowance changes affect the next call.
  Unlimited state still reports metered current-month usage.
- The dashboard contains no grant/request controls and deployers cannot see or
  mutate provider, model, default, quota, usage, or app policy metadata.
