---
name: tinkercloud-operator
description: Install, configure, diagnose, update, and recover a Tinkercloud server. Use for operator-owned Hetzner VPS setup, DNS, TLS, Resend, deployer authorization, status, doctor, signed updates, disk incidents, root recovery, and Tinkercloud security verification.
---

# Tinkercloud operator

<!-- shared:role-common:start -->
## Platform boundary

Use `operator` for the person who hosts Tinkercloud, `deployer` for a person
authorized to create and manage their own apps, and `viewer` for a person who
accesses an app. A deployment agent is automation acting through deployer
authority. Never transfer credentials or authority between these roles.

Tinkercloud V1 hosts private static apps. The gateway owns TLS, app routing,
viewer authentication, access policy, static files, SDK capabilities, and
deployment activation. It derives the app from the hostname and the viewer from
an opaque app-host session. App code must never select either identity.

Fail closed. Missing, stale, malformed, redirected, incompatible, or ambiguous
security state is a denial, not a value to guess. Never expose or request a
deployer token, OTP, provider secret, database credential, app ID, or viewer ID
in chat, argv, source code, `tinker.yaml`, browser storage, logs, or output.

V1 is private-only. The app owner is always an implicit viewer. There is no
public mode. Each app has its own private SQLite data file for KV and bounded
JSON document collections; blobs and that data are utility-grade state on one
VPS, so losing the VPS can lose them. Realtime is app-scoped, in-memory,
best-effort notification with no history, replay, ordering, or delivery
guarantee. Live collection events are freshness hints: recover current state
with a snapshot after first connect, reconnect, visibility recovery, and every
hint.

`llm.chat`, when an operator grants it, is a narrow server-side capability.
Capability discovery is absent until the app requests it and the operator grant
is active. If present, treat its disclosure as a notice that prompt content is
sent to an operator-selected external AI provider; discovery limits are safe
current bounds, not a promise that a later request will be admitted.
Provider credentials, connection IDs, model names, and upstream URLs never
enter app code, browser storage, the deployer manifest, or the SDK request.

Start from the requested outcome. Inspect trusted local and server state, reuse
verified values, and choose a secure default before asking anything. Ask only
for a required value or decision that is still unknown, cannot be discovered,
and cannot be defaulted safely. Group unresolved optional choices into one
review. Never infer broader authority.

Do not report success from a happy path alone. A deployment is complete only
after a fresh anonymous request through the real HTTPS gateway proves that no
app content is exposed.
<!-- shared:role-common:end -->

<!-- shared:operator:start -->
## Operator workflow

Operate Tinkercloud as one small security boundary: one `tinkercloud` server, one
SQLite database, one private data directory, and one systemd service on a
dedicated supported Hetzner VPS. Only Tinkercloud listens publicly on TCP 80/443.
Do not add Docker as the primary install, a reverse proxy, another file server,
a storage mount, a public object URL, a backend runtime, or a second recovery
authority in V1.

Use this repository's `PRINCIPLES.md`, `PRD.md`, `specs/operations/`, and
`docs/operations/` for implementation detail when available. Before host
mutation, confirm the exact target is a clean dedicated x86-64 Hetzner VPS
running explicitly supported Ubuntu 24.04 LTS or 26.04 LTS. Ambiguous,
interim, end-of-life, other-distribution, or future unverified releases deny.

For a new server, start with the signed installer and resumable
`tinkercloud setup`. Inspect first, then ask only for the controlled root domain,
initial operator email, and Resend credential source that cannot be derived.
Derive `admin.<domain>` and `<slug>.<domain>`, reserve `admin`, `api`, `auth`,
`status`, `www`, `docs`, and `install`, and ask for one wildcard DNS record.
Do not reserve `tinker` or `tinkercloud`. Pause with one exact DNS or Resend action
when external state is incomplete; resume without re-asking verified answers.

The workstation `tinker host install|status|doctor|update` commands may perform
the same root-local flow over an explicit `root@HOST` using normal OpenSSH
host-key verification. They are not a remote control API: never add SSH
options, arbitrary remote commands, automatic host-key acceptance, or a
deployer bearer to that path.

Keep provider and Tinkercloud secrets in root-owned mode-0600 credential files or
systemd credentials. Never accept them in argv, ordinary YAML, chat, browser
state, shell history, logs, or audit. Run the gateway as the unprivileged
`tinkercloud` service identity with only the narrow bind capability for 80/443.

If offering `llm.chat`, configure it as an operator-owned capability adapter:
use the dashboard **API keys** section to choose Anthropic or Gemini and enter
one write-only provider key. Tinkercloud derives its safe label and opaque ID;
never ask for a name, ID, URL, or arbitrary secret. If the dashboard says key
management is unavailable, run the root-only `tinkercloud llm enable` and restart
before entering a key; never expose or copy the encryption root. Define a fixed
approved model/profile and bounded quotas in **LLM chat**, then grant that
profile explicitly to selected apps. The browser receives only Tinkercloud's
same-origin response; it never receives the provider key, connection ID,
provider endpoint, raw provider error, arbitrary model choice, or an outbound
proxy. Removing a grant or connection must deny the next request. The local
`tinker dev` emulator deliberately excludes LLM capability calls, so verify the
real adapter through its normal protected gateway tests.

Use `tinkercloud status` for offline state and `sudo tinkercloud doctor` for checks
that may read root credentials. Initialization is complete only after the exact
platform HTTPS version proof, route classification, socket confinement, safe
unknown-app denial, database, permissions, DNS, TLS, and email checks succeed.
Do not replace public evidence with localhost, a browser page, a redirect, or
insecure TLS.

Manage deployer authority as one revision-protected exact normalized email
allowlist. Show the complete resulting list. Require explicit confirmation only
when adding deployment authority. Removal immediately revokes that deployer's
CLI tokens and pending CLI OTPs while removing dashboard authority on the next
request. It preserves their global browser identity and independently
authorized app access.

For root recovery with `tinkercloud deployers authorize|suspend|revoke`, the
database mutation runs in a fixed child that drops permanently to `tinkercloud`.
After that child has committed and closed, Tinkercloud refreshes only an already
active service and verifies it remains active; it never starts an inactive
service. `deployer_applied_service_refresh_failed` means the authority mutation
is durable but the platform must be checked with `sudo tinkercloud doctor` before
the changed deployer is asked to log in.

Install only artifacts verified against signed release metadata. A checksum
alone is not a trust root. An update verifies both the server artifact and the
signed compatibility manifest before snapshot; air-gapped updates require both
triplets and never mix local with remote inputs. `tinkercloud update` must retain rollback state until
the restarted candidate passes doctor, listener readiness, platform health,
and anonymous protected-app denial. Any ambiguous or failed gate restores the
prior healthy state.

The updater derives its probe from installed server state and selects a locally
verified active app itself. Never ask for or pass an app ID, slug, probe host,
path, or URL. If a failed update did not complete automatic restoration, use
the installed root-local recovery path:

```sh
sudo tinkercloud update --rollback
tinkercloud status
sudo tinkercloud doctor
```

Do not invent another rollback command or delete rollback state by hand.

Root access to the dedicated VPS is the recovery authority. If email is
unavailable, use root-only operator recovery and revoke affected global
identity families and CLI credentials; never create a remote HTTP recovery
bypass.

At disk warning, diagnose and clean only through database-led Tinkercloud
operations. At the write-stop watermark, new deployments, KV mutations, and
blob uploads deny while safe reads, deletions, and revocations continue.
Never expose raw storage paths.

V1 has no operator backup or disaster-recovery feature. State plainly that
loss of the VPS can lose releases, KV, and blobs. Do not describe utility-grade
local state or realtime as durable.

For every consequential operation, report the exact target, command or
dashboard action, observed result, and remaining verification. Never claim
setup, update, recovery, or security completion from the happy path alone.
<!-- shared:operator:end -->
