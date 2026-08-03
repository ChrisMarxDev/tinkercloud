#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
fixture=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-beta-onboarding.XXXXXX")
trap 'rm -rf "$fixture"' EXIT HUP INT TERM

mkdir -p "$fixture/scripts" "$fixture/sdk/typescript/src" "$fixture/skills" "$fixture/specs/agent" "$fixture/specs/control" "$fixture/docs/getting-started"
cp "$repo_root/scripts/test-beta-onboarding-docs.sh" "$fixture/scripts/"
cp "$repo_root/scripts/extract-sdk-version.mjs" "$fixture/scripts/"
cp "$repo_root/sdk/typescript/src/index.ts" "$fixture/sdk/typescript/src/"
cp -R "$repo_root/skills/tinkercloud-platform" "$fixture/skills/"
cp -R "$repo_root/skills/tinkercloud-operator" "$fixture/skills/"
cp -R "$repo_root/skills/tinkercloud-deployer" "$fixture/skills/"
cp -R "$repo_root/skills/tinkercloud-full-stack-test" "$fixture/skills/"
cp "$repo_root/README.md" "$fixture/README.md"
cp "$repo_root/CHANGELOG.md" "$fixture/CHANGELOG.md"
cp "$repo_root/specs/agent/beta-onboarding-contract.md" "$fixture/specs/agent/"
cp "$repo_root/specs/control/deployment-contract.md" "$fixture/specs/control/"
cp "$repo_root/docs/getting-started/first-app.md" "$fixture/docs/getting-started/"

perl -0pi -e 's/sudo tinkercloud setup\n//; s/Setup derives `https:\/\/admin\.<domain>\/login`//; s/Sign in there with one human browser\nOTP only when there is no reusable browser identity//; s/and record the no-redirect `https:\/\/admin\.<domain>\/api\/v1\/version` gateway proof//;' "$fixture/README.md"

set +e
output=$(
  {
    cd "$fixture"
    ./scripts/test-beta-onboarding-docs.sh
  } 2>&1
)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected operator-evidence removal to fail" >&2
  exit 1
}

expected_output=$(cat <<'EOF'
beta onboarding documentation missing from README Start the beta: sudo tinkercloud setup
beta onboarding documentation missing from README Start the beta: Setup derives `https://admin.<domain>/login`
beta onboarding documentation missing from README Start the beta: Sign in there with one human browser OTP only when there is no reusable browser identity
beta onboarding documentation missing from README Start the beta: record the no-redirect `https://admin.<domain>/api/v1/version` gateway proof
beta onboarding documentation missing or reordered README operator-flow step: sudo tinkercloud setup
EOF
)

test "$output" = "$expected_output" || {
  echo "beta onboarding documentation self-test expected exactly five operator-evidence diagnostics" >&2
  echo "expected:" >&2
  printf '%s\n' "$expected_output" >&2
  echo "actual:" >&2
  printf '%s\n' "$output" >&2
  exit 1
}

# Exact release/package references may not silently fall back to the prior beta.
cp "$repo_root/README.md" "$fixture/README.md"
perl -0pi -e 's/@tinkercloud\/cli\@0\.1\.6/@tinkercloud\/cli\@0.1.5/' "$fixture/README.md"

set +e
output=$(
  {
    cd "$fixture"
    ./scripts/test-beta-onboarding-docs.sh
  } 2>&1
)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected prior package version to fail" >&2
  exit 1
}

printf '%s\n' "$output" | grep -F -- 'beta onboarding documentation has a non-0.1.6 exact release/package reference in README.md' >/dev/null || {
  echo "beta onboarding documentation self-test missed prior package version" >&2
  exit 1
}

# A prepared beta must not be represented as a dated published changelog release.
cp "$repo_root/CHANGELOG.md" "$fixture/CHANGELOG.md"
perl -0pi -e 's/### Planned for 0\.1\.6/## [0.1.6] - 2026-08-03/; s/\[0\.1\.5\]:/\[0.1.6\]: https:\/\/github.com\/ChrisMarxDev\/tinkercloud\/compare\/v0.1.5...v0.1.6\n[0.1.5]:/' "$fixture/CHANGELOG.md"

set +e
output=$(
  {
    cd "$fixture"
    ./scripts/test-beta-onboarding-docs.sh
  } 2>&1
)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected published changelog claim to fail" >&2
  exit 1
}

for expected in \
  'beta onboarding documentation missing from CHANGELOG.md: ### Planned for 0.1.6' \
  'beta onboarding documentation contains forbidden text in CHANGELOG.md: ## [0.1.6] -' \
  'beta onboarding documentation contains forbidden text in CHANGELOG.md: [0.1.6]:'; do
  printf '%s\n' "$output" | grep -F -- "$expected" >/dev/null || {
    echo "beta onboarding documentation self-test missed pre-publication changelog safeguard: $expected" >&2
    exit 1
  }
done

# The standalone operator skill must carry the executable beta path itself.
cp "$repo_root/README.md" "$fixture/README.md"
perl -0pi -e 's/scp -p -- "<LOCAL_CREDENTIAL_FILE>" "root\@<HOST>:\/root\/\.config\/tinkercloud\/resend-api-key"/copy credential separately/; s/1\. `Base domain:`\n2\. `Operator email:`/1. `Operator email:`\n2. `Base domain:`/; s/\*\*Deployers\*\*, then \*\*Active deployer allowlist\*\*/**Active deployer allowlist**, then **Deployers**/; s/VPS provider console is the trust source for the SSH host-key fingerprint/ssh-keyscan is the trust source/; s/uname -m && \. \/etc\/os-release && printf/uname -m && printf/; s/Create one `\*\.<DOMAIN>` wildcard record/Create a separate admin record/; s/\(dig \+short A admin\.<DOMAIN>; dig \+short AAAA admin\.<DOMAIN>\)/dig +short A admin.<DOMAIN>/; s/\(dig \+short A onboarding-check\.<DOMAIN>; dig \+short AAAA onboarding-check\.<DOMAIN>\)/dig +short A onboarding-check.<DOMAIN>/; s/copy\/paste installer verifies checksums/copy\/paste installer downloads/; s/no other addresses/optional extra addresses/; s/uses zero when a reusable browser identity is valid/uses one even when a reusable browser identity is valid/' "$fixture/skills/tinkercloud-operator/SKILL.md"
perl -0pi -e 's/getent ahosts admin\.<DOMAIN> >\/dev\/null/getent omitted/; s/getent ahosts onboarding-check\.<DOMAIN> >\/dev\/null/getent omitted/' "$fixture/skills/tinkercloud-operator/SKILL.md"

set +e
output=$(
  {
    cd "$fixture"
    ./scripts/test-beta-onboarding-docs.sh
  } 2>&1
)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected standalone operator path removal to fail" >&2
  exit 1
}

for expected in \
  'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: scp -p -- "<LOCAL_CREDENTIAL_FILE>" "root@<HOST>:/root/.config/tinkercloud/resend-api-key"' \
  'beta onboarding documentation missing ordered operator-flow block in skills/tinkercloud-operator/SKILL.md: 1. `Base domain:`' \
  'beta onboarding documentation missing ordered operator-flow block in skills/tinkercloud-operator/SKILL.md: human browser OTP. In the dashboard select **Deployers**, then **Active deployer allowlist**; enter the reviewed normalized set in **Allowed deployer emails**.' \
  'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: VPS provider console is the trust source for the SSH host-key fingerprint' \
  'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: uname -m && . /etc/os-release && printf' \
  'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: Create one `*.<DOMAIN>` wildcard record using `A`/`AAAA` or `CNAME` as the DNS provider supports.' \
  'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: getent ahosts admin.<DOMAIN> >/dev/null' \
  'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: getent ahosts onboarding-check.<DOMAIN> >/dev/null' \
  'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: The copy/paste installer verifies checksums and a pinned Ed25519 signature before installation.' \
  'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: Replace the initial active deployer allowlist with exactly the intended normalized deployer email set and no other addresses; the operator email alone is sufficient only when that is the exact intended set.' \
  'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: Operator setup permits at most one human browser OTP, uses zero when a reusable browser identity is valid, and uses no CLI deployer OTP.'; do
  printf '%s\n' "$output" | grep -F -- "$expected" >/dev/null || {
    echo "beta onboarding documentation self-test missed standalone operator safeguard: $expected" >&2
    exit 1
  }
done

# Retaining every step out of order must fail specifically on flow ordering.
cp "$repo_root/skills/tinkercloud-operator/SKILL.md" "$fixture/skills/tinkercloud-operator/SKILL.md"
perl -0pi -e 's/### 2\. Verify the SSH host key/### ORDER-SWAP/; s/### 3\. Configure one wildcard DNS record/### 2. Verify the SSH host key/; s/### ORDER-SWAP/### 3. Configure one wildcard DNS record/' "$fixture/skills/tinkercloud-operator/SKILL.md"

set +e
output=$(
  {
    cd "$fixture"
    ./scripts/test-beta-onboarding-docs.sh
  } 2>&1
)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected operator flow reorder to fail" >&2
  exit 1
}

expected='beta onboarding documentation missing or reordered operator-flow step in skills/tinkercloud-operator/SKILL.md: ssh-keygen -l -f /etc/ssh/ssh_host_ed25519_key.pub'
printf '%s\n' "$output" | grep -F -- "$expected" >/dev/null || {
  echo "beta onboarding documentation self-test missed operator flow reorder diagnostic" >&2
  exit 1
}

check_completion_mutation() {
  mutation=$1
  expected=$2
  cp "$repo_root/skills/tinkercloud-operator/SKILL.md" "$fixture/skills/tinkercloud-operator/SKILL.md"
  case "$mutation" in
    status)
      perl -0pi -e 's/sudo tinkercloud status/status omitted/' "$fixture/skills/tinkercloud-operator/SKILL.md"
      ;;
    doctor)
      perl -0pi -e 's/sudo tinkercloud doctor/doctor omitted/' "$fixture/skills/tinkercloud-operator/SKILL.md"
      ;;
    version)
      perl -0pi -e 's/curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https:\/\/admin\.<domain>\/api\/v1\/version/version proof omitted/' "$fixture/skills/tinkercloud-operator/SKILL.md"
      ;;
    evidence)
      perl -0pi -e 's/Completion requires HTTP `200`,\s+an `application\/json` media type, and exact bounded body `\{"api_version":1\}`\s+with no extra or error fields/unbounded response/' "$fixture/skills/tinkercloud-operator/SKILL.md"
      ;;
    boundary)
      perl -0pi -e 's/Protected-app/anonymous/' "$fixture/skills/tinkercloud-operator/SKILL.md"
      ;;
  esac

  set +e
  output=$(
    {
      cd "$fixture"
      ./scripts/test-beta-onboarding-docs.sh
    } 2>&1
  )
  result=$?
  set -e

  test "$result" -ne 0 || {
    echo "beta onboarding documentation self-test expected ${mutation} completion removal to fail" >&2
    exit 1
  }
  printf '%s\n' "$output" | grep -F -- "$expected" >/dev/null || {
    echo "beta onboarding documentation self-test missed ${mutation} completion diagnostic" >&2
    exit 1
  }
}

check_completion_mutation status \
  'beta onboarding documentation missing operator completion status evidence in skills/tinkercloud-operator/SKILL.md: sudo tinkercloud status'
check_completion_mutation doctor \
  'beta onboarding documentation missing operator completion doctor evidence in skills/tinkercloud-operator/SKILL.md: sudo tinkercloud doctor'
check_completion_mutation version \
  'beta onboarding documentation missing operator completion version command in skills/tinkercloud-operator/SKILL.md: curl --no-location --fail-with-body --include --max-time 15 --max-filesize 32768 https://admin.<domain>/api/v1/version'
check_completion_mutation evidence \
  'beta onboarding documentation missing operator completion header/body evidence in skills/tinkercloud-operator/SKILL.md: Completion requires HTTP `200`, an `application/json` media type, and exact bounded body `{"api_version":1}` with no extra or error fields'
check_completion_mutation boundary \
  'beta onboarding documentation missing operator completion anonymous-denial boundary in skills/tinkercloud-operator/SKILL.md: Protected-app anonymous denial belongs to deployer deployment completion once an app exists; do not fabricate it during empty operator setup.'

# A failed operator browser OTP is terminal for onboarding, independently of the
# unattended machine OTP path.
cp "$repo_root/skills/tinkercloud-operator/SKILL.md" "$fixture/skills/tinkercloud-operator/SKILL.md"
perl -0pi -e 's/stop operator onboarding/continue operator onboarding/' "$fixture/skills/tinkercloud-operator/SKILL.md"

set +e
output=$(cd "$fixture" && ./scripts/test-beta-onboarding-docs.sh 2>&1)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected terminal operator OTP mutation to fail" >&2
  exit 1
}
printf '%s\n' "$output" | grep -F -- 'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: Terminal operator browser authentication rule: immediately after an operator browser OTP attempt fails, is malformed, times out, or is denied, stop operator onboarding.' >/dev/null || {
  echo "beta onboarding documentation self-test missed terminal operator OTP diagnostic" >&2
  exit 1
}

# The fully fresh human path requires exactly its two nonfungible role codes.
cp "$repo_root/skills/tinkercloud-operator/SKILL.md" "$fixture/skills/tinkercloud-operator/SKILL.md"
perl -0pi -e 's/requests exactly two codes total/requests one or two codes total/' "$fixture/skills/tinkercloud-operator/SKILL.md"

set +e
output=$(cd "$fixture" && ./scripts/test-beta-onboarding-docs.sh 2>&1)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected exact human OTP-count mutation to fail" >&2
  exit 1
}
printf '%s\n' "$output" | grep -F -- 'beta onboarding documentation missing from skills/tinkercloud-operator/SKILL.md: A fully fresh successful human onboarding with neither a reusable browser identity nor a saved CLI bearer requests exactly two codes total: exactly one operator browser code and exactly one deployer CLI code. A reusable identity reduces the relevant lane to zero.' >/dev/null || {
  echo "beta onboarding documentation self-test missed exact human OTP-count diagnostic" >&2
  exit 1
}

# Moving the intact completion block before dashboard authorization must fail.
cp "$repo_root/skills/tinkercloud-operator/SKILL.md" "$fixture/skills/tinkercloud-operator/SKILL.md"
perl -0pi -e 's{(### 7\. Authorize the exact deployer set.*?)(### 8\. Verify operator completion.*?)(<!-- beta-operator-clean:end -->)}{$2$1$3}s' "$fixture/skills/tinkercloud-operator/SKILL.md"

set +e
output=$(
  {
    cd "$fixture"
    ./scripts/test-beta-onboarding-docs.sh
  } 2>&1
)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected early operator completion to fail" >&2
  exit 1
}

expected='beta onboarding documentation missing or reordered operator-flow step in skills/tinkercloud-operator/SKILL.md: ### 8. Verify operator completion'
printf '%s\n' "$output" | grep -F -- "$expected" >/dev/null || {
  echo "beta onboarding documentation self-test missed early completion ordering diagnostic" >&2
  exit 1
}

# A failed CLI authentication or OTP transaction is terminal for that deploy.
# Each wording boundary and the position immediately after the normal auth flow
# is independently mutation-tested in the standalone deployer skill.
check_terminal_cli_auth_mutation() {
  mutation=$1
  expected=$2
  cp "$repo_root/skills/tinkercloud-deployer/SKILL.md" "$fixture/skills/tinkercloud-deployer/SKILL.md"
  case "$mutation" in
    stop) perl -0pi -e 's/stop that deploy attempt/continue deployment/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    retry) perl -0pi -e 's/Do not retry `tinker login`/Retry login/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    force) perl -0pi -e 's/use `--force`/use force/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    switch) perl -0pi -e 's/switch account or identity/switch identity/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    logout) perl -0pi -e 's/log out/sign out/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    clear) perl -0pi -e 's/delete or clear saved credentials/clear credentials elsewhere/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    forcepath) perl -0pi -e 's/alternate OTP path/another OTP path/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    zero) perl -0pi -e 's/valid\s+exact server-scoped saved identity uses zero OTP/valid saved identity may use OTP/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    cli) perl -0pi -e 's/directly\s+in the CLI, never chat/through any channel/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    ordering) perl -0pi -e 's/^Terminal CLI authentication rule:/Former terminal rule:/m; s/For a known server, run /Terminal CLI authentication rule: misplaced\n\nFor a known server, run /' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
  esac
  set +e
  output=$(cd "$fixture" && ./scripts/test-beta-onboarding-docs.sh 2>&1)
  result=$?
  set -e
  test "$result" -ne 0 || { echo "beta onboarding documentation self-test expected ${mutation} terminal CLI-auth mutation to fail" >&2; exit 1; }
  test "$expected" = '-' && return
  printf '%s\n' "$output" | grep -F -- "$expected" >/dev/null || { echo "beta onboarding documentation self-test missed ${mutation} terminal CLI-auth diagnostic" >&2; exit 1; }
}

check_terminal_cli_auth_mutation stop 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: stop that deploy attempt'
check_terminal_cli_auth_mutation retry 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: Do not retry `tinker login`'
check_terminal_cli_auth_mutation force 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: use `--force`'
check_terminal_cli_auth_mutation switch 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: switch account or identity'
check_terminal_cli_auth_mutation logout 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: log out'
check_terminal_cli_auth_mutation clear 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: delete or clear saved credentials'
check_terminal_cli_auth_mutation forcepath 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: alternate OTP path'
check_terminal_cli_auth_mutation zero 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: valid exact server-scoped saved identity uses zero OTP'
check_terminal_cli_auth_mutation cli 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: directly in the CLI, never chat'
check_terminal_cli_auth_mutation ordering '-'

# Authentication owns only Email/Code prompts; the six manifest prompts remain
# legitimate human prompts later in the same workflow.
cp "$repo_root/skills/tinkercloud-deployer/SKILL.md" "$fixture/skills/tinkercloud-deployer/SKILL.md"
perl -0pi -e 's/only\s+authentication prompts are exactly/only human prompts are exactly/' "$fixture/skills/tinkercloud-deployer/SKILL.md"

set +e
output=$(cd "$fixture" && ./scripts/test-beta-onboarding-docs.sh 2>&1)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected authentication-prompt wording regression to fail" >&2
  exit 1
}
for expected in \
  'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: only authentication prompts are exactly `Email: ` and `Code: `' \
  'beta onboarding documentation contains forbidden text in skills/tinkercloud-deployer/SKILL.md: only human prompts are exactly `Email: ` and `Code: `'; do
  printf '%s\n' "$output" | grep -F -- "$expected" >/dev/null || {
    echo "beta onboarding documentation self-test missed authentication-prompt wording diagnostic: $expected" >&2
    exit 1
  }
done

# The standalone deployer skill must retain the exact fully-fresh two-code rule.
cp "$repo_root/skills/tinkercloud-deployer/SKILL.md" "$fixture/skills/tinkercloud-deployer/SKILL.md"
perl -0pi -e 's/requests exactly two codes total/requests one or two codes total/' "$fixture/skills/tinkercloud-deployer/SKILL.md"

set +e
output=$(cd "$fixture" && ./scripts/test-beta-onboarding-docs.sh 2>&1)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected standalone deployer OTP-count mutation to fail" >&2
  exit 1
}
printf '%s\n' "$output" | grep -F -- 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: A fully fresh successful human onboarding with neither a reusable browser identity nor a saved CLI bearer requests exactly two codes total: exactly one operator browser code and exactly one deployer CLI code. A reusable identity reduces the relevant lane to zero.' >/dev/null || {
  echo "beta onboarding documentation self-test missed standalone deployer OTP-count diagnostic" >&2
  exit 1
}

# A reviewed deploy command is invoked once. Internal readiness retries and the
# one exact-URL recheck are not authority to create a second deployment.
check_single_deploy_invocation_mutation() {
  mutation=$1
  expected=$2
  cp "$repo_root/skills/tinkercloud-deployer/SKILL.md" "$fixture/skills/tinkercloud-deployer/SKILL.md"
  case "$mutation" in
    command) perl -0pi -e 's/invoke `tinker deploy \.`/invoke the deploy command/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    once) perl -0pi -e 's/exactly once for that deploy attempt/as often as needed/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    rerun) perl -0pi -e 's/do not rerun deploy/rerun deploy/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    upload) perl -0pi -e 's/upload another release/upload a replacement release/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    chat) perl -0pi -e 's/retry from chat/retry elsewhere/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    internal) perl -0pi -e 's/already-bounded transient readiness retries/unbounded readiness retries/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    recheck) perl -0pi -e 's/independent exact-URL recheck/another verification/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    second) perl -0pi -e 's/never a second deployment/a second deployment is allowed/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    human) perl -0pi -e 's/explicit new human\s+request/automatic retry/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
    ordering) perl -0pi -e 's/^One-invocation deployment rule:/Former invocation rule:/m; s/Terminal CLI authentication rule:/One-invocation deployment rule: misplaced\n\nTerminal CLI authentication rule:/' "$fixture/skills/tinkercloud-deployer/SKILL.md" ;;
  esac
  set +e
  output=$(cd "$fixture" && ./scripts/test-beta-onboarding-docs.sh 2>&1)
  result=$?
  set -e
  test "$result" -ne 0 || { echo "beta onboarding documentation self-test expected ${mutation} single-invocation mutation to fail" >&2; exit 1; }
  test "$expected" = '-' && return
  printf '%s\n' "$output" | grep -F -- "$expected" >/dev/null || { echo "beta onboarding documentation self-test missed ${mutation} single-invocation diagnostic" >&2; exit 1; }
}

check_single_deploy_invocation_mutation command 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: invoke `tinker deploy .`'
check_single_deploy_invocation_mutation once 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: exactly once for that deploy attempt'
check_single_deploy_invocation_mutation rerun '-'
check_single_deploy_invocation_mutation upload 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: upload another release'
check_single_deploy_invocation_mutation chat 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: retry from chat'
check_single_deploy_invocation_mutation internal 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: already-bounded transient readiness retries'
check_single_deploy_invocation_mutation recheck 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: independent exact-URL recheck'
check_single_deploy_invocation_mutation second 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: never a second deployment'
check_single_deploy_invocation_mutation human 'beta onboarding documentation missing from skills/tinkercloud-deployer/SKILL.md: explicit new human request'
check_single_deploy_invocation_mutation ordering '-'

# An executable deploy command before the final review must fail even when the
# canonical deploy command and one-invocation wording remain intact.
cp "$repo_root/skills/tinkercloud-deployer/SKILL.md" "$fixture/skills/tinkercloud-deployer/SKILL.md"
perl -0pi -e 's{(### 2\. Propose the access policy)}{```sh\ntinker deploy .\n```\n\n$1}' "$fixture/skills/tinkercloud-deployer/SKILL.md"

set +e
output=$(cd "$fixture" && ./scripts/test-beta-onboarding-docs.sh 2>&1)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected early executable deploy command to fail" >&2
  exit 1
}
printf '%s\n' "$output" | grep -F -- 'beta onboarding documentation has executable deploy command before final review in skills/tinkercloud-deployer/SKILL.md: tinker deploy .' >/dev/null || {
  echo "beta onboarding documentation self-test missed early executable deploy diagnostic" >&2
  exit 1
}

# Moving the canonical command before the final review is equally invalid.
cp "$repo_root/skills/tinkercloud-deployer/SKILL.md" "$fixture/skills/tinkercloud-deployer/SKILL.md"
perl -0pi -e 's{```sh\ntinker deploy \.\n```}{}; s{(### 5\. Deploy and verify)}{```sh\ntinker deploy .\n```\n\n$1}' "$fixture/skills/tinkercloud-deployer/SKILL.md"

set +e
output=$(cd "$fixture" && ./scripts/test-beta-onboarding-docs.sh 2>&1)
result=$?
set -e

test "$result" -ne 0 || {
  echo "beta onboarding documentation self-test expected moved executable deploy command to fail" >&2
  exit 1
}
printf '%s\n' "$output" | grep -F -- 'beta onboarding documentation has executable deploy command before final review in skills/tinkercloud-deployer/SKILL.md: tinker deploy .' >/dev/null || {
  echo "beta onboarding documentation self-test missed moved executable deploy diagnostic" >&2
  exit 1
}

# This focused Taskfile composition test proves `check` reaches the docs
# mutation self-test through `skills:check` exactly once, without independently
# running the happy docs checker a second time. Do not invoke Task recursively:
# this script itself runs under `task test:beta-onboarding-docs`.
taskfile=$(cat "$repo_root/Taskfile.yml")
while IFS= read -r command; do
  count=$(printf '%s\n' "$taskfile" | grep -F -c -- "- $command" || true)
  test "$count" -eq 1 || {
    echo "beta onboarding documentation self-test expected Taskfile to declare ${command} exactly once; found ${count}" >&2
    exit 1
  }
done <<'EOF'
./scripts/test-beta-onboarding-docs.sh
./scripts/test-beta-onboarding-docs-self-test.sh
python3 test/security/test_beta_onboarding_docs.py
./scripts/check-skill-drift
EOF

skills_block=$(sed -n '/^  skills:check:/,/^  [^ ]/p' "$repo_root/Taskfile.yml")
printf '%s\n' "$skills_block" | grep -F -- 'deps: [test:beta-onboarding-docs]' >/dev/null || {
  echo "beta onboarding documentation self-test expected skills:check to depend on test:beta-onboarding-docs" >&2
  exit 1
}
check_block=$(sed -n '/^  check:/,/^  [^ ]/p' "$repo_root/Taskfile.yml")
! printf '%s\n' "$check_block" | grep -F -- 'test:beta-onboarding-docs' >/dev/null || {
  echo "beta onboarding documentation self-test expected check to compose docs verification only through skills:check" >&2
  exit 1
}

! grep -F -- './scripts/test-beta-onboarding-docs.sh' "$repo_root/scripts/check-skill-drift" >/dev/null || {
  echo "beta onboarding documentation self-test expected skill drift to avoid rerunning the happy docs checker" >&2
  exit 1
}
