#!/usr/bin/env bash
# V2 EVM-based archive indexer - replaces V1 CosmWasm LCD queries
# Replicates every historical igit pack CID to Kubo and S3 CAR archive
set -euo pipefail

EVM_RPC="${EVM_RPC:-https://evm-rpc.injective.network}"
SUITE_DIRECTORY="${SUITE_DIRECTORY:?set SUITE_DIRECTORY}"
IGIT_HOME="${IGIT_HOME:-/var/lib/igit-archive}"
ARCHIVE_ENV="${ARCHIVE_ENV:-/etc/igit/filone.env}"
SOURCE_PEER="${SOURCE_PEER:-/ip4/45.202.249.80/tcp/4001/p2p/12D3KooWRfRoRqEyC4Qsb4ow2yfGsSAAymTFSxj6vr2SYQnxk55W}"
PIN_TIMEOUT="${PIN_TIMEOUT:-10m}"
MAX_BLOCKS_PER_QUERY="${MAX_BLOCKS_PER_QUERY:-10000}"

PINNED_STATE="$IGIT_HOME/pinned.list"
ARCHIVED_STATE="$IGIT_HOME/archived.tsv"
CHECKPOINT_FILE="$IGIT_HOME/archive-checkpoint.json"

# RefUpdated event signature
REF_UPDATED_TOPIC="0xa7c1db3f7e3d8c2f5b8e9a1c4d6f2e8b3a5c7d9f1e4b6a8c2d5f7e9b1c3d5f7e9"

usage() {
    echo "usage: evm-archive-indexer.sh [--pin-only] [--once] [--from-block BLOCK]" >&2
    exit 2
}

mode="archive"
once=false
from_block=""

while [ $# -gt 0 ]; do
    case "$1" in
        --pin-only) mode="pin-only"; shift ;;
        --once) once=true; shift ;;
        --from-block) from_block="$2"; shift 2 ;;
        *) usage ;;
    esac
done

mkdir -p "$IGIT_HOME"
touch "$PINNED_STATE" "$ARCHIVED_STATE"

if [ "$mode" = "archive" ]; then
    if [ -r "$ARCHIVE_ENV" ]; then
        . "$ARCHIVE_ENV"
    fi
    : "${FILONE_ACCESS_KEY:?missing FILONE_ACCESS_KEY in $ARCHIVE_ENV}"
    : "${FILONE_SECRET_KEY:?missing FILONE_SECRET_KEY in $ARCHIVE_ENV}"
    : "${FILONE_BUCKET:?missing FILONE_BUCKET in $ARCHIVE_ENV}"
    export AWS_ACCESS_KEY_ID="$FILONE_ACCESS_KEY"
    export AWS_SECRET_ACCESS_KEY="$FILONE_SECRET_KEY"
    export AWS_DEFAULT_REGION="${FILONE_REGION:-us-east-1}"
    export AWS_EC2_METADATA_DISABLED=true
    FILONE_ENDPOINT="${FILONE_ENDPOINT:-https://us-east-1.s3.fil.one}"
fi

# RPC helper
rpc_call() {
    local method="$1"
    local params="$2"
    curl -fsSL -X POST "$EVM_RPC" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$method\",\"params\":$params}" \
        | jq -r '.result // empty'
}

# Get RepositoryCore address
get_core_address() {
    local core_id="0x$(echo -n 'igit.module.repository-core' | sha256sum | cut -d' ' -f1)"
    local calldata="0x40a3d246${core_id:2}"
    local result=$(rpc_call "eth_call" "[{\"to\":\"$SUITE_DIRECTORY\",\"data\":\"$calldata\"},\"latest\"]")
    echo "0x${result:26:40}"
}

get_latest_block() {
    rpc_call "eth_blockNumber" "[]" | xargs printf "%d"
}

# Load checkpoint
load_checkpoint() {
    if [ -f "$CHECKPOINT_FILE" ]; then
        jq -r '.last_indexed_block // "0"' "$CHECKPOINT_FILE"
    elif [ -n "$from_block" ]; then
        echo "$from_block"
    else
        echo "0"
    fi
}

# Save checkpoint
save_checkpoint() {
    local block="$1"
    jq -n --arg block "$block" --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{last_indexed_block: $block, updated_at: $timestamp}' > "$CHECKPOINT_FILE.tmp"
    mv "$CHECKPOINT_FILE.tmp" "$CHECKPOINT_FILE"
}

# Query RefUpdated events
query_ref_events() {
    local from=$(printf "0x%x" "$1")
    local to=$(printf "0x%x" "$2")
    local address="$3"

    local filter=$(jq -nc \
        --arg from "$from" \
        --arg to "$to" \
        --arg address "$address" \
        --arg topic "$REF_UPDATED_TOPIC" \
        '{fromBlock: $from, toBlock: $to, address: $address, topics: [$topic]}')

    rpc_call "eth_getLogs" "[$filter]"
}

# Extract CIDs from event (simplified - use proper ABI decoder in production)
extract_cids() {
    local log="$1"
    local data=$(echo "$log" | jq -r '.data')
    # Parse packUris array from ABI-encoded data
    echo "$data" | grep -oE 'ipfs://[a-zA-Z0-9]+' | sed 's|ipfs://||'
}

# Process block range
process_range() {
    local from="$1"
    local to="$2"
    local core_address="$3"

    echo "Processing blocks $from to $to..." >&2

    local logs=$(query_ref_events "$from" "$to" "$core_address")

    if [ "$logs" = "null" ] || [ "$logs" = "[]" ] || [ -z "$logs" ]; then
        return 0
    fi

    local count=$(echo "$logs" | jq 'length')
    for i in $(seq 0 $((count - 1))); do
        local log=$(echo "$logs" | jq ".[$i]")
        extract_cids "$log" | while read -r cid; do
            if [[ "$cid" =~ ^b[a-z2-7]+$ ]] && ! grep -qxF "$cid" "$PINNED_STATE"; then
                echo "New CID: $cid"

                # Pin to local Kubo
                if timeout "$PIN_TIMEOUT" ipfs pin add --progress "$cid" 2>&1; then
                    echo "$cid" >> "$PINNED_STATE"

                    # Archive to S3 if not in pin-only mode
                    if [ "$mode" = "archive" ] && ! grep -qxF "$cid" "$ARCHIVED_STATE"; then
                        echo "Archiving $cid to S3..."
                        if ipfs dag export "$cid" | \
                           aws s3 cp --endpoint-url="$FILONE_ENDPOINT" \
                               - "s3://$FILONE_BUCKET/igit-packs/$cid.car"; then
                            echo -e "$cid\t$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$ARCHIVED_STATE"
                        else
                            echo "Failed to archive $cid" >&2
                        fi
                    fi
                else
                    echo "Failed to pin $cid" >&2
                fi
            fi
        done
    done
}

# Main loop
main() {
    echo "V2 EVM Archive Indexer"
    echo "Suite Directory: $SUITE_DIRECTORY"
    echo "EVM RPC: $EVM_RPC"
    echo "Mode: $mode"

    local core_address=$(get_core_address)
    echo "RepositoryCore address: $core_address"

    local checkpoint=$(load_checkpoint)
    echo "Starting from block: $checkpoint"

    while true; do
        local latest=$(get_latest_block)

        if [ "$checkpoint" -ge "$latest" ]; then
            if [ "$once" = true ]; then
                echo "Reached latest block $latest, exiting"
                break
            fi
            echo "Caught up to block $latest, waiting..."
            sleep 30
            continue
        fi

        local next_end=$((checkpoint + MAX_BLOCKS_PER_QUERY))
        if [ "$next_end" -gt "$latest" ]; then
            next_end="$latest"
        fi

        process_range "$checkpoint" "$next_end" "$core_address"
        checkpoint="$next_end"
        save_checkpoint "$checkpoint"

        if [ "$once" = true ]; then
            echo "Single run complete at block $checkpoint"
            break
        fi
    done
}

main
