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

ignored tinkercloud-vps-known_hosts
ignored resend-api-key
ignored local-resend.key
ignored app-hmac.key
ignored .tinker/credentials.json
ignored .tinker/vps/id_ed25519
ignored .tinker/vps/id_ed25519.pub
ignored .tinker/vps/known_hosts
ignored .tinker/vps/ssh_config
ignored .hermes-task-loop/issues.json
ignored GITHUB_TOKEN
ignored landing/.git/config
visible packaging/release-public-key.pem
visible sdk/typescript/package-lock.json
visible examples/live-presence/index.html
visible loop/fetch-issues.sh
echo "gitignore visibility assertions passed"
