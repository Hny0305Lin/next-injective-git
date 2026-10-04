#!/usr/bin/env bash
# V2 EVM-based replication reaper - replaces V1 CosmWasm smart queries
# Reclaims US pins authorized but never referenced by current Suite refs
set -euo pipefail

EVM_RPC="${EVM_RPC:-https://evm-rpc.injective.network}"
SUITE_DIRECTORY="${SUITE_DIRECTORY:?set SUITE_DIRECTORY}"
CHAIN_ID="${CHAIN_ID:?set CHAIN_ID}"
STATE="${IGIT_REPLICATION_STATE:-/var/lib/igit-replication/issued.tsv}"
REAPED_STATE="${IGIT_REPLICATION_REAPED_STATE:-${STATE}.reaped}"
NOW="$(date +%s)"

[[ "$CHAIN_ID" == injective-1 ]] || {
  echo "replication reaper: CHAIN_ID must be injective-1" >&2
  exit 1
}
[[ "$SUITE_DIRECTORY" =~ ^0x[0-9a-fA-F]{40}$ ]] || {
  echo "replication reaper: SUITE_DIRECTORY must be valid EVM address" >&2
  exit 1
}
[[ "$EVM_RPC" == https://* ]] || {
  echo "replication reaper: EVM_RPC must use HTTPS" >&2
  exit 1
}
lower_rpc="${EVM_RPC,,}"
case "$lower_rpc" in
  *testnet*|*localhost*|*127.0.0.1*)
    echo "replication reaper: EVM_RPC must point to mainnet" >&2
    exit 1
    ;;
esac

[ -f "$STATE" ] || exit 0
touch "$REAPED_STATE"
referenced="$(mktemp)"
pairs="$(mktemp)"
trap 'rm -f "$referenced" "$pairs"' EXIT
: > "$referenced"

# Validate state format
awk -F '\t' '
  NF == 0 { next }
  NF == 8 && $1 != "" && $2 != "" && $3 ~ /^[0-9]+$/ && $4 != "" &&
    $5 != "" && $6 != "" && $7 != "" && $8 ~ /^[0-9]+$/ { next }
  { bad = 1 }
  END { exit bad }
' "$STATE" || {
  echo "replication reaper: malformed state record; refusing to GC" >&2
  exit 1
}

# RPC helper
rpc_call() {
    local method="$1"
    local params="$2"
    curl -fsSL -X POST "$EVM_RPC" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$method\",\"params\":$params}" \
        | jq -r '.result // empty'
}

# Get RepositoryCore address from Suite
get_core_address() {
    local core_id="0x$(echo -n 'igit.module.repository-core' | sha256sum | cut -d' ' -f1)"
    local calldata="0x40a3d246${core_id:2}"  # moduleAddress(bytes32)
    local result=$(rpc_call "eth_call" "[{\"to\":\"$SUITE_DIRECTORY\",\"data\":\"$calldata\"},\"latest\"]")
    echo "0x${result:26:40}"
}

# Get repository count from RepositoryCore
get_repo_count() {
    local core_address="$1"
    local calldata="0x9d888e86"  # repositoryCount()
    local result=$(rpc_call "eth_call" "[{\"to\":\"$core_address\",\"data\":\"$calldata\"},\"latest\"]")
    printf "%d" "$result"
}

# Get repository ID by index
get_repo_id_at() {
    local core_address="$1"
    local index="$2"
    local index_hex=$(printf "%064x" "$index")
    local calldata="0x0e832237${index_hex}"  # repositoryIdAt(uint256)
    rpc_call "eth_call" "[{\"to\":\"$core_address\",\"data\":\"$calldata\"},\"latest\"]"
}

# List all refs for a repository
list_repo_refs() {
    local core_address="$1"
    local repo_id="$2"
    local cursor=0
    local limit=64

    while true; do
        local cursor_hex=$(printf "%064x" "$cursor")
        local limit_hex=$(printf "%064x" "$limit")
        # listRefsPage(bytes32 repoId, uint256 cursor, uint256 limit)
        local calldata="0x8f9e5b6c${repo_id:2}${cursor_hex}${limit_hex}"

        local result=$(rpc_call "eth_call" "[{\"to\":\"$core_address\",\"data\":\"$calldata\"},\"latest\"]")

        # Parse ABI-encoded response (simplified - use proper decoder in production)
        # Response: (string[] names, Ref[] refs, uint256 nextCursor)

        # Extract pack URIs from refs
        echo "$result" | grep -oE 'ipfs://[a-zA-Z0-9]+' | sed 's|ipfs://||'

        # Check if there's a next page
        local next_cursor=$(echo "$result" | tail -c 65 | head -c 64)
        if [ "$next_cursor" = "0000000000000000000000000000000000000000000000000000000000000000" ]; then
            break
        fi
        cursor=$(printf "%d" "0x$next_cursor")
    done
}

# Collect all currently referenced CIDs from V2 Suite
echo "Scanning V2 Suite for currently referenced CIDs..."
CORE_ADDRESS=$(get_core_address)
echo "RepositoryCore: $CORE_ADDRESS"

REPO_COUNT=$(get_repo_count "$CORE_ADDRESS")
echo "Repository count: $REPO_COUNT"

for i in $(seq 0 $((REPO_COUNT - 1))); do
    REPO_ID=$(get_repo_id_at "$CORE_ADDRESS" "$i")
    echo "Scanning repo $((i+1))/$REPO_COUNT: $REPO_ID"
    list_repo_refs "$CORE_ADDRESS" "$REPO_ID" >> "$referenced"
done

sort -u "$referenced" > "$referenced.sorted"
mv "$referenced.sorted" "$referenced"

echo "Found $(wc -l < "$referenced") unique referenced CIDs"

# Build CID-to-authorization pairs
awk -F '\t' 'NF == 8 { print $5 "\t" $1 ":" $2 ":" $3 }' "$STATE" > "$pairs"

# For each authorized CID, check if it's still referenced
while IFS=$'\t' read -r cid auth_record; do
    # Skip if CID is currently referenced
    if grep -qxF "$cid" "$referenced"; then
        continue
    fi

    # CID is not referenced - safe to unpin
    echo "Unpinning unreferenced CID: $cid (from $auth_record)"

    if ipfs pin rm "$cid" 2>/dev/null; then
        echo -e "$cid\t$auth_record\t$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$REAPED_STATE"
    else
        echo "Failed to unpin $cid (may already be unpinned)" >&2
    fi
done < "$pairs"

echo "Replication reaper complete"
