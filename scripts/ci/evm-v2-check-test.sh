#!/usr/bin/env bash
# Regression tests for the EVM V2 gate's fail-closed required-file checks.
# Fixtures are isolated under a temporary root and never compile or deploy.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ ! -f "$ROOT/CLAUDE.md" && ! -f "$ROOT/README.md" ]]; then
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fi
if [[ ! -f "$ROOT/CLAUDE.md" ]]; then
  ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo "$ROOT")"
fi
GATE="$ROOT/scripts/ci/evm-v2-check.sh"
TEMP_ROOT="$(mktemp -d)"
trap 'rm -rf -- "$TEMP_ROOT"' EXIT

required_files=(
  foundry.toml
  src/SuiteDirectory.sol
  src/BootstrapCoordinator.sol
  src/modules/RepositoryCore.sol
  src/modules/RecoveryModule.sol
  src/modules/ModerationModule.sol
  src/modules/EconomicModule.sol
  src/modules/UsernameModule.sol
  src/modules/BadgeModule.sol
  src/modules/ReleaseModule.sol
  src/suite/ISuite.sol
  src/suite/SuiteModule.sol
  test/SuiteArchitecture.t.sol
  abi/SuiteDirectory.json
  abi/BootstrapCoordinator.json
  abi/RepositoryCore.json
  abi/RecoveryModule.json
  abi/ModerationModule.json
  abi/EconomicModule.json
  abi/UsernameModule.json
  abi/BadgeModule.json
  abi/ReleaseModule.json
  artifacts/SuiteDirectory.json
  artifacts/BootstrapCoordinator.json
  artifacts/RepositoryCore.json
  artifacts/RecoveryModule.json
  artifacts/ModerationModule.json
  artifacts/EconomicModule.json
  artifacts/UsernameModule.json
  artifacts/BadgeModule.json
  artifacts/ReleaseModule.json
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

run_required_file_failure economic-abi-empty abi/EconomicModule.json empty
run_required_file_failure architecture-test-missing test/SuiteArchitecture.t.sol missing
run_required_file_failure repository-artifact-missing artifacts/RepositoryCore.json missing

echo "EVM V2 gate fail-closed regression: pass"
