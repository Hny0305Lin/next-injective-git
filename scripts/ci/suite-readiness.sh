#!/usr/bin/env bash
# Source and test gate for the immutable Injective EVM Suite.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ ! -f "$ROOT/CLAUDE.md" && ! -f "$ROOT/README.md" ]]; then
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fi
if [[ ! -f "$ROOT/CLAUDE.md" ]]; then
  ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo "$ROOT")"
fi
mode=source

usage() {
  echo "usage: suite-readiness.sh [--source-only|--required]" >&2
}

case "${1:-}" in
  ""|--source-only) mode=source ;;
  --required) mode=required ;;
  *) usage; exit 2 ;;
esac
[[ $# -le 1 ]] || { usage; exit 2; }

required_files=(
  contracts/evm-v2/src/SuiteDirectory.sol
  contracts/evm-v2/src/BootstrapCoordinator.sol
  contracts/evm-v2/src/RepositoryCore.sol
  contracts/evm-v2/src/RecoveryModule.sol
  contracts/evm-v2/src/ModerationModule.sol
  contracts/evm-v2/src/EconomicModule.sol
  contracts/evm-v2/src/UsernameModule.sol
  contracts/evm-v2/src/BadgeModule.sol
  contracts/evm-v2/src/ReleaseModule.sol
  scripts/ci/evm-suite-solc-check.mjs
  cli/internal/chain/suite_directory.go
  cli/internal/chain/evm_suite_registry.go
  cli/internal/chain/evm_transactor.go
  cli/internal/archivev1/client.go
  cli/internal/archivev1/inventory.go
  archive/cosmwasm-v1/README.md
  archive/cosmwasm-v1/contracts/repo-registry/Cargo.toml
)

for file in "${required_files[@]}"; do
  [[ -s "$ROOT/$file" ]] || { echo "FAIL: missing or empty Suite source: $file" >&2; exit 1; }
done

for contract in SuiteDirectory BootstrapCoordinator RepositoryCore RecoveryModule ModerationModule EconomicModule UsernameModule BadgeModule ReleaseModule; do
  [[ -s "$ROOT/contracts/evm-v2/abi/$contract.json" ]] || { echo "FAIL: missing ABI for $contract" >&2; exit 1; }
  [[ -s "$ROOT/contracts/evm-v2/artifacts/$contract.json" ]] || { echo "FAIL: missing compiler artifact for $contract" >&2; exit 1; }
done

if find "$ROOT/contracts/evm-v2" -type f -name 'RepoRegistryV2*' -print -quit | grep -q .; then
  echo "FAIL: undeployed RepoRegistryV2 alpha files remain in the Suite package" >&2
  exit 1
fi

[[ ! -e "$ROOT/contracts/repo-registry" ]] || {
  echo "FAIL: CosmWasm V1 contract must live only under archive/cosmwasm-v1" >&2
  exit 1
}

if grep -R -n -E 'cosmwasm/wasm/v1|"tx"[[:space:]]*,[[:space:]]*"wasm"[[:space:]]*,[[:space:]]*"execute"|CosmWasmRegistryV1|BackendCosmWasm' \
  "$ROOT/cli/internal/chain" "$ROOT/cli/cmd" --include='*.go'; then
  echo "FAIL: ordinary Go runtime contains a CosmWasm V1 path" >&2
  exit 1
fi

web_v1_archive="$ROOT/web/src/lib/cosmwasm-v1.ts"
mapfile -t web_runtime_sources < <(
  find "$ROOT/web/src" -type f \( -name '*.ts' -o -name '*.tsx' \) ! -path "$web_v1_archive"
)
if grep -R -n -E 'legacyClient|cosmwasm/wasm/v1|MsgExecuteContract|SigningCosmWasmClient|contractVersion[[:space:]]*:[[:space:]]*"(auto|v1|v2)"' \
  "${web_runtime_sources[@]}"; then
  echo "FAIL: ordinary Web runtime contains a V1 fallback or pre-Suite profile" >&2
  exit 1
fi

if [[ ! -f "$web_v1_archive" ]] || \
   ! grep -q 'method: "GET"' "$web_v1_archive" || \
   grep -n -E 'MsgExecuteContract|SigningCosmWasmClient|eth_sendTransaction|wallet_|\.execute\s*\(|method:[[:space:]]*"(POST|PUT|PATCH|DELETE)"' "$web_v1_archive"; then
  echo "FAIL: Web V1 archive adapter must remain an isolated GET-only read path" >&2
  exit 1
fi

if grep -n -E '"@(cosmjs|injectivelabs)/(cosmwasm-stargate|sdk-ts|networks|ts-types)"' \
  "$ROOT/web/package.json"; then
  echo "FAIL: Web dependencies contain a Cosmos or CosmWasm transaction stack" >&2
  exit 1
fi

web_transport="$ROOT/web/src/lib/transport.ts"
grep -q 'createWalletClient' "$web_transport" || { echo "FAIL: Web wallet broadcasts must use viem" >&2; exit 1; }
grep -q 'estimateGas' "$web_transport" || { echo "FAIL: Web wallet broadcasts must estimate gas explicitly" >&2; exit 1; }
grep -q 'type: "legacy"' "$web_transport" || { echo "FAIL: Web wallet broadcasts must be legacy type 0x0" >&2; exit 1; }
grep -q 'MIN_INJECTIVE_GAS_PRICE = 160_000_000n' "$web_transport" || {
  echo "FAIL: Web wallet gasPrice must use the Injective minimum" >&2
  exit 1
}
grep -q 'transaction receipt status' "$web_transport" || {
  echo "FAIL: Web wallet broadcasts must validate receipt status" >&2
  exit 1
}

if grep -n -E 'repo-registry\.wasm|contracts/repo-registry|cargo build.*wasm32' \
  "$ROOT/.github/workflows/ci.yml" "$ROOT/.github/workflows/release.yml"; then
  echo "FAIL: default CI/release still builds or publishes CosmWasm V1" >&2
  exit 1
fi

if grep -R -n -E 'RepoRegistryV2|igit-migrate-v(1|2)|igit upgrade|contracts/repo-registry' \
  "$ROOT/README.md" "$ROOT/CLAUDE.md" "$ROOT/docs" --include='*.md'; then
  echo "FAIL: ordinary documentation still advertises a removed V1 or alpha path" >&2
  exit 1
fi

for file in migration/migration-cutover-readiness.sh migration/migration-cutover-readiness.ps1 deploy/validate-suite-cutover.mjs; do
  if [[ -s "$ROOT/scripts/$file" || -s "$ROOT/scripts/$(basename "$file")" ]]; then continue; fi
  echo "FAIL: missing Suite cutover gate: scripts/$file (or shim)" >&2; exit 1
done

echo "PASS: immutable Suite source, checked artifacts, pure-EVM runtime, and V1 archive boundaries"

[[ "$mode" == required ]] || exit 0

command -v go >/dev/null 2>&1 || { echo "FAIL: go is required" >&2; exit 1; }
command -v node >/dev/null 2>&1 || { echo "FAIL: node is required" >&2; exit 1; }
command -v npm >/dev/null 2>&1 || { echo "FAIL: npm is required" >&2; exit 1; }

(cd "$ROOT/cli" && go vet ./... && go test ./...)
(cd "$ROOT/web" && npm run test:api && npm run typecheck && npm run build)
node "$ROOT/scripts/deploy/identity-readiness.mjs"
bash "$ROOT/scripts/ci/evm-v2-check.sh" --required

echo "PASS: immutable Suite required test gate"
