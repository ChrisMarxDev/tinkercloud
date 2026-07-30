#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-vps-ssh.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir -p "$tmp/bin"

cat >"$tmp/bin/ssh-keygen" <<'EOF'
#!/bin/sh
set -eu
key=
derive=0
while test "$#" -gt 0; do
  case "$1" in
    -f) key=$2; shift 2 ;;
    -y) derive=1; shift ;;
    *) shift ;;
  esac
done
test -n "$key"
if test "$derive" -eq 1; then
  printf '%s\n' 'ssh-ed25519 test-public-key tinkercloud-testing-vps'
  exit 0
fi
printf '%s\n' 'test private key fixture' >"$key"
printf '%s\n' 'ssh-ed25519 test-public-key tinkercloud-testing-vps' >"$key.pub"
EOF
chmod 700 "$tmp/bin/ssh-keygen"

ssh_dir="$tmp/material"
if PATH="$tmp/bin:$PATH" TINKERCLOUD_VPS_SSH_DIR="$ssh_dir" \
  "$root/scripts/setup-vps-ssh.sh" 'host with spaces' >/dev/null 2>&1; then
  echo "unsafe host was accepted" >&2
  exit 1
fi
test ! -e "$ssh_dir"

PATH="$tmp/bin:$PATH" TINKERCLOUD_VPS_SSH_DIR="$ssh_dir" \
  "$root/scripts/setup-vps-ssh.sh" 203.0.113.10 2222 >/dev/null

test -f "$ssh_dir/id_ed25519"
test -f "$ssh_dir/id_ed25519.pub"
test -f "$ssh_dir/known_hosts"
test -f "$ssh_dir/ssh_config"
grep -F 'HostName 203.0.113.10' "$ssh_dir/ssh_config" >/dev/null
grep -F 'Port 2222' "$ssh_dir/ssh_config" >/dev/null
grep -F 'StrictHostKeyChecking yes' "$ssh_dir/ssh_config" >/dev/null
grep -F "UserKnownHostsFile \"$ssh_dir/known_hosts\"" "$ssh_dir/ssh_config" >/dev/null
if grep -E 'StrictHostKeyChecking +(no|accept-new)' "$ssh_dir/ssh_config" >/dev/null; then
  echo "unsafe host-key mode was written" >&2
  exit 1
fi

before=$(cksum "$ssh_dir/id_ed25519")
if PATH="$tmp/bin:$PATH" TINKERCLOUD_VPS_SSH_DIR="$ssh_dir" \
  "$root/scripts/setup-vps-ssh.sh" 203.0.113.10 2222 >/dev/null 2>&1; then
  echo "existing key was overwritten" >&2
  exit 1
fi
after=$(cksum "$ssh_dir/id_ed25519")
test "$before" = "$after"

reuse_dir="$tmp/reuse"
mkdir -p "$reuse_dir"
cp "$ssh_dir/id_ed25519" "$reuse_dir/id_ed25519"
cp "$ssh_dir/id_ed25519.pub" "$reuse_dir/id_ed25519.pub"
PATH="$tmp/bin:$PATH" TINKERCLOUD_VPS_SSH_DIR="$reuse_dir" \
  "$root/scripts/setup-vps-ssh.sh" test-vps.example 22 >/dev/null
grep -F 'HostName test-vps.example' "$reuse_dir/ssh_config" >/dev/null

echo "VPS SSH setup deny/allow self-test passed"
