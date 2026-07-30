# Host Your First Tinkercloud App

This guide deploys **Tinker Ritual**, a small dependency-free sample app. It has
no build step and is private by default.

## Before you start

You need:

- macOS or Linux with Go 1.25.12 and Python 3
- a clone of this repository
- the URL of a running Tinkercloud server
- an email address authorized to deploy

If you are setting up the server too, follow the
[Hetzner deployment guide](../operations/hetzner-deployment.md) first.

## 1. Build the CLI

From the Tinkercloud repository:

```sh
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/tinker" ./cmd/tinker
export PATH="$HOME/.local/bin:$PATH"
```

## 2. Copy and preview the sample

```sh
cp -R examples/starter-app "$HOME/tinker-ritual"
cd "$HOME/tinker-ritual"
python3 -m http.server 4173
```

Open [http://localhost:4173](http://localhost:4173). Press `Ctrl+C` when you
are done previewing it.

## 3. Choose an app name

Open `tinker.yaml` and replace `tinker-ritual` with a unique name:

```yaml
version: 1
name: my-tinker-ritual
description: A private daily ritual tracker.

build:
  output: .

access:
  mode: private
  allow:
    emails: []
    domains: []
```

An empty allowlist means only the app owner can open it. To invite someone,
add their email address under `emails`.

## 4. Check the manifest

```sh
tinker inspect-manifest tinker.yaml
```

The full format is documented in the
[manifest contract](../../specs/manifest/tinker-yaml.md).

## 5. Sign in and deploy

Replace the example URL with your Tinkercloud server:

```sh
tinker --server https://admin.example.com login
tinker --server https://admin.example.com deploy .
```

The deploy command prints the private app URL. Open it and sign in with the
one-time code sent to your email.

To publish an update, edit the files and run the same deploy command again.

## Notes

- Tinkercloud does not provide a public access mode.
- Tinker Ritual stores its checklist in that browser only.
- Do not put passwords, API keys, or other secrets in app files.
- A local preview does not test Tinkercloud authentication or access rules.

If login or deployment fails, confirm the server URL, your deployer
authorization, and that `tinker inspect-manifest tinker.yaml` succeeds.
