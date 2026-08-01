#!/bin/sh
# Offline evidence for the reviewed CLI bootstrap and signed root installer.
# It never contacts a network endpoint or writes outside its temporary tree.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-host-bootstrap-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
real_python=$(command -v python3)

fail() { echo "host bootstrap test: $*" >&2; exit 1; }

mkdir -p "$tmp/bin" "$tmp/release"
openssl genpkey -algorithm ED25519 -out "$tmp/private.pem" >/dev/null 2>&1
openssl pkey -in "$tmp/private.pem" -pubout -out "$tmp/public.pem" >/dev/null 2>&1

# Substitute only the embedded public authority in disposable copies. This
# keeps the test bound to the shipped scripts while allowing valid signatures
# without access to a production signing key.
replace_key() {
  source=$1
  destination=$2
  awk -v key="$tmp/public.pem" '
    BEGIN {
      while ((getline line < key) > 0) replacement = replacement line "\n"
      close(key)
    }
    $0 == "-----BEGIN PUBLIC KEY-----" { in_key = 1; printf "%s", replacement; next }
    in_key && $0 == "-----END PUBLIC KEY-----" { in_key = 0; next }
    !in_key { print }
  ' "$source" >"$destination"
  chmod +x "$destination"
}
replace_key "$root/internal/hostops/bootstrap.sh" "$tmp/bootstrap.sh"
replace_key "$root/packaging/install-host.sh" "$tmp/install-host.sh"

sign() {
  artifact=$1
  digest=$(sha256sum "$tmp/release/$artifact" | awk '{print $1}')
  printf '{"version":"0.1.0","api":"1","schema":"1","sha256":"%s"}\n' "$digest" >"$tmp/release/$artifact.metadata.json"
  printf '0.1.0\n1\n1\n%s' "$digest" >"$tmp/signed"
  openssl pkeyutl -sign -inkey "$tmp/private.pem" -rawin -in "$tmp/signed" -out "$tmp/signature.raw" >/dev/null 2>&1
  # release-build uses this canonical Base64 form, including one final LF.
  base64 <"$tmp/signature.raw" | tr -d '\n' >"$tmp/release/$artifact.signature"
  printf '\n' >>"$tmp/release/$artifact.signature"
}

checksums() {
  (cd "$tmp/release" && sha256sum \
    tinkercloud-linux-amd64 tinkercloud-linux-amd64.metadata.json tinkercloud-linux-amd64.signature \
    tinkercloud.service tinkercloud.service.metadata.json tinkercloud.service.signature >SHA256SUMS)
}

prepare_release() {
  rm -f "$tmp/release"/*
  printf 'inert tinkercloud candidate\n' >"$tmp/release/tinkercloud-linux-amd64"
  cp "$root/packaging/systemd/tinkercloud.service" "$tmp/release/tinkercloud.service"
  cp "$tmp/install-host.sh" "$tmp/release/install-host.sh"
  sign tinkercloud-linux-amd64
  sign tinkercloud.service
  sign install-host.sh
  checksums
}

cat >"$tmp/bin/id" <<'EOF'
#!/bin/sh
echo "${TINKER_TEST_UID:-0}"
EOF
cat >"$tmp/bin/uname" <<'EOF'
#!/bin/sh
case "$1" in -s) echo Linux;; -m) echo x86_64;; esac
EOF
cat >"$tmp/bin/python3" <<'EOF'
#!/bin/sh
# install-host.sh's first Python invocation validates the origin. Its second
# parses signed release bytes and is delegated to the real interpreter.
case "${2:-}" in
  https://releases.example.test/*|https://releases.example.test) exit 0 ;;
  http://*) exit 1 ;;
esac
exec "$TINKER_TEST_REAL_PYTHON" "$@"
EOF
cat >"$tmp/bin/curl" <<'EOF'
#!/bin/sh
set -eu
out= url= saw_location=0
while test $# -gt 0; do
  case "$1" in
    --output|-o) out=$2; shift 2 ;;
    --location|-L) saw_location=1; shift ;;
    *) url=$1; shift ;;
  esac
done
test -n "$out" && test -n "$url"
case "$url" in "$TINKER_TEST_RELEASE_BASE"*) ;; *) exit 1;; esac
# Simulate a redirect response. A bootstrap that does not opt into redirect
# following must fail the fetch rather than accepting a different origin.
if test "${TINKER_TEST_REDIRECT:-0}" != 0; then
  test "$saw_location" = 0 || exit 1
  exit 1
fi
cp "$TINKER_TEST_RELEASE/${url##*/}" "$out"
EOF
cat >"$tmp/bin/install" <<'EOF'
#!/bin/sh
set -eu
test "$1" = -m
case "$4" in
  /usr/local/bin/.tinkercloud.new) cp "$3" "$TINKER_TEST_MARKERS/binary" ;;
  /etc/systemd/system/.tinkercloud.service.new) cp "$3" "$TINKER_TEST_MARKERS/service" ;;
  *) exit 1 ;;
esac
EOF
cat >"$tmp/bin/mv" <<'EOF'
#!/bin/sh
set -eu
case "$2" in
  /usr/local/bin/tinkercloud|/etc/systemd/system/tinkercloud.service) : ;;
  *) exit 1 ;;
esac
EOF
cat >"$tmp/bin/systemctl" <<'EOF'
#!/bin/sh
test "$1" = daemon-reload
touch "$TINKER_TEST_MARKERS/systemctl"
EOF
chmod +x "$tmp/bin"/*

run_bootstrap() {
  markers=$1
  shift
  rm -rf "$markers"
  mkdir -p "$markers"
  PATH="$tmp/bin:$PATH" TINKER_TEST_REAL_PYTHON="$real_python" \
    TINKER_TEST_RELEASE="$tmp/release" TINKER_TEST_RELEASE_BASE=https://releases.example.test/v0.1.0/ \
    TINKER_TEST_MARKERS="$markers" "$tmp/bootstrap.sh" "$@"
}

prepare_release
run_bootstrap "$tmp/valid" https://releases.example.test/v0.1.0/
test -f "$tmp/valid/binary" && test -f "$tmp/valid/service" && test -f "$tmp/valid/systemctl" ||
  fail "valid signed bootstrap/install path did not complete"
cmp -s "$tmp/valid/service" "$root/packaging/systemd/tinkercloud.service" ||
  fail "valid signed service unit was not the installed unit"

TINKER_TEST_UID=1000 PATH="$tmp/bin:$PATH" TINKER_TEST_REAL_PYTHON="$real_python" \
  "$tmp/bootstrap.sh" https://releases.example.test/v0.1.0/ >/dev/null 2>&1 &&
  fail "bootstrap accepted a non-root caller"
TINKER_TEST_UID=0 PATH="$tmp/bin:$PATH" TINKER_TEST_REAL_PYTHON="$real_python" \
  "$tmp/install-host.sh" http://releases.example.test/v0.1.0/ >/dev/null 2>&1 &&
  fail "host installer accepted an insecure origin"
grep -F -- '--proto-redir' "$tmp/bootstrap.sh" >/dev/null || fail "bootstrap redirect protocol denial missing"
if grep -E -- 'curl[^\n]*(--location|-L)' "$tmp/bootstrap.sh" >/dev/null; then
  fail "bootstrap may follow a cross-origin redirect"
fi
prepare_release
TINKER_TEST_REDIRECT=1 run_bootstrap "$tmp/redirect" https://releases.example.test/v0.1.0/ >/dev/null 2>&1 &&
  fail "bootstrap followed a redirect"
test ! -e "$tmp/redirect/systemctl" || fail "redirect failure reached the installer"

prepare_release
printf '{"version":"0.1.0","api":"1","schema":"1","sha256":"%064d"}\n' 0 >"$tmp/release/install-host.sh.metadata.json"
checksums
run_bootstrap "$tmp/bad-metadata" https://releases.example.test/v0.1.0/ >/dev/null 2>&1 &&
  fail "bootstrap accepted tampered installer metadata"
test ! -e "$tmp/bad-metadata/systemctl" || fail "metadata failure reached the installer"

prepare_release
printf 'invalid signature\n' >"$tmp/release/install-host.sh.signature"
checksums
run_bootstrap "$tmp/bad-signature" https://releases.example.test/v0.1.0/ >/dev/null 2>&1 &&
  fail "bootstrap accepted tampered installer signature"
test ! -e "$tmp/bad-signature/systemctl" || fail "signature failure reached the installer"

prepare_release
sed 's/./& /2' "$tmp/release/install-host.sh.signature" >"$tmp/inner-signature"
mv "$tmp/inner-signature" "$tmp/release/install-host.sh.signature"
checksums
run_bootstrap "$tmp/inner-bootstrap-signature" https://releases.example.test/v0.1.0/ >/dev/null 2>&1 &&
  fail "bootstrap accepted internal signature whitespace"
test ! -e "$tmp/inner-bootstrap-signature/systemctl" || fail "internal bootstrap signature junk reached the installer"

prepare_release
printf '\n# unsigned service-unit tamper\n' >>"$tmp/release/tinkercloud.service"
# An attacker can recompute unsigned SHA256SUMS; the signed service evidence
# must still deny before either replacement or systemd reload.
checksums
run_bootstrap "$tmp/bad-service" https://releases.example.test/v0.1.0/ >/dev/null 2>&1 &&
  fail "host installer accepted a tampered signed service unit"
test ! -e "$tmp/bad-service/systemctl" || fail "service-unit tamper reached systemctl"

prepare_release
sed 's/./&\t/2' "$tmp/release/tinkercloud.service.signature" >"$tmp/inner-service-signature"
mv "$tmp/inner-service-signature" "$tmp/release/tinkercloud.service.signature"
checksums
run_bootstrap "$tmp/inner-service-signature" https://releases.example.test/v0.1.0/ >/dev/null 2>&1 &&
  fail "host installer accepted internal service signature whitespace"
test ! -e "$tmp/inner-service-signature/systemctl" || fail "internal service signature junk reached systemctl"

echo "host bootstrap offline evidence passed"
