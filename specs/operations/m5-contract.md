# M5 Operations Contract

All operations commands are local composition calls; none creates a remote
recovery surface. Doctor results are typed, bounded, and redact secrets.

- `tinkercloud status` is an offline-safe local status surface: private paths and
  permissions, init state, SQLite `quick_check`, disk watermark, local clock,
  system service and listeners, installed version, and update rollback state.
  It does not resolve DNS, open a public TLS connection, read provider
  credentials, or send email.
- `tinkercloud doctor` includes every `status` check and makes bounded (five
  second) outbound checks for platform DNS, the configured wildcard DNS record,
  platform TLS hostname/chain/expiry, and a read-only authenticated request to
  the environment-selected Resend, Postmark, SendGrid, or SMTP adapter. It requires local root before
  reading provider credentials.
  By default it reads `/etc/tinkercloud/credentials/tinkercloud.env`; an explicit
  absolute credential-file path is allowed only when it has no symlinked path
  component, is a root-owned regular file with mode `0600`, and has exactly the
  configured email-provider and HMAC environment assignments once each. It
  passes the parsed provider key through typed local composition, never by changing the
  caller environment. It never sends email and never prints addresses, keys,
  secret references, credential paths, or provider response bodies. A residual
  rollback snapshot is unhealthy until an operator resolves it; an absent
  rollback directory is healthy. The updater's replacement-process doctor
  invocation may treat only the canonical private `update-rollback/previous`
  snapshot as expected while it completes the same update transaction. It
  still runs every other doctor check, and an absent, malformed, symlinked, or
  group/world-accessible snapshot remains unhealthy. This candidate-only mode
  is not `status` or ordinary operator `doctor` behavior.

- Init advances idempotent named steps only after each adapter reports durable
  success; an interrupted step remains retryable.
- Before installing or starting the ACME-capable service, init performs a
  bounded DNS preflight for the derived exact platform hostname
  `admin.{domain}` and a fixed synthetic one-label app hostname beneath
  `{domain}`. The latter proves that the configured wildcard resolves without
  requiring an active app. Either lookup failure or an empty answer leaves the
  DNS step incomplete, starts no service, and reports the exact non-secret host
  or wildcard action to correct. This is a resolution gate, not an IP-address
  comparison: DNS may legitimately use an A, AAAA, or CNAME answer. It does
  not contact ACME or any certificate authority.
- An existing host enables the LLM encryption root only through local-root
  `tinkercloud llm enable`. It creates or verifies the root-owned credential entry
  without revealing it, writes only the environment reference to config, then
  requires a service restart. A subsequent root-only `doctor` validates the
  credential shape without provider use beyond its existing read-only selected
  email-provider dependency check.
- Init is root-only and non-interactive. It validates an explicit host
  allowlist of Ubuntu 24.04 LTS/amd64 and Ubuntu 26.04 LTS/amd64, NTP, and
  exclusive 80/443 availability before creating the service identity or
  database. The allowlist is not a numeric range. Non-secret typed config is
  readable by the service group; secret values are copied from root-readable
  files into a root-only systemd EnvironmentFile and never appear in config,
  diagnostics, or command output.
- Init's database creation and initial-operator mutation each run in a fixed
  child that permanently drops to the installed `tinkercloud` service identity
  before opening SQLite. The control database and its WAL/SHM artifacts are
  therefore service-owned from a clean install. On a resumable legacy init,
  root may hand off only an existing regular root-owned database/WAL/SHM
  artifact after no-symlink, owner, and non-permissive-mode validation; it
  never recursively changes the data directory or widens modes.

- The generated systemd sandbox derives its writable allowlist from the
  validated config and contains exactly the data and ACME cache directories.
  Root, non-canonical, control-character, whitespace, duplicate, and symlinked
  paths deny service installation before `systemctl` runs.
- The gateway service runs as the unprivileged `tinkercloud` user with
  `NoNewPrivileges=yes`. Its capability bounding and ambient sets contain
  exactly `CAP_NET_BIND_SERVICE`, solely so the gateway can bind ports 80 and
  443; no root execution or broader filesystem/network privilege is granted.
  The systemd cgroup bind policy denies every other TCP or UDP bind and allows
  only TCP ports 80 and 443. The production configuration must name port 80 for
  HTTP and port 443 for HTTPS.
- Tinkercloud does not install, enable, disable, or rewrite the host firewall,
  cloud firewall, SSH service, or any pre-existing listener. Those remain
  operator-owned. Tinkercloud's network-exposure delta is exactly its TCP 80 and
  443 gateway listeners.
- VPS acceptance must reject an additional non-loopback listener owned by the
  `tinkercloud` process, even when the required 80/443 listeners are healthy.
  Pre-existing listeners owned by other services are outside Tinkercloud's
  exposure delta and do not fail this check.
- The canonical V1 topology requires public DNS plus Internet reachability of
  TCP 80/443 for per-host HTTP-01 certificates and public gateway evidence.
  Firewall and SSH policy remain operator-owned.
- Strict VPN-only ingress is not a supported V1 topology. Certificate or public
  proof failure leaves setup incomplete; no VPN mode may skip TLS verification,
  hostname verification, anonymous-denial evidence, or app authorization.
- The accepted first post-V1 VPN direction uses one operator-supplied
  certificate/key pair covering the platform and wildcard app hostnames. This
  direction does not become supported behavior until protected installation,
  atomic replacement, trusted-network verification, expiry diagnostics, and
  the VPN acceptance suite pass.
- The first VPN-only mode retains email OTP authentication and current per-app
  authorization. VPN membership alone never creates or substitutes for a
  Tinkercloud session.
- `tinkercloud deployers authorize|suspend|revoke` authenticates its local caller
  as root before parsing configuration, then runs its one fixed SQLite mutation
  in a child that permanently drops to the installed `tinkercloud` service
  identity before opening SQLite. The database, WAL, and SHM files are
  therefore created and written only by that service identity; the command
  never widens their modes. Before the child starts, root may hand off only an
  existing regular root-owned database/WAL/SHM artifact after no-symlink,
  owner, and non-permissive-mode validation. After the child durably commits
  and closes, the root parent `try-restart`s only an already active service and
  confirms it is active; it never starts an inactive service. Missing service
  identity, privilege-drop, handoff, SQLite, mutation, service-state, or
  refresh failure denies a success result. A refresh failure uses explicit
  `deployer_applied_service_refresh_failed` because the durable mutation is not
  rolled back.
- Init state is an ordered, fail-closed JSON record beside the root-owned
  config. It records `preflight`, `paths`, `database`, `operator`, `dns`,
  `service`, and `verified` only after each step succeeds; gaps or unknown
  steps deny continuation.
- The final init public-health proof is derived only from the configured root
  `domain`: `https://admin.{domain}/api/v1/version`. It uses verified
  TLS (with no insecure override), follows no redirect, and accepts only the
  exact final host, a `200` JSON object containing only `{"api_version":1}`,
  and the gateway's `Cache-Control: no-store` and
  `X-Content-Type-Options: nosniff` headers. A 401/404/5xx, arbitrary 2xx or
  HTML response, malformed/oversized JSON, redirect, hostname mismatch, TLS,
  transport, or timeout failure leaves `verified` incomplete.
- Recovery requires an explicit local-root guard before operator replacement.
  In the same transaction it revokes prior operator CLI credentials and the
  affected global browser identity families, including credentials belonging
  to a same-email operator being reactivated.
- At the critical disk watermark, writes deny while safe existing reads remain.
- The authenticated operator dashboard includes a small host-resource read
  model for CPU utilization, RAM utilization, and Tinkercloud data-volume usage.
  It is not returned to deployers or anonymous callers.
- Resource sampling runs inside the existing `tinkercloud` process through an
  injected host-metrics source. It retains at most 60 one-minute samples in a
  rolling in-memory window, does not sample in response to dashboard requests,
  does not persist time-series data, and resets cleanly on restart. It creates
  no metrics listener, monitoring service, or second operational authority.
- Samples contain only timestamps, bounded utilization values, and aggregate
  byte counts. They contain no process list, command line, hostname, network
  address, email, app identity, filesystem path, or credential-derived value.
  Data-volume usage uses the same configured volume and source semantics as the
  disk watermark gate.
- Missing, stale, malformed, or failed resource samples render as unavailable,
  never as zero or healthy. Metrics failure does not bypass or weaken the
  independent disk write gate, and rendering the chart performs no mutation.
- Resource limits default to: 20 apps/deployer, 100 MiB archive, 250 MiB
  expanded release, 10,000 files, 50 MiB file, 20 deployment attempts/hour,
  10 inactive releases plus the active release, 25 MB/blob, 1,000 blobs/app,
  250 MB total blobs/app, and 80%/90% disk watermarks.
  Configuration may tune these only inside the server's bounded V1 ranges.
- At the 90% critical disk watermark, app creation, new deployment creation,
  KV mutation, and blob upload deny closed. Existing authorized static/blob
  reads, blob deletion when safe, session revocation, policy revocation, and
  suspension remain available.
- Cleanup selects only database-derived, inactive immutable release hashes;
  it never accepts a client path, never deletes the active release, retains at
  least one recovery release, and leaves metadata intact if deletion fails.
- Cleanup runs only inside the single gateway process under a bounded lease and
  cancellation-aware ticker. Each outcome is audited with a bounded count, not
  a filesystem path or release hash.
- Request and service logs are JSON records containing only request ID, route
  class, outcome, status, and duration (or a fixed service event and count).
  They never contain URL/query/body data, hostnames, cookies, credentials,
  email addresses, or private filesystem paths.
- Updates accept only a pinned Ed25519 public key and exact artifact digest,
  create a bounded rollback state before replacement, restart the service, then
  first waits at most ten seconds, with bounded cancellation-aware backoff,
  for TCP connection establishment to the configured local HTTP and HTTPS
  listeners. It then commits only after one-shot local doctor, public-health,
  and composed-gateway
  anonymous-denial probe pass. When an active app exists, the updater
  deterministically selects the lexicographically first locally verified active
  app and targets `/_tinker/api/v1/capabilities` on its derived host. It accepts
  only the protected-route gateway response: `401`,
  `application/json`, `Cache-Control: no-store`,
  `X-Content-Type-Options: nosniff`, and the stable `not_authorized` error
  envelope whose request ID matches `X-Request-ID`. A 404, redirect, 2xx,
  malformed denial, timeout, or dependency error is unhealthy. Any failure
  restores the old binary and restarts the restored service.
  Candidate doctor health may tolerate only the updater-owned secure rollback
  snapshot that must remain until commit; it must not mask any other doctor
  failure or alter ordinary rollback-pending diagnostics.
- Manual updates use either the local air-gapped server artifact triplet plus
  the signed release-manifest triplet, or one operator-configured/explicit
  HTTPS release origin. Remote retrieval accepts
  only the V1 `tinkercloud-linux-amd64` filename, follows no redirects, rejects
  credentials, query strings, private/link-local/loopback origins, cross-origin
  components, and bodies over the per-component limits. The public health URL
  is derived as `admin.<domain>`; the anonymous-denial URL is derived from
  locally verified installed app state, never caller input. If no active app
  exists, the updater must prove that exact database state, platform health,
  socket confinement, and safe unknown-app-host denial; any ambiguous read is
  unhealthy. Update URLs are never accepted as arbitrary health-probe targets.
  Automatic updates are disabled by default. Root may enable exactly `beta` or
  `stable`; a fixed systemd timer invokes no shell, arbitrary path, URL, or
  operator-controlled command. Discovery is bounded untrusted input and may
  select only a strictly newer official GitHub release. The candidate artifact
  and complete release manifest remain the authority and must pass pinned
  Ed25519 verification. Automatic updates reject same/older versions,
  redirects, cross-origin components, prerelease/stable channel mismatches,
  and every persistence-schema change before snapshot. Disabling the channel
  stops future timer runs without changing installed or durable server state.
- A signed update candidate is also rejected before snapshot or replacement
  unless its strict semantic version, control API, and persistence schema
  labels satisfy the current compatibility contract. The complete signed
  release manifest binds the CLI and SDK ranges used by runtime HTTP and
  WebSocket negotiation.
- A supported update replaces only the installed server binary. Configuration,
  root-owned credentials, the control database, immutable app releases,
  app-local databases and blobs, browser identity/session rows, deployer
  bearers, and ACME state retain their existing paths and authority. Release
  acceptance seeds these objects before an actual version transition and
  proves them usable after success and after an injected failed-candidate
  rollback.
- The workstation `tinker host install|status|doctor|update|uninstall` surface is only a
  fixed SSH adapter to these root-local operations. It opens no listener,
  stores no root credential, preserves normal host-key verification, and
  accepts no arbitrary remote command.

## Host preflight deny charter

Host support is derived only from the runtime OS/architecture and exact,
unambiguous `ID` plus `VERSION_ID` fields in `/etc/os-release`.

- Non-Linux and non-amd64 runtimes deny.
- Missing, unreadable, malformed, empty, duplicated, or conflicting `ID` or
  `VERSION_ID` fields deny.
- A distribution other than exact `ubuntu` denies; `ID_LIKE=ubuntu` is not
  sufficient.
- Ubuntu 24.10, 25.04, and 25.10 deny because they are interim/end-of-life
  releases, despite falling numerically between supported LTS releases.
- Ubuntu releases before 24.04, after 26.04, and future unverified releases
  deny.
- Version prefixes, suffixes, point-like impostors, and numeric range matching
  deny; only exact `24.04` and `26.04` values pass.
- Every denial happens before service-user, config, credential, database,
  systemd, or listener mutation.

## Update candidate-health deny charter

- A normal `tinkercloud status` or `tinkercloud doctor` with a rollback snapshot
  remains degraded; candidate-health handling does not change operator
  diagnostics.
- Candidate health accepts a rollback snapshot only at the configured private
  data-directory path when both the directory and `previous` are non-symlinked
  private artifacts of the expected types.
- A missing, malformed, symlinked, group/world-accessible, or otherwise
  incomplete rollback snapshot remains unhealthy even in candidate health.
- Any database, disk, clock, service, listener, DNS, TLS, provider, public
  health, or anonymous-denial failure remains unhealthy in candidate health
  and restores the old binary.
- Listener readiness retries only failed local TCP connection establishment for
  the configured HTTP and HTTPS addresses after restart. It is capped at ten
  seconds, observes cancellation, and precedes the one-shot doctor, public,
  and anonymous-denial gates; none of those gates is retried or weakened.

## Gateway bind-policy deny charter

- HTTP configured on a port other than 80 denies configuration.
- HTTPS configured on a port other than 443 denies configuration.
- A generated or packaged service unit missing the default bind deny, either
  TCP allow, or the restricted address-family set denies validation.
- UDP bind permission, a port range, an additional TCP port, or a broader
  address family denies service-unit validation.
- VPS evidence containing a `tinkercloud`-owned non-loopback listener outside TCP
  80/443 fails, while an operator-owned listener does not become Tinkercloud's
  responsibility.

## Init DNS preflight deny charter

- A missing, failing, empty, or context-cancelled lookup for `admin.{domain}`
  leaves init at `dns`; it must not install or start the service or contact
  ACME.
- A missing, failing, empty, or context-cancelled lookup for the synthetic
  one-label app hostname leaves init at `dns` with the same no-service,
  no-ACME result.
- A DNS answer is never required to equal a guessed public VPS address.

## VPN-only topology deny charter

- Unreachable public HTTP-01 leaves certificate readiness incomplete.
- An unavailable public HTTPS version proof leaves initialization incomplete.
- VPN membership never creates a Tinkercloud viewer, deployer, or operator
  identity and never bypasses current app policy.
- A self-signed, hostname-mismatched, expired, or unverified certificate never
  becomes healthy evidence.
- A supplied certificate that does not cover both the exact admin hostname
  and wildcard app hostname, does not match its private key, or is read through
  a symlink or permissive secret path never becomes installed state.
- Temporary renewal exposure, disabled TLS verification, a skipped
  anonymous-denial probe, or a trusted forwarded identity is not an accepted
  workaround for VPN-only ingress.
