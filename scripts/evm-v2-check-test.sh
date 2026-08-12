#!/usr/bin/env bash
# Regression tests for the EVM V2 gate's fail-closed required-file checks.
# Fixtures are isolated under a temporary root and never compile or deploy.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATE="$ROOT/scripts/evm-v2-check.sh"
TEMP_ROOT="$(mktemp -d)"
trap 'rm -rf -- "$TEMP_ROOT"' EXIT

required_files=(
  foundry.toml
  src/RepoRegistryV2.sol
  src/RepoRegistryV2ImportController.sol
  src/RepoRegistryV2BadgeModule.sol
  src/RepoRegistryV2EconomicModule.sol
  src/RepoRegistryV2ModerationModule.sol
  test/RepoRegistryV2.t.sol
  test/RepoRegistryV2ImportController.t.sol
  test/RepoRegistryV2BadgeModule.t.sol
  test/RepoRegistryV2EconomicModule.t.sol
  test/RepoRegistryV2ModerationModule.t.sol
  test/RepoRegistryV2Invariant.t.sol
  test/RepoRegistryV2Gas.t.sol
  abi/RepoRegistryV2.json
  abi/RepoRegistryV2ImportController.json
  abi/RepoRegistryV2BadgeModule.json
  abi/RepoRegistryV2EconomicModule.json
  abi/RepoRegistryV2ModerationModule.json
  README.md
)

run_required_file_failure() {
  local case_name="$1"
  local target="$2"
  local state="$3"
  local fixture="$TEMP_ROOT/$case_name"
  local relative path output

  mkdir -p "$fixture/scripts" "$fixture/contracts/evm-v2"
  cp -- "$GATE" "$fixture/scripts/evm-v2-check.sh"
  for relative in "${required_files[@]}"; do
    path="$fixture/contracts/evm-v2/$relative"
    mkdir -p "$(dirname "$path")"
    printf 'fixture\n' > "$path"
  done
  case "$state" in
    missing) rm -f -- "$fixture/contracts/evm-v2/$target" ;;
    empty) : > "$fixture/contracts/evm-v2/$target" ;;
    *) echo "unknown fixture state: $state" >&2; exit 2 ;;
  esac

  if output="$(bash "$fixture/scripts/evm-v2-check.sh" 2>&1)"; then
    echo "$case_name unexpectedly passed" >&2
    exit 1
  fi
  if ! grep -Fq "missing or empty EVM V2 file: contracts/evm-v2/$target" <<< "$output"; then
    echo "$case_name failed for the wrong reason" >&2
    printf '%s\n' "$output" >&2
    exit 1
  fi
}

run_required_file_failure economic-abi-empty abi/RepoRegistryV2EconomicModule.json empty
run_required_file_failure invariant-test-missing test/RepoRegistryV2Invariant.t.sol missing
run_required_file_failure gas-test-missing test/RepoRegistryV2Gas.t.sol missing

abi_fixture="$TEMP_ROOT/abi-compare"
mkdir -p \
  "$abi_fixture/out/RepoRegistryV2.sol" \
  "$abi_fixture/out/RepoRegistryV2ImportController.sol" \
  "$abi_fixture/out/RepoRegistryV2BadgeModule.sol" \
  "$abi_fixture/out/RepoRegistryV2EconomicModule.sol" \
  "$abi_fixture/out/RepoRegistryV2ModerationModule.sol" \
  "$abi_fixture/abi"
for name in RepoRegistryV2 RepoRegistryV2ImportController RepoRegistryV2BadgeModule RepoRegistryV2EconomicModule RepoRegistryV2ModerationModule; do
  artifact_dir="$abi_fixture/out/$name.sol"
  printf '{"abi":[{"type":"function","name":"probe","inputs":[],"outputs":[]}]}' > "$artifact_dir/$name.json"
  printf '[{"type":"function","name":"probe","inputs":[],"outputs":[]}]' > "$abi_fixture/abi/$name.json"
done
node "$ROOT/scripts/evm-v2-foundry-abi-check.mjs" "$abi_fixture" >/dev/null
printf '[]' > "$abi_fixture/abi/RepoRegistryV2EconomicModule.json"
if abi_output="$(node "$ROOT/scripts/evm-v2-foundry-abi-check.mjs" "$abi_fixture" 2>&1)"; then
  echo "stale ABI fixture unexpectedly passed" >&2
  exit 1
fi
if ! grep -Fq 'contracts/evm-v2/abi/RepoRegistryV2EconomicModule.json is stale' <<< "$abi_output"; then
  echo "stale ABI fixture failed for the wrong reason" >&2
  printf '%s\n' "$abi_output" >&2
  exit 1
fi

echo "EVM V2 gate fail-closed regression: pass"
