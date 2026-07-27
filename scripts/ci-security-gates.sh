#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
./scripts/scan-secrets-self-test.sh
./scripts/scan-secrets.sh
./scripts/gitignore-self-test.sh
./scripts/setup-vps-ssh-self-test.sh
./scripts/run-fuzz-gates.sh
