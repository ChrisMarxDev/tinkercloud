# VPS E2E denial charter

Before reporting the external lifecycle successful, the suite proves:

- the root-only local deployer command accepts exactly `tinyhost deployers
  ACTION [--config PATH] EMAIL`, where `ACTION` is `authorize`, `suspend`, or
  `revoke`; an unknown action, extra positional argument, unknown flag, or a
  flag placed after `EMAIL` fails with a stable redacted error before any
  mutation;
- no live action occurs without `TINYHOST_VPS_E2E=1` and an acknowledgement
  exactly equal to the SSH target;
- missing or unsafe SSH input and a missing, mismatching, or unreadable
  known-hosts file deny before remote administration; SSH never falls back to
  `StrictHostKeyChecking=no` or `accept-new`;
- a normal run refuses a host containing TinyHost state; reuse requires an
  exact suite marker matching the target and one configured root domain;
- after deployer login, fixture cleanup lists only that deployer's
  ownership-scoped apps and deletes only listed members of the fixed
  `vps-e2e-update-probe`, `vps-e2e-primary`, `vps-e2e-isolation`, and
  `vps-e2e-denied` set with distinct fresh idempotency keys; absent fixtures
  are left alone and any list/delete error stops the run without broad cleanup;
- repeated runs use those same four fixture hostnames while retaining a fresh
  archive marker, so stale bytes cannot pass and no randomized hostname causes
  another certificate request;
- missing OTP helper in noninteractive execution, invalid helper output, or an
  invalid deployer/viewer OTP prevents login, without reading OTP data from
  VPS storage or logs;
- an anonymous request for the generated app's root, private asset, current-
  viewer API, or WebSocket upgrade is denied, does not upgrade the socket, and
  does not include the generated app marker;
- only the configured allowlisted viewer whose browser identity was verified
  at `admin.<domain>` reaches the marker and receives the server-derived
  current-viewer identity; the dashboard role check never grants app access;
- app-local logout denies only that app session while global dashboard logout
  at `admin.<domain>` denies the browser identity and every derived app
  session on the next request;
- the smoke archive's immutable `tiny.yaml` carries that exact viewer email in
  its private allowlist, so activation cannot replace the preceding control
  policy with an owner-only candidate policy;
- deployment does not count as complete until the client reports verified
  release/activation evidence; and
- a supported reuse-mode signed update preserves the authorized deployer, its
  app ownership, and the active private access policy before the post-update
  CLI and gateway checks continue; and
- ports 80 and 443 are present and owned by `tinyhost` after installation;
- any additional non-loopback TCP listener owned by `tinyhost` fails the
  acceptance run, while an operator-owned listener remains outside TinyHost's
  exposure delta; and
- installation leaves firewall, SSH, and pre-existing listener ownership with
  the operator.
