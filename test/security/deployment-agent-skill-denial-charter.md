# Local deployment-agent skill denial charter

The `tinkercloud-deployment-agent` skill is opt-in local deployer-workstation test
infrastructure. It is not a production CI/noninteractive deployment-agent
credential path; production automation uses an independently provisioned
app-scoped deployer token and never a prompt or implicit credential write.

Before a successful test deployment, prove these denials:

- A relative, symlinked, non-owned, missing, or non-directory CLI/app path is
  rejected before a CLI or reader invocation.
- A missing deployer email, app directory, `tinker.yaml`, or HTTPS platform URL
  denies before any CLI action. Reader sender, root domain, key path, and local
  consumed-message ledger are required only when the saved exact identity cannot
  be reused and one forced login is necessary. HTTP, redirects, URL
  paths/query/fragment, unsafe ports, and malformed root domains deny.
- `tinker whoami --json` must contain exactly the expected valid identity before
  a saved credential can be reused. Malformed JSON, compatibility/transport
  failure, an unexpected response shape, or a non-auth error deny rather than
  triggering login.
- A missing/invalid saved credential or a verified different identity triggers
  at most one normal `tinker login --force`; it must complete as the exact
  requested deployer and then pass a new `whoami --json` proof. Reader,
  prompt, timeout, redirect, provider, OTP, or identity failure prevents
  deployment.
- The wrapper never passes a bearer or OTP through argv/environment, emits an
  OTP/bearer in stdout/stderr, copies a reader key/ledger to a project or VPS,
  or lets a production CI deployment agent use its interactive path.
- Exactly one `tinker deploy --json` may run after identity proof. A nonzero
  result, malformed JSON, failed/active-but-unverified result, non-HTTPS URL,
  redirect, timeout, or missing CLI anonymous-denial evidence fails the run;
  no retry uploads another release.

The deterministic fake-CLI/fake-reader test must cover exact-identity reuse
without OTP, wrong-identity forced login, redacted OTP output, login failure
blocking deploy, deployment failure, and unsafe/missing inputs. It runs without
network access or live mutation.
