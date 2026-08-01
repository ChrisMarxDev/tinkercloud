#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

if test "$(id -u)" != 0 && "$root/packaging/install-host.sh" https://releases.example.test/v1/ >/dev/null 2>&1; then
  echo "host installer accepted a non-root caller" >&2
  exit 1
fi

# The non-root installer carries the public release key as data. Extract that
# literal PEM without sourcing the installer, then compare canonical public-key
# DER with the repository authority. This never reads a production private key.
embedded_key="$tmp/embedded-release-public-key.pem"
awk '
$0 == "-----BEGIN PUBLIC KEY-----" {
  if (seen) exit 1
  seen = 1
}
seen { print }
$0 == "-----END PUBLIC KEY-----" {
  if (!seen) exit 1
  done = 1
  exit
}
END { if (!seen || !done) exit 1 }
' "$root/packaging/install-client.sh" >"$embedded_key" || {
  echo "client installer embedded public key missing or malformed" >&2
  exit 1
}
openssl pkey -pubin -in "$embedded_key" -pubout -outform DER \
  -out "$tmp/embedded-release-public-key.der" >/dev/null 2>&1 || {
  echo "client installer embedded public key is invalid" >&2
  exit 1
}
openssl pkey -pubin -in "$root/packaging/release-public-key.pem" -pubout -outform DER \
  -out "$tmp/release-public-key.der" >/dev/null 2>&1 || {
  echo "repository release public key is invalid" >&2
  exit 1
}
cmp -s "$tmp/embedded-release-public-key.der" "$tmp/release-public-key.der" || {
  echo "client installer embedded public key drifts from release public key" >&2
  exit 1
}

printf not-a-binary >"$tmp/candidate"
printf '{"version":"1","api":"1","schema":"1","sha256":"%064d"}' 0 >"$tmp/metadata"
printf invalid >"$tmp/signature"
mkdir "$tmp/bin"
printf '#!/bin/sh\ncase "$1" in -s) echo Linux;; -m) echo x86_64;; esac\n' >"$tmp/bin/uname"
printf '#!/bin/sh\necho 0\n' >"$tmp/bin/id"
printf '#!/bin/sh\necho invoked >"$TINKERCLOUD_TEST_MARKER"\nexit 99\n' >"$tmp/bin/tinkercloud"
chmod +x "$tmp/bin/uname" "$tmp/bin/id" "$tmp/bin/tinkercloud"
if PATH="$tmp/bin:$PATH" TINKERCLOUD_TEST_MARKER="$tmp/invoked" "$root/packaging/install.sh" "$tmp/candidate" "$tmp/metadata" "$tmp/signature" >/dev/null 2>&1; then
  echo "unsigned candidate accepted" >&2
  exit 1
fi
test ! -e "$tmp/invoked" || { echo "candidate verifier executed tinkercloud" >&2; exit 1; }

# The privileged installer must reconstruct exactly the same LF-delimited
# payload that the release builder signs. This uses a disposable authority and
# inert candidate; fake install and systemctl commands prevent host mutation.
mkdir -p "$tmp/valid-installer/packaging/systemd"
cp "$root/packaging/install.sh" "$tmp/valid-installer/packaging/install.sh"
cp "$root/packaging/systemd/tinkercloud.service" "$tmp/valid-installer/packaging/systemd/tinkercloud.service"
openssl genpkey -algorithm ED25519 -out "$tmp/valid-installer/private.pem" >/dev/null 2>&1
openssl pkey -in "$tmp/valid-installer/private.pem" -pubout -out "$tmp/valid-installer/packaging/release-public-key.pem" >/dev/null 2>&1
printf 'inert signed candidate\n' >"$tmp/valid-installer/tinkercloud-linux-amd64"
valid_digest=$(sha256sum "$tmp/valid-installer/tinkercloud-linux-amd64" | awk '{print $1}')
printf '{"version":"installer-test","api":"1","schema":"1","sha256":"%s"}\n' "$valid_digest" >"$tmp/valid-installer/tinkercloud-linux-amd64.metadata.json"
printf 'installer-test\n1\n1\n%s' "$valid_digest" >"$tmp/valid-installer/signed"
openssl pkeyutl -sign -inkey "$tmp/valid-installer/private.pem" -rawin -in "$tmp/valid-installer/signed" -out "$tmp/valid-installer/signature.raw" >/dev/null 2>&1
base64 <"$tmp/valid-installer/signature.raw" >"$tmp/valid-installer/tinkercloud-linux-amd64.signature"
printf '#!/bin/sh\ntest "$1" = -m || exit 1\ntouch "$TINKERCLOUD_TEST_INSTALL_MARKER"\n' >"$tmp/bin/install"
printf '#!/bin/sh\nexit 0\n' >"$tmp/bin/systemctl"
chmod +x "$tmp/bin/install" "$tmp/bin/systemctl"
PATH="$tmp/bin:$PATH" TINKERCLOUD_TEST_INSTALL_MARKER="$tmp/valid-installer/installed" \
  "$tmp/valid-installer/packaging/install.sh" "$tmp/valid-installer/tinkercloud-linux-amd64" \
  "$tmp/valid-installer/tinkercloud-linux-amd64.metadata.json" "$tmp/valid-installer/tinkercloud-linux-amd64.signature" >/dev/null
test -f "$tmp/valid-installer/installed" || { echo "installer rejected valid signed artifact" >&2; exit 1; }

# The deployer installer is deliberately non-root and must reject an insecure
# origin, unsupported platform, bad checksums, and invalid signatures before it
# can replace the local client. The fake curl only supplies inert test bytes.
mkdir "$tmp/client-release" "$tmp/client-home"
printf '#!/bin/sh\necho invoked >"$TINKERCLOUD_TEST_MARKER"\n' >"$tmp/client-release/tinker-linux-amd64"
client_digest=$(sha256sum "$tmp/client-release/tinker-linux-amd64" | awk '{print $1}')
printf '{"version":"1","api":"1","schema":"1","sha256":"%s"}\n' "$client_digest" >"$tmp/client-release/tinker-linux-amd64.metadata.json"
printf 'invalid-signature\n' >"$tmp/client-release/tinker-linux-amd64.signature"
(cd "$tmp/client-release" && sha256sum tinker-linux-amd64 tinker-linux-amd64.metadata.json tinker-linux-amd64.signature >SHA256SUMS)
cat >"$tmp/bin/curl" <<'EOF'
#!/bin/sh
set -eu
out=
url=
while test $# -gt 0; do
  case "$1" in
    --output|-o) out=$2; shift 2 ;;
    *) url=$1; shift ;;
  esac
done
cp "$TINKER_TEST_RELEASE/${url##*/}" "$out"
EOF
cat >"$tmp/bin/id" <<'EOF'
#!/bin/sh
echo 1000
EOF
cat >"$tmp/bin/uname" <<'EOF'
#!/bin/sh
case "$1" in -s) echo Linux;; -m) echo x86_64;; esac
EOF
chmod +x "$tmp/bin/curl" "$tmp/bin/id" "$tmp/bin/uname"
if PATH="$tmp/bin:$PATH" HOME="$tmp/client-home" TINKER_TEST_RELEASE="$tmp/client-release" \
  TINKER_RELEASE_BASE=https://releases.example.test/v1 \
  TINKERCLOUD_TEST_MARKER="$tmp/client-invoked" "$root/packaging/install-client.sh" >/dev/null 2>&1; then
  echo "client installer accepted invalid signature" >&2
  exit 1
fi
test ! -e "$tmp/client-invoked" || { echo "client installer executed candidate" >&2; exit 1; }
printf '%064d  tinker-linux-amd64\n' 0 >"$tmp/client-release/SHA256SUMS"
if PATH="$tmp/bin:$PATH" HOME="$tmp/client-home" TINKER_TEST_RELEASE="$tmp/client-release" \
  TINKER_RELEASE_BASE=https://releases.example.test/v1 "$root/packaging/install-client.sh" >/dev/null 2>&1; then
  echo "client installer accepted checksum mismatch" >&2
  exit 1
fi
if PATH="$tmp/bin:$PATH" HOME="$tmp/client-home" TINKER_RELEASE_BASE=http://releases.example.test/v1 \
  "$root/packaging/install-client.sh" >/dev/null 2>&1; then
  echo "client installer accepted insecure release origin" >&2
  exit 1
fi
printf '#!/bin/sh\ncase "$1" in -s) echo Windows;; -m) echo x86_64;; esac\n' >"$tmp/bin/uname"
chmod +x "$tmp/bin/uname"
if PATH="$tmp/bin:$PATH" HOME="$tmp/client-home" TINKER_RELEASE_BASE=https://releases.example.test/v1 \
  "$root/packaging/install-client.sh" >/dev/null 2>&1; then
  echo "client installer accepted unsupported platform" >&2
  exit 1
fi

# A release-build sidecar ends with LF. Verify that the real non-root client
# installer accepts that canonical form but still rejects whitespace injected
# into the Base64 payload. A disposable authority avoids reading any release
# private key.
openssl genpkey -algorithm ED25519 -out "$tmp/client-private.pem" >/dev/null 2>&1
openssl pkey -in "$tmp/client-private.pem" -pubout -out "$tmp/client-public.pem" >/dev/null 2>&1
awk -v key="$tmp/client-public.pem" '
  BEGIN {
    while ((getline line < key) > 0) replacement = replacement line "\n"
    close(key)
  }
  $0 == "-----BEGIN PUBLIC KEY-----" { in_key = 1; printf "%s", replacement; next }
  in_key && $0 == "-----END PUBLIC KEY-----" { in_key = 0; next }
  !in_key { print }
' "$root/packaging/install-client.sh" >"$tmp/install-client-valid.sh"
chmod +x "$tmp/install-client-valid.sh"
rm -rf "$tmp/client-release" "$tmp/client-home"
mkdir "$tmp/client-release" "$tmp/client-home"
printf '#!/bin/sh\necho inert client\n' >"$tmp/client-release/tinker-linux-amd64"
client_digest=$(sha256sum "$tmp/client-release/tinker-linux-amd64" | awk '{print $1}')
printf '{"version":"0.1.0","api":"1","schema":"1","sha256":"%s"}\n' "$client_digest" >"$tmp/client-release/tinker-linux-amd64.metadata.json"
printf '0.1.0\n1\n1\n%s' "$client_digest" >"$tmp/client-signed"
openssl pkeyutl -sign -inkey "$tmp/client-private.pem" -rawin -in "$tmp/client-signed" -out "$tmp/client-signature.raw" >/dev/null 2>&1
base64 <"$tmp/client-signature.raw" | tr -d '\n' >"$tmp/client-release/tinker-linux-amd64.signature"
printf '\n' >>"$tmp/client-release/tinker-linux-amd64.signature"
(cd "$tmp/client-release" && sha256sum tinker-linux-amd64 tinker-linux-amd64.metadata.json tinker-linux-amd64.signature >SHA256SUMS)
printf '#!/bin/sh\ncase "$1" in -s) echo Linux;; -m) echo x86_64;; esac\n' >"$tmp/bin/uname"
chmod +x "$tmp/bin/uname"
# Earlier privileged-installer evidence deliberately shadows these commands.
# The unprivileged client test may safely use the real tools inside its temp
# HOME; otherwise the fake privileged installer would require its marker.
rm -f "$tmp/bin/install" "$tmp/bin/mv" "$tmp/bin/systemctl"
PATH="$tmp/bin:$PATH" HOME="$tmp/client-home" TINKER_TEST_RELEASE="$tmp/client-release" \
  TINKER_RELEASE_BASE=https://releases.example.test/v1 "$tmp/install-client-valid.sh" >/dev/null
test -f "$tmp/client-home/.local/bin/tinker" || { echo "client installer rejected canonical trailing newline" >&2; exit 1; }
sed 's/./& /2' "$tmp/client-release/tinker-linux-amd64.signature" >"$tmp/client-inner-signature"
mv "$tmp/client-inner-signature" "$tmp/client-release/tinker-linux-amd64.signature"
(cd "$tmp/client-release" && sha256sum tinker-linux-amd64 tinker-linux-amd64.metadata.json tinker-linux-amd64.signature >SHA256SUMS)
if PATH="$tmp/bin:$PATH" HOME="$tmp/client-home" TINKER_TEST_RELEASE="$tmp/client-release" \
  TINKER_RELEASE_BASE=https://releases.example.test/v1 "$tmp/install-client-valid.sh" >/dev/null 2>&1; then
  echo "client installer accepted internal signature whitespace" >&2
  exit 1
fi
echo "installer denial tests passed"
"$root/packaging/host-bootstrap_test.sh"
