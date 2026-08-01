# Tinker CLI

`@tinkercloud/cli` installs the native `tinker` deployer command for
Tinkercloud on supported macOS and Linux machines.

```sh
npm install -g @tinkercloud/cli
tinker version
```

The same package works with pnpm, Yarn, and Bun. Modern Yarn uses a project
dependency plus `yarn tinker`, or `yarn dlx @tinkercloud/cli@latest` for a
one-off command. The package contains the reviewed native binaries and does
not download or execute code during installation.
It does not contain the privileged `tinkercloud` server.

Source, signed releases, and issues:
<https://github.com/ChrisMarxDev/tinkercloud>
