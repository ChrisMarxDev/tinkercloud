#!/bin/sh
# Deterministic no-network self-test for the unattended wrapper.
set -eu
umask 077

root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd -P)
runner="$root/skills/tinkercloud-full-stack-test/scripts/run-unattended.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir -m 700 "$tmp/bin" "$tmp/reports"
log="$tmp/calls"

for command in python3 git; do
  cat > "$tmp/bin/$command" <<'EOF'
#!/bin/sh
printf '%s %s\n' "$(basename "$0")" "$*" >> "$TINKERCLOUD_UNATTENDED_TEST_LOG"
exit 0
EOF
  chmod 700 "$tmp/bin/$command"
done
cat > "$tmp/bin/go" <<'EOF'
#!/bin/sh
printf 'go %s\n' "$*" >> "$TINKERCLOUD_UNATTENDED_TEST_LOG"
case " $* " in *' -run TestVPSAcceptance '*) exit "${TINKERCLOUD_UNATTENDED_TEST_LIVE_EXIT:-0}";; esac
case " $* " in *' -skip ^TestVPSAcceptance$ '*) exit "${TINKERCLOUD_UNATTENDED_TEST_OFFLINE_EXIT:-0}";; esac
exit 97
EOF
chmod 700 "$tmp/bin/go"

make_file() { : > "$1"; chmod 600 "$1"; }
make_file "$tmp/known_hosts"
make_file "$tmp/send-key"
make_file "$tmp/reader-key"
make_file "$tmp/ledger.json"

run() {
  PATH="$tmp/bin:$PATH" TINKERCLOUD_UNATTENDED_TEST_LOG="$log" \
  TINKERCLOUD_VPS_UNATTENDED_REPORT_DIR="$tmp/reports" \
  TINKERCLOUD_VPS_E2E=1 TINKERCLOUD_VPS_SSH_TARGET=root@203.0.113.10 \
  TINKERCLOUD_VPS_ACKNOWLEDGE=root@203.0.113.10 \
  TINKERCLOUD_VPS_KNOWN_HOSTS_FILE="$tmp/known_hosts" \
  TINKERCLOUD_VPS_DOMAIN=example.test \
  TINKERCLOUD_VPS_OPERATOR_EMAIL=operator@example.test TINKERCLOUD_VPS_DEPLOYER_EMAIL=deployer@example.test \
  TINKERCLOUD_VPS_VIEWER_EMAIL=viewer@example.test TINKERCLOUD_VPS_EMAIL_FROM=tinker@example.test \
  TINKERCLOUD_VPS_RESEND_API_KEY_FILE="$tmp/send-key" \
  TINKERCLOUD_RESEND_READER_API_KEY_FILE="$tmp/reader-key" TINKERCLOUD_RESEND_OTP_LEDGER_FILE="$tmp/ledger.json" \
  TINKERCLOUD_VPS_OTP_COMMAND="$root/skills/tinkercloud-full-stack-test/scripts/read-resend-otp.py" "$runner"
}

run >/dev/null 2>&1
report=$(find "$tmp/reports" -type f -name '*.status')
[ "$(stat -f %Lp "$report" 2>/dev/null || stat -c %a "$report")" = 600 ]
grep -qx 'preflight=passed' "$report"
grep -qx 'offline_reader=passed' "$report"
grep -qx 'live_vps_acceptance=passed' "$report"
grep -qx 'result=passed' "$report"
if grep -Eq 'send-key|reader-key|ledger|203\.0\.113\.10|operator@example' "$report"; then
  echo "status artifact exposed a local path or input value" >&2
  exit 1
fi
[ "$(grep -c '^go test ./test/vps -run TestVPSAcceptance -count=1 -v$' "$log")" = 1 ]
[ "$(grep -c '^go test ./test/vps -skip \^TestVPSAcceptance\$ -count=1$' "$log")" = 1 ]
[ "$(grep -c '^go test ./test/vps ' "$log")" = 2 ]

: > "$log"
if env -u TINKERCLOUD_VPS_E2E PATH="$tmp/bin:$PATH" TINKERCLOUD_UNATTENDED_TEST_LOG="$log" TINKERCLOUD_VPS_UNATTENDED_REPORT_DIR="$tmp/reports" "$runner" >/dev/null 2>&1; then
  echo "missing gate unexpectedly succeeded" >&2
  exit 1
fi
failed=$(find "$tmp/reports" -type f -name '*.status' | sort | tail -n 1)
grep -qx 'preflight=failed' "$failed"
grep -qx 'result=failed' "$failed"
[ ! -s "$log" ]

: > "$log"
if (
  export TINKERCLOUD_VPS_REUSE=1
  unset TINKERCLOUD_VPS_RELEASE_DIR
  run
) >/dev/null 2>&1; then
  echo "reuse without a release directory unexpectedly succeeded" >&2
  exit 1
fi
failed=$(find "$tmp/reports" -type f -name '*.status' | sort | tail -n 1)
grep -qx 'preflight=failed' "$failed"
grep -qx 'result=failed' "$failed"
[ ! -s "$log" ]
