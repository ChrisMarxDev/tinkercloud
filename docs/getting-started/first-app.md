# Host Your First Tinkercloud App

This guide deploys **Tinker Ritual**, a small dependency-free sample app. It is
private to its deployer by default.

## Before you start

You need:

- macOS or Linux;
- the HTTPS endpoint of a running Tinkercloud server; and
- an email address the operator authorized as a deployer.

If you are setting up the server too, follow the
[Hetzner deployment guide](../operations/hetzner-deployment.md) first.

This journey is prepared for `v0.1.6`, which is currently unpublished. Run its
commands only after that exact GitHub prerelease has been published; do not
substitute `v0.1.5`, `latest`, or a mutable branch.

## 1. Install Tinker

Install that exact beta as your normal workstation account, without `sudo`:

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-client.sh | sh
```

Open a new terminal, then verify the installed command:

```sh
tinker version
```

## 2. Copy and preview the sample

```sh
git clone --depth 1 --branch v0.1.6 https://github.com/ChrisMarxDev/tinkercloud.git tinkercloud-source
cp -R tinkercloud-source/examples/starter-app my-tinker-ritual
cd my-tinker-ritual
tinker dev
```

Open the local URL printed by Tinker. Press `Ctrl+C` when you are done. The
local preview is only a development convenience; it does not reproduce hosted
authentication, TLS, blobs, deployment policy, or provider capabilities.

## 3. Deploy

```sh
tinker --server <remembered-server> deploy .
```

After a verified `whoami --server <remembered-server>`, the first manifest
wizard asks exactly: `App slug (<suggested>, Enter to accept):`, `Description
(optional):`, `Build output (<default>, Enter to accept):`, `Allowed emails or
domains, comma-separated (optional):`, `Features (kv,blobs,realtime;
optional):`, and `SPA fallback (optional):`. Enter empty allowlist and features
for owner-only with no features; leave SPA fallback empty unless separately
reviewed. Review endpoint, slug, description, output, owner-only access, no
features, and SPA fallback, then give affirmative go-ahead before the one
deployment invocation.

Ask `Deploy this owner-only app to <server> now? [y/N]`; only explicit yes
continues. Success prints `Deployment: <id>`, `State: active`, and `URL:
<exact-origin>`; the CLI's platform-health and anonymous-denial checks are
internal preconditions.

If the result is `active_but_unverified`, do not deploy again. Recheck only the
returned exact URL as a fresh anonymous request:

```sh
curl --include --silent --show-error --no-location --cookie '' --max-time 15 --max-filesize 32768 -H 'Accept: application/json' <returned-url>
```

This evidence-only recheck requires `401`, `Cache-Control: no-store`, JSON
`not_authorized`, no `Set-Cookie` or `Location`, and no app bytes. It sends no
authentication and is never a second deployment.

The command prints the protected app URL. Do not use this bounded deployer
journey to obtain another browser identity. Open it only with a proven reusable
exact browser identity for that protected app; otherwise defer opening it as
separate later viewer work, must not request a viewer/browser OTP, and finish
this journey with fresh anonymous denial evidence instead.

To publish an update, edit the files and run the same command again:

```sh
tinker --server <remembered-server> deploy .
```

## Use your own project

Run `tinker --server <remembered-server> deploy .` from the root of an existing static web project. When
`tinker.yaml` is missing, the human wizard inspects the project, asks only for
required values it cannot infer safely, and writes the manifest as a reusable
deployment receipt. You do not need to author configuration before the first
deployment.

Use `tinker init .` only when you deliberately want to create and review that
receipt before deploying. The full format is documented in the
[manifest contract](../../specs/manifest/tinker-yaml.md).

## Safety notes

- With no viewer rule, only the active deployer who owns the app may open it.
- Do not put passwords, API keys, or other secrets in app files.
- Tinkercloud beta is for replaceable toy, prototype, and utility apps—not
  business-critical data.
- For the separately reviewed capability-free public-static flow, follow the
  [public-static access contract](../../specs/api/public-static-access.md).

If deployment fails, run `tinker version`, confirm the displayed endpoint, and
share only redacted diagnostics with the operator. Never paste an OTP, bearer,
cookie, provider key, or private configuration into a public issue.
