#!/usr/bin/env bash
# Extract CIDs from V2 EVM RefUpdated events for hot-pin indexer
# Replaces V1 CosmWasm LCD query pattern
set -euo pipefail

EVM_RPC="${EVM_RPC:-https://evm-rpc.injective.network}"
SUITE_DIRECTORY="${SUITE_DIRECTORY:?set SUITE_DIRECTORY}"
NEW_DAYS="${NEW_DAYS:-14}"
HOT_WINDOW_DAYS="${HOT_WINDOW_DAYS:-30}"
HOT_MIN_HITS="${HOT_MIN_HITS:-3}"
IGIT_HOME="${IGIT_HOME:-/var/lib/igit}"
IMPORTANT_CIDS="${IMPORTANT_CIDS:-/etc/igit/important-cids.list}"
DURABLE_CIDS="${DURABLE_CIDS:-/var/lib/igit/durable-cids.list}"
ALLOW_UNPIN="${ALLOW_UNPIN:-false}"
MAX_BLOCKS_PER_QUERY="${MAX_BLOCKS_PER_QUERY:-10000}"

# RefUpdated event signature
REF_UPDATED_TOPIC="0xa7c1db3f7e3d8c2f5b8e9a1c4d6f2e8b3a5c7d9f1e4b6a8c2d5f7e9b1c3d5f7e9"

mkdir -p "$IGIT_HOME"
touch "$IMPORTANT_CIDS" "$DURABLE_CIDS"

# RPC helper
rpc_call() {
    local method="$1"
    local params="$2"
    curl -fsSL -X POST "$EVM_RPC" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$method\",\"params\":$params}" \
        | jq -r '.result // empty'
}

# Get RepositoryCore address from SuiteDirectory
get_core_address() {
    local core_id="0x$(echo -n 'igit.module.repository-core' | sha256sum | cut -d' ' -f1)"
    local calldata="0x40a3d246${core_id:2}"  # moduleAddress(bytes32)
    local result=$(rpc_call "eth_call" "[{\"to\":\"$SUITE_DIRECTORY\",\"data\":\"$calldata\"},\"latest\"]")
    echo "0x${result:26:40}"
}

# Get current block number
get_latest_block() {
    rpc_call "eth_blockNumber" "[]" | xargs printf "%d"
}

# Query RefUpdated logs
query_ref_events() {
    local from="$1"
    local to="$2"
    local address="$3"

    local from_hex=$(printf "0x%x" "$from")
    local to_hex=$(printf "0x%x" "$to")

    local filter=$(jq -nc \
        --arg from "$from_hex" \
        --arg to "$to_hex" \
        --arg address "$address" \
        --arg topic "$REF_UPDATED_TOPIC" \
        '{fromBlock: $from, toBlock: $to, address: $address, topics: [$topic]}')

    rpc_call "eth_getLogs" "[$filter]"
}

# Extract CIDs from event data (simplified - production needs proper ABI decoder)
extract_cids_from_event() {
    local log="$1"
    local data=$(echo "$log" | jq -r '.data')
    local block_number=$(echo "$log" | jq -r '.blockNumber')
    local timestamp=$(get_block_timestamp "$block_number")

    # Extract packUris array from ABI-encoded data
    # This is simplified - use proper ABI decoder in production
    echo "$data" | grep -oE 'ipfs://[a-zA-Z0-9]+' | sed 's|ipfs://||' | while read -r cid; do
        echo -e "$timestamp\t$cid"
    done
}

# Get block timestamp
get_block_timestamp() {
    local block_hex=$(printf "0x%x" "$1")
    local block_data=$(rpc_call "eth_getBlockByNumber" "[\"$block_hex\",false]")
    local timestamp_hex=$(echo "$block_data" | jq -r '.timestamp')
    printf "%d" "$timestamp_hex"
}

# Scan recent events
scan_recent_cids() {
    local core_address=$(get_core_address)
    local latest=$(get_latest_block)
    local cutoff_seconds=$(($(date +%s) - NEW_DAYS * 86400))

    # Estimate blocks (assuming ~2s per block on Injective)
    local approx_blocks=$((NEW_DAYS * 86400 / 2))
    local from_block=$((latest - approx_blocks))
    if [ "$from_block" -lt 0 ]; then
        from_block=0
    fi

    echo "Scanning blocks $from_block to $latest for RefUpdated events..." >&2

    local current="$from_block"
    while [ "$current" -lt "$latest" ]; do
        local next=$((current + MAX_BLOCKS_PER_QUERY))
        if [ "$next" -gt "$latest" ]; then
            next="$latest"
        fi

        local logs=$(query_ref_events "$current" "$next" "$core_address")

        if [ "$logs" != "null" ] && [ "$logs" != "[]" ] && [ -n "$logs" ]; then
            local count=$(echo "$logs" | jq 'length')
            for i in $(seq 0 $((count - 1))); do
                local log=$(echo "$logs" | jq ".[$i]")
                extract_cids_from_event "$log"
            done
        fi

        current="$next"
    done | while IFS=$'\t' read -r timestamp cid; do
        local epoch=$(date -d "$timestamp" +%s 2>/dev/null || echo 0)
        if [ "$epoch" -ge "$cutoff_seconds" ] && [[ "$cid" =~ ^b[a-z2-7]+$ ]]; then
            echo "$cid"
        fi
    done
}

# Calculate hot CIDs based on frequency
hot_cids() {
    local recent_window_seconds=$((HOT_WINDOW_DAYS * 86400))
    local cutoff=$(($(date +%s) - recent_window_seconds))

    while IFS=$'\t' read -r timestamp cid; do
        local epoch=$(date -d "$timestamp" +%s 2>/dev/null || echo 0)
        if [ "$epoch" -ge "$cutoff" ]; then
            echo "$cid"
        fi
    done | sort | uniq -c | while read -r count cid; do
        if [ "$count" -ge "$HOT_MIN_HITS" ]; then
            echo "$cid"
        fi
    done
}

# Main execution
echo "V2 EVM Hot-Pin Indexer"
echo "Suite Directory: $SUITE_DIRECTORY"
echo "EVM RPC: $EVM_RPC"

# Get all important and recent CIDs
{
    cat "$IMPORTANT_CIDS"
    scan_recent_cids
    scan_recent_cids | hot_cids
} | sort -u > "$IGIT_HOME/should-pin.list"

# Current pins
ipfs pin ls --type=recursive | awk '{print $1}' > "$IGIT_HOME/current-pins.list"

# Add missing pins
comm -23 <(sort "$IGIT_HOME/should-pin.list") <(sort "$IGIT_HOME/current-pins.list") | \
    while read -r cid; do
        echo "Pinning $cid"
        ipfs pin add --progress "$cid" || echo "Failed to pin $cid" >&2
    done

# Optional: unpin stale CIDs (only if allowed and CID is in durable archive)
if [ "$ALLOW_UNPIN" = "true" ]; then
    comm -13 <(sort "$IGIT_HOME/should-pin.list") <(sort "$IGIT_HOME/current-pins.list") | \
        while read -r cid; do
            if grep -qxF "$cid" "$DURABLE_CIDS"; then
                echo "Unpinning archived $cid"
                ipfs pin rm "$cid" || echo "Failed to unpin $cid" >&2
            fi
        done
fi

echo "Hot-pin indexer complete"
