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

## 1. Install Tinker

Install the current exact beta as your normal workstation account, without
`sudo`:

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/install-client.sh | sh
```

Open a new terminal, then verify the installed command:

```sh
tinker version
```

## 2. Copy and preview the sample

```sh
git clone --depth 1 https://github.com/ChrisMarxDev/tinkercloud.git tinkercloud-source
cp -R tinkercloud-source/examples/starter-app my-tinker-ritual
cd my-tinker-ritual
tinker dev
```

Open the local URL printed by Tinker. Press `Ctrl+C` when you are done. The
local preview is only a development convenience; it does not reproduce hosted
authentication, TLS, blobs, deployment policy, or provider capabilities.

## 3. Deploy

```sh
tinker deploy .
```

On the first run, Tinker asks for the platform endpoint, your deployer email,
and the code sent to that email. It reuses those verified values later. Review
the owner-only access summary and confirm the deployment.

The command prints the protected app URL. Open it in the same browser where you
use Tinkercloud; the normal email identity flow grants the owner access.

To publish an update, edit the files and run the same command again:

```sh
tinker deploy .
```

## Use your own project

Run `tinker deploy .` from the root of an existing static web project. When
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
