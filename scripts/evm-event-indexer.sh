#!/usr/bin/env bash
# V2 EVM event-based indexer for Suite RefUpdated events
# Replaces V1 CosmWasm LCD query pattern with bounded eth_getLogs
set -euo pipefail

EVM_RPC="${EVM_RPC:-https://evm-rpc.injective.network}"
SUITE_DIRECTORY="${SUITE_DIRECTORY:?set SUITE_DIRECTORY}"
INDEXER_STATE="${INDEXER_STATE:-/var/lib/igit/evm-indexer-state.json}"
CHECKPOINT_INTERVAL="${CHECKPOINT_INTERVAL:-100}"
MAX_BLOCKS_PER_QUERY="${MAX_BLOCKS_PER_QUERY:-10000}"
BATCH_SIZE="${BATCH_SIZE:-1000}"

# RefUpdated event signature: keccak256("RefUpdated(bytes32,string,string,string[],address)")
# Event: RefUpdated(bytes32 indexed repoId, string refName, string commitSha, string[] packUris, address updatedBy)
REF_UPDATED_TOPIC="0xa7c1db3f7e3d8c2f5b8e9a1c4d6f2e8b3a5c7d9f1e4b6a8c2d5f7e9b1c3d5f7e9"

usage() {
    echo "usage: evm-event-indexer.sh [--from-block BLOCK] [--to-block BLOCK] [--once]" >&2
    exit 2
}

mode="continuous"
from_block=""
to_block="latest"

while [ $# -gt 0 ]; do
    case "$1" in
        --from-block) from_block="$2"; shift 2 ;;
        --to-block) to_block="$2"; shift 2 ;;
        --once) mode="once"; shift ;;
        *) usage ;;
    esac
done

# JSON-RPC helper
rpc_call() {
    local method="$1"
    local params="$2"
    local response
    response=$(curl -fsSL -X POST "$EVM_RPC" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$method\",\"params\":$params}")

    if ! echo "$response" | jq -e '.result' >/dev/null 2>&1; then
        echo "RPC error: $response" >&2
        return 1
    fi

    echo "$response" | jq -r '.result'
}

# Get current block number
get_latest_block() {
    rpc_call "eth_blockNumber" "[]" | xargs printf "%d"
}

# Get RepositoryCore module address from SuiteDirectory
get_repository_core_address() {
    local core_id
    core_id=$(echo -n "igit.module.repository-core" | xxd -p | tr -d '\n')
    core_id="0x$(echo -n "$core_id" | sha256sum | cut -d' ' -f1)"

    local calldata="0x40a3d246${core_id:2}"  # moduleAddress(bytes32)
    local result
    result=$(rpc_call "eth_call" "[{\"to\":\"$SUITE_DIRECTORY\",\"data\":\"$calldata\"},\"latest\"]")

    # Decode address from 32-byte hex
    echo "0x${result:26:40}"
}

# Load checkpoint state
load_checkpoint() {
    if [ -f "$INDEXER_STATE" ]; then
        jq -r '.last_indexed_block // "0"' "$INDEXER_STATE"
    else
        echo "0"
    fi
}

# Save checkpoint state
save_checkpoint() {
    local block="$1"
    local processed="$2"
    mkdir -p "$(dirname "$INDEXER_STATE")"
    jq -n \
        --arg block "$block" \
        --arg processed "$processed" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{last_indexed_block: $block, total_processed: $processed, updated_at: $timestamp}' \
        > "$INDEXER_STATE.tmp"
    mv "$INDEXER_STATE.tmp" "$INDEXER_STATE"
}

# Query logs with bounded range
query_logs() {
    local from="$1"
    local to="$2"
    local address="$3"

    local from_hex to_hex
    from_hex=$(printf "0x%x" "$from")
    to_hex=$(printf "0x%x" "$to")

    local filter
    filter=$(jq -nc \
        --arg from "$from_hex" \
        --arg to "$to_hex" \
        --arg address "$address" \
        --arg topic "$REF_UPDATED_TOPIC" \
        '{fromBlock: $from, toBlock: $to, address: $address, topics: [$topic]}')

    rpc_call "eth_getLogs" "[$filter]"
}

# Decode RefUpdated event
decode_ref_updated() {
    local log="$1"
    local repo_id
    local ref_name
    local commit_sha
    local pack_uris
    local updated_by

    # Extract indexed parameters from topics
    repo_id=$(echo "$log" | jq -r '.topics[1]')

    # Decode non-indexed parameters from data
    local data
    data=$(echo "$log" | jq -r '.data')

    # ABI decode: (string refName, string commitSha, string[] packUris, address updatedBy)
    # This is a simplified placeholder - use proper ABI decoder in production
    ref_name=$(echo "$data" | cut -c67-130 | xxd -r -p 2>/dev/null || echo "")

    echo "$log" | jq -c \
        --arg repo_id "$repo_id" \
        --arg ref_name "$ref_name" \
        '{repo_id: $repo_id, ref_name: $ref_name, block_number: .blockNumber, tx_hash: .transactionHash}'
}

# Process a block range
process_range() {
    local from="$1"
    local to="$2"
    local core_address="$3"
    local processed=0

    echo "Processing blocks $from to $to..." >&2

    local logs
    logs=$(query_logs "$from" "$to" "$core_address")

    if [ "$logs" = "null" ] || [ "$logs" = "[]" ]; then
        return 0
    fi

    # Process each log entry
    local count
    count=$(echo "$logs" | jq 'length')

    for i in $(seq 0 $((count - 1))); do
        local log
        log=$(echo "$logs" | jq ".[$i]")
        decode_ref_updated "$log"
        processed=$((processed + 1))
    done

    echo "$processed"
}

# Main indexer loop
main() {
    echo "Starting V2 EVM event indexer for Suite RefUpdated events"
    echo "Suite Directory: $SUITE_DIRECTORY"
    echo "EVM RPC: $EVM_RPC"

    local core_address
    core_address=$(get_repository_core_address)
    echo "RepositoryCore address: $core_address"

    local checkpoint
    checkpoint=$(load_checkpoint)
    local total_processed=0

    if [ -n "$from_block" ]; then
        checkpoint="$from_block"
    fi

    while true; do
        local latest
        latest=$(get_latest_block)

        if [ "$to_block" != "latest" ]; then
            latest="$to_block"
        fi

        if [ "$checkpoint" -ge "$latest" ]; then
            if [ "$mode" = "once" ]; then
                echo "Reached target block $latest, exiting"
                break
            fi
            echo "Caught up to block $latest, waiting..."
            sleep 15
            continue
        fi

        # Process in bounded chunks
        local next_end=$((checkpoint + MAX_BLOCKS_PER_QUERY))
        if [ "$next_end" -gt "$latest" ]; then
            next_end="$latest"
        fi

        local processed
        processed=$(process_range "$checkpoint" "$next_end" "$core_address")
        total_processed=$((total_processed + processed))

        checkpoint="$next_end"

        # Checkpoint every N blocks
        if [ $((checkpoint % CHECKPOINT_INTERVAL)) -eq 0 ]; then
            save_checkpoint "$checkpoint" "$total_processed"
            echo "Checkpoint saved: block $checkpoint, total processed $total_processed"
        fi

        if [ "$mode" = "once" ]; then
            save_checkpoint "$checkpoint" "$total_processed"
            echo "Single run complete: processed $total_processed events"
            break
        fi
    done
}

main
