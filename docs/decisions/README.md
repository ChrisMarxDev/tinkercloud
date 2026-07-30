# Architecture Decision Log

| ID | Decision | Status |
|---|---|---|
| [0001](0001-modular-go-monolith.md) | Modular Go monolith with SQLite and private filesystem | Accepted for V1 |
| [0002](0002-app-scoped-sessions.md) | App-scoped opaque sessions | Accepted for V1 |
| [0003](0003-static-runtime-first.md) | Static runtime before backend execution | Accepted |
| [0004](0004-per-host-tls-first.md) | Per-host ACME before wildcard automation | Accepted for V1 |
| [0005](0005-split-server-and-client-binaries.md) | Separate server and deployer binaries | Accepted |
| [0006](0006-hetzner-first-native-install.md) | Hetzner-first native install and signed self-update | Accepted |
| [0007](0007-defer-backups.md) | Defer operator backups beyond V1 | Accepted |
| [0008](0008-sdk-and-capability-boundary.md) | First-class SDK and server-side capability broker boundary | Accepted direction |
| [0009](0009-v1-kv-and-ephemeral-realtime.md) | Bounded KV plus ephemeral app-scoped realtime in V1 | Accepted for V1 |
| [0010](0010-generic-first-agent-skills.md) | Generic-first, self-contained coding-agent skills | Accepted for V1; package split superseded by 0042 |
| [0012](0012-candidate-aware-activation-gates.md) | Candidate-aware immutable activation gates | Accepted; rollback portion superseded by 0039 |
| [0013](0013-confirmed-app-deletion.md) | Confirmed app deletion permanently removes an app playground | Accepted for V1 |
| [0014](0014-signed-release-artifacts.md) | Signed artifacts pinned to an Ed25519 release key | Accepted for V1 |
| [0016](0016-bounded-live-transport-liveness.md) | Bounded authenticated WebSocket liveness and prompt revocation | Accepted for V1 |
| [0017](0017-deployer-public-denial-proof.md) | Deployer-side public denial proof | Accepted for V1 |
| [0022](0022-app-host-login-navigation.md) | App-host document login and account switching | Accepted for V1 |
| [0018](0018-root-only-doctor-credential-read.md) | Root-only credential file for doctor | Accepted for V1 |
| [0019](0019-root-deployer-command-grammar.md) | Root-only deployer command grammar | Accepted for V1 |
| [0023](0023-policy-revision-concurrency.md) | Optimistic concurrency for access-policy replacement | Accepted for V1; rollback references superseded by 0039 |
| [0020](0020-root-deployer-database-identity.md) | Root deployer command writes SQLite as the service identity | Accepted for V1 |
| [0021](0021-embedded-native-web-design-system.md) | Embedded dependency-free design system for Tinkercloud-owned web UI | Accepted for V1 |
| [0024](0024-separated-control-browser-sessions.md) | Separate control browser sessions from CLI bearer tokens | Dashboard-session portion superseded by 0051; CLI bearer separation retained |
| [0025](0025-tinkercloud-owned-port-confinement.md) | Confine Tinkercloud-owned network exposure to TCP 80 and 443 | Accepted for V1 |
| [0026](0026-sdk-registry-distribution.md) | One SDK API across npm and JSR | Accepted for preparation |
| [0027](0027-immutable-deployment-descriptions-and-stable-launch.md) | Immutable deployment descriptions and stable dashboard launch | Accepted; rollback portion superseded by 0039 |
| [0028](0028-operator-supplied-tls-for-vpn-only.md) | Operator-supplied TLS for the first VPN-only topology | Accepted post-V1 direction |
| [0029](0029-updater-candidate-doctor-rollback-snapshot.md) | Candidate doctor tolerates only its expected rollback snapshot | Accepted for V1 |
| [0034](0034-update-denial-probe-capability-route.md) | Update denial health uses a protected capability route | Accepted for V1 |
| [0030](0030-v1-lightweight-local-blob-storage.md) | Lightweight local blob storage behind a provider-neutral seam | Accepted for V1 |
| [0031](0031-trusted-issue-loop-draft-prs.md) | Trusted issue loop proposes changes through draft pull requests | Accepted for repository delivery |
| [0032](0032-local-resend-reader-for-unattended-vps-acceptance.md) | Local Resend reader for unattended VPS acceptance | Accepted for V1 acceptance evidence |
| [0033](0033-global-viewer-identity-app-bound-handoff.md) | Global viewer identity with app-bound handoffs | Handoff retained; naming/dashboard topology superseded by 0051 |
| [0035](0035-bounded-update-listener-readiness.md) | Bounded local listener readiness before update health gates | Accepted for V1 |
| [0036](0036-per-user-cli-credential-file.md) | Protected per-user CLI credential file | Accepted for V1 |
| [0037](0037-deploy-first-cli-onboarding.md) | Deploy-first local manifest onboarding | Accepted for V1 |
| [0038](0038-bounded-certificate-readiness-retry.md) | Bounded pre-activation certificate readiness retry | Accepted for V1 |
| [0039](0039-defer-deployer-release-rollback.md) | Defer deployer-selected release rollback | Accepted post-V1 |
| [0040](0040-operator-deployer-allowlist-reconciliation.md) | Reconcile one active deployer allowlist | Accepted for V1 |
| [0041](0041-minimum-necessary-guided-flows.md) | Minimum-necessary guided human flows | Accepted for V1 |
| [0042](0042-two-role-facing-agent-skills.md) | Two role-facing agent skills | Accepted for V1 |
| [0043](0043-workstation-cli-host-operations.md) | Workstation CLI coordinates a fixed SSH host grammar | Accepted for distribution preparation |
| [0044](0044-signed-distribution-compatibility-manifest.md) | Signed distribution compatibility manifest | Accepted for distribution preparation |
| [0045](0045-separate-maintainer-distribution-skills.md) | Separate maintainer skills for CLI and SDK distribution | Accepted for distribution preparation |
| [0046](0046-github-beta-release-channel.md) | GitHub prereleases are the pre-stable beta channel | Accepted for beta distribution |
| [0047](0047-operator-governed-llm-chat.md) | Operator-governed encrypted LLM chat capability | Accepted for post-V1 L1/L2 |
| [0048](0048-per-app-sqlite-collections.md) | Per-app SQLite databases and bounded document collections | Accepted |
| [0049](0049-typed-deployer-app-data-access.md) | Typed deployer access to managed app data | Accepted |
| [0050](0050-local-deployer-workstation-otp-automation.md) | Local deployer-workstation OTP automation is not CI credentialing | Accepted for V1 test infrastructure |
| [0051](0051-one-domain-single-browser-identity.md) | One root domain and a single browser identity broker | Accepted replacement architecture; implementation in progress |
| [0052](0052-tinkercloud-product-identity.md) | Tinkercloud product and distribution identity | Accepted |

## Decision rule

Create an ADR when a choice changes:

- a trust boundary or public listener;
- data ownership, persistence, update/recovery, backup, or migration;
- the public HTTP/CLI/manifest contract;
- production dependencies or deployment topology;
- the V1 scope/non-goals.

An ADR records why and consequences. It does not substitute for component or
contract documentation.
