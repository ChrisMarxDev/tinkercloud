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
