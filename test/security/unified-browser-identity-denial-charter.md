# Unified browser identity denial charter

Before the one-domain browser architecture is complete, tests must prove:

- only `admin.<domain>` can issue or rotate the global browser identity;
- the former dashboard OTP channel, control cookie, and control-session
  credential cannot authenticate or mint any browser state;
- viewer identity authentication and current dashboard role authorization are
  separate, with inactive/non-role identities denied before dashboard reads;
- dashboard role does not grant app access and app access does not grant a
  dashboard role;
- admin identity, app child, browser binding, CLI bearer, and agent bearer
  transports are mutually rejected;
- exact HTTPS Origin plus CSRF rejects mutations initiated by any sibling app
  origin despite same-site browser semantics;
- no credentialed wildcard CORS response is emitted;
- reserved labels `admin`, `api`, `auth`, `status`, `www`, `docs`, and
  `install`, including case variants and malformed encodings, cannot resolve or
  be created as apps;
- handoff replay, wrong host, wrong app, stale state, expired state, policy
  revocation, and malformed safe-return input expose no app bytes or session;
- a safe path and query are restored exactly after authentication while
  fragments remain outside the server guarantee;
- app-local logout revokes exactly one child session; global logout and account
  switch revoke the identity family and all children after durable commit,
  close live connections, and do not revoke CLI tokens; and
- wildcard DNS readiness is never treated as proof of admin/app TLS readiness.
