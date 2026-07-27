#!/bin/sh
# Assert that V1 local operator/deployer state cannot be added accidentally,
# while released trust material and reproducibility inputs remain visible.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

ignored() {
  git check-ignore -q -- "$1" || {
    echo "expected ignored: $1" >&2
    exit 1
  }
}
visible() {
  if git check-ignore -q -- "$1"; then
    echo "must remain visible: $1" >&2
    exit 1
  fi
}

ignored tinyhost-vps-known_hosts
ignored resend-api-key
ignored local-resend.key
ignored app-hmac.key
ignored .tiny/credentials.json
ignored .tiny/vps/id_ed25519
ignored .tiny/vps/id_ed25519.pub
ignored .tiny/vps/known_hosts
ignored .tiny/vps/ssh_config
ignored landing/.git/config
visible packaging/release-public-key.pem
visible sdk/typescript/package-lock.json
visible examples/live-presence/index.html
echo "gitignore visibility assertions passed"
