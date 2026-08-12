#!/usr/bin/env bash
# Build and test the Solidity EVM V2 core with Foundry.
# This script is intentionally local-only: it never deploys or sends a
# transaction. Use an explicit --required in CI/release gates so a missing
# forge installation cannot be mistaken for a passing V2 check.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CONTRACT_DIR="$ROOT/contracts/evm-v2"
required=0
verbose=0

usage() {
  cat <<'EOF'
usage: evm-v2-check.sh [--required] [--verbose]

Runs the offline Solidity source/ABI check and, when Foundry is installed,
Foundry unit/fuzz/stateful-invariant tests plus representative executable gas
ceilings and a gas report for contracts/evm-v2. Without --required, a missing Foundry
installation is reported as SKIP and exits successfully. With --required, the
same condition is a failure.
EOF
}

while (($#)); do
  case "$1" in
    --required) required=1 ;;
    --verbose) verbose=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

if [[ ! -d "$CONTRACT_DIR" ]]; then
  echo "FAIL: missing EVM V2 contract directory: $CONTRACT_DIR" >&2
  exit 1
fi
if ! command -v node >/dev/null 2>&1; then
  if ((required)); then
    echo "FAIL: node not found; Solidity source/ABI compile check is required" >&2
    exit 1
  fi
  echo "SKIP: node not found; Solidity source/ABI compile check was not run"
elif [[ ! -f "$CONTRACT_DIR/node_modules/solc/solc.js" ]]; then
  if ((required)); then
    echo "FAIL: locked solc is not installed; run npm ci --prefix contracts/evm-v2" >&2
    exit 1
  fi
  echo "SKIP: locked solc is not installed; run npm ci --prefix contracts/evm-v2"
else
  node "$ROOT/scripts/evm-suite-solc-check.mjs"
fi
for file in \
  foundry.toml \
  src/SuiteDirectory.sol \
  src/BootstrapCoordinator.sol \
  src/RepositoryCore.sol \
  src/RecoveryModule.sol \
  src/ModerationModule.sol \
  src/EconomicModule.sol \
  src/UsernameModule.sol \
  src/BadgeModule.sol \
  src/ReleaseModule.sol \
  src/suite/ISuite.sol \
  src/suite/SuiteModule.sol \
  test/SuiteArchitecture.t.sol \
  abi/SuiteDirectory.json \
  abi/BootstrapCoordinator.json \
  abi/RepositoryCore.json \
  abi/RecoveryModule.json \
  abi/ModerationModule.json \
  abi/EconomicModule.json \
  abi/UsernameModule.json \
  abi/BadgeModule.json \
  abi/ReleaseModule.json \
  artifacts/SuiteDirectory.json \
  artifacts/BootstrapCoordinator.json \
  artifacts/RepositoryCore.json \
  artifacts/RecoveryModule.json \
  artifacts/ModerationModule.json \
  artifacts/EconomicModule.json \
  artifacts/UsernameModule.json \
  artifacts/BadgeModule.json \
  artifacts/ReleaseModule.json \
  README.md; do
  if [[ ! -s "$CONTRACT_DIR/$file" ]]; then
    echo "FAIL: missing or empty EVM V2 file: contracts/evm-v2/$file" >&2
    exit 1
  fi
done
if [[ ! -s "$ROOT/scripts/evm-suite-solc-check.mjs" ]]; then
  echo "FAIL: missing or empty EVM suite compiler gate: scripts/evm-suite-solc-check.mjs" >&2
  exit 1
fi
if ! command -v forge >/dev/null 2>&1; then
  if ((required)); then
    echo "FAIL: forge not found; install Foundry before running the V2 gate" >&2
    exit 1
  fi
  echo "SKIP: forge not found; EVM V2 source presence checked only"
  exit 0
fi

echo "== EVM V2 Foundry check =="
echo "contract_dir=$CONTRACT_DIR"
if ((verbose)); then
  forge --version
fi

pushd "$CONTRACT_DIR" >/dev/null
forge build
forge test -vvv
listed_tests="$(forge test --list)"
for required_contract in SuiteArchitectureTest; do
  if ! grep -Fq "$required_contract" <<< "$listed_tests"; then
    echo "FAIL: Foundry did not discover required test contract: $required_contract" >&2
    exit 1
  fi
done
forge test --match-contract '^SuiteArchitectureTest$' -vvv
forge test --gas-report
popd >/dev/null

echo "PASS: immutable EVM suite build, checked ABIs, unit/stateful invariant tests, executable gas ceilings, and gas report"
