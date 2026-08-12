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
  node "$ROOT/scripts/evm-v2-solc-check.mjs"
fi
for file in \
  foundry.toml \
  src/RepoRegistryV2.sol \
  src/RepoRegistryV2ImportController.sol \
  src/RepoRegistryV2BadgeModule.sol \
  src/RepoRegistryV2EconomicModule.sol \
  src/RepoRegistryV2ModerationModule.sol \
  test/RepoRegistryV2.t.sol \
  test/RepoRegistryV2ImportController.t.sol \
  test/RepoRegistryV2BadgeModule.t.sol \
  test/RepoRegistryV2EconomicModule.t.sol \
  test/RepoRegistryV2ModerationModule.t.sol \
  test/RepoRegistryV2Invariant.t.sol \
  test/RepoRegistryV2Gas.t.sol \
  abi/RepoRegistryV2.json \
  abi/RepoRegistryV2ImportController.json \
  abi/RepoRegistryV2BadgeModule.json \
  abi/RepoRegistryV2EconomicModule.json \
  abi/RepoRegistryV2ModerationModule.json \
  README.md; do
  if [[ ! -s "$CONTRACT_DIR/$file" ]]; then
    echo "FAIL: missing or empty EVM V2 file: contracts/evm-v2/$file" >&2
    exit 1
  fi
done
if [[ ! -s "$ROOT/scripts/evm-v2-foundry-abi-check.mjs" ]]; then
  echo "FAIL: missing or empty EVM V2 ABI comparison helper: scripts/evm-v2-foundry-abi-check.mjs" >&2
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
for required_contract in RepoRegistryV2InvariantTest RepoRegistryV2GasTest; do
  if ! grep -Fq "$required_contract" <<< "$listed_tests"; then
    echo "FAIL: Foundry did not discover required test contract: $required_contract" >&2
    exit 1
  fi
done
forge test --match-contract '^RepoRegistryV2InvariantTest$' -vvv
forge test --match-contract '^RepoRegistryV2GasTest$' -vvv
forge test --gas-report
node "$ROOT/scripts/evm-v2-foundry-abi-check.mjs" "$CONTRACT_DIR"
popd >/dev/null

echo "PASS: EVM V2 build, checked ABIs, unit/fuzz/stateful invariant tests, executable gas ceilings, and gas report"
