#!/bin/sh
# Create passphrase-protected, repo-local SSH connection material for the
# dedicated TinyHost acceptance VPS. Host-key trust is completed separately
# after an out-of-band fingerprint check.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
destination=${TINYHOST_VPS_SSH_DIR:-"$root/.tiny/vps"}

usage() {
  echo "usage: $0 HOST_OR_IPV4 [SSH_PORT]" >&2
  exit 2
}

test "$#" -ge 1 && test "$#" -le 2 || usage
host=$1
port=${2:-22}

case "$host" in
  ""|-*|*[!A-Za-z0-9.-]*)
    echo "host must be a DNS name or IPv4 address without whitespace" >&2
    exit 2
    ;;
esac
case "$port" in
  ""|*[!0-9]*)
    echo "SSH port must be a decimal number" >&2
    exit 2
    ;;
esac
if test "$port" -lt 1 || test "$port" -gt 65535; then
  echo "SSH port must be between 1 and 65535" >&2
  exit 2
fi

key="$destination/id_ed25519"
known_hosts="$destination/known_hosts"
config="$destination/ssh_config"

for path in "$known_hosts" "$config"; do
  if test -e "$path"; then
    echo "refusing to overwrite existing VPS SSH material: $path" >&2
    exit 1
  fi
done
if test -e "$key" && test ! -f "$key.pub"; then
  echo "private key exists without its public key: $key" >&2
  exit 1
fi
if test -e "$key.pub" && test ! -f "$key"; then
  echo "public key exists without its private key: $key.pub" >&2
  exit 1
fi

umask 077
mkdir -p "$destination"
chmod 700 "$destination"

if test -f "$key" && test -f "$key.pub"; then
  derived_public=$(ssh-keygen -y -f "$key" | awk 'NF >= 2 { print $1 " " $2; exit }') || {
    echo "existing private key could not be read: $key" >&2
    exit 1
  }
  stored_public=$(awk 'NF >= 2 { print $1 " " $2; exit }' "$key.pub")
  if test "$derived_public" != "$stored_public"; then
    echo "existing private and public keys do not match" >&2
    exit 1
  fi
  echo "Reusing verified keypair in $destination"
else
  echo "Create a dedicated key for the disposable testing VPS."
  echo "Choose a passphrase when ssh-keygen prompts; load it with ssh-add before an unattended test."
  ssh-keygen -t ed25519 -a 100 -f "$key" -C "tinyhost-testing-vps"
fi

: >"$known_hosts"
cat >"$config" <<EOF
Host tinyhost-test
  HostName $host
  User root
  Port $port
  IdentityFile "$key"
  IdentitiesOnly yes
  PasswordAuthentication no
  StrictHostKeyChecking yes
  UserKnownHostsFile "$known_hosts"
EOF
chmod 600 "$key" "$known_hosts" "$config"
chmod 644 "$key.pub"

echo
echo "Created ignored connection material in $destination"
echo "Public key: $key.pub"
echo "SSH config: $config"
echo
echo "Next, add the public key while provisioning the VPS, then pin its host key:"
echo "  ssh-keyscan -p $port -t ed25519 $host > \"$known_hosts.candidate\""
echo "  ssh-keygen -lf \"$known_hosts.candidate\""
echo "Compare that fingerprint out of band. Only after it matches:"
echo "  mv \"$known_hosts.candidate\" \"$known_hosts\""
echo "  ssh -F \"$config\" tinyhost-test"
