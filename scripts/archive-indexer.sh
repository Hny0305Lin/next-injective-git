#!/usr/bin/env bash
# Replicate every historical igit pack CID to this Kubo node and a private S3
# CAR archive. A CID is only recorded as archived after the remote checksum
# metadata has been verified.
set -euo pipefail

LCD="${LCD:-https://testnet.sentry.lcd.injective.network:443}"
CONTRACT="${CONTRACT:-inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh}"
IGIT_HOME="${IGIT_HOME:-/var/lib/igit-archive}"
ARCHIVE_ENV="${ARCHIVE_ENV:-/etc/igit/filone.env}"
SOURCE_PEER="${SOURCE_PEER:-/ip4/45.202.249.80/tcp/4001/p2p/12D3KooWRfRoRqEyC4Qsb4ow2yfGsSAAymTFSxj6vr2SYQnxk55W}"
PIN_TIMEOUT="${PIN_TIMEOUT:-10m}"

PINNED_STATE="$IGIT_HOME/pinned.list"
ARCHIVED_STATE="$IGIT_HOME/archived.tsv"

usage() {
    echo "usage: archive-indexer.sh [--pin-only] [--once]" >&2
    exit 2
}

mode="archive"
case "${1:-}" in
    "") ;;
    --pin-only) mode="pin-only" ;;
    --once) ;;
    *) usage ;;
esac

if [ "${2:-}" != "" ]; then
    usage
fi

mkdir -p "$IGIT_HOME"
touch "$PINNED_STATE" "$ARCHIVED_STATE"

if [ "$mode" = "archive" ]; then
    # Manual invocations may read the env file; systemd injects it for ipfs.
    if [ -r "$ARCHIVE_ENV" ]; then
        # shellcheck disable=SC1090
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

scan_cids() {
    local next_key="" response
    while :; do
        if [ -n "$next_key" ]; then
            response=$(curl -fsS -G "${LCD}/cosmos/tx/v1beta1/txs" \
                --data-urlencode "query=wasm._contract_address='${CONTRACT}' AND wasm.action='update_ref'" \
                --data-urlencode "pagination.limit=100" \
                --data-urlencode "pagination.key=${next_key}" \
                --data-urlencode "order_by=ORDER_BY_DESC")
        else
            response=$(curl -fsS -G "${LCD}/cosmos/tx/v1beta1/txs" \
                --data-urlencode "query=wasm._contract_address='${CONTRACT}' AND wasm.action='update_ref'" \
                --data-urlencode "pagination.limit=100" \
                --data-urlencode "order_by=ORDER_BY_DESC")
        fi
        printf '%s' "$response" | jq -r '.tx_responses[]?.tx.body.messages[]? | (.msg.update_ref.pack_uris // [])[]' \
            | sed 's|^ipfs://||'
        next_key=$(printf '%s' "$response" | jq -r '.pagination.next_key // empty')
        [ -n "$next_key" ] || break
    done
}

is_pinned() {
    ipfs pin ls --type=recursive "$1" >/dev/null 2>&1
}

is_archived() {
    local cid="$1" expected_sha
    expected_sha=$(awk -F '\t' -v cid="$cid" '$1 == cid { print $3; exit }' "$ARCHIVED_STATE")
    [ -n "$expected_sha" ] || return 1
    [ "$(aws s3api head-object --endpoint-url "$FILONE_ENDPOINT" --bucket "$FILONE_BUCKET" \
        --key "cars/v1/${cid:0:2}/${cid}.car" --query 'Metadata.sha256' --output text 2>/dev/null)" = "$expected_sha" ]
}

archive_one() {
    local cid="$1" car sha256 key remote_sha version_id
    if is_archived "$cid"; then
        return 0
    fi
    car=$(mktemp --suffix=.car)
    trap 'rm -f "$car"' RETURN
    ipfs dag export "$cid" > "$car"
    sha256=$(sha256sum "$car" | awk '{print $1}')
    key="cars/v1/${cid:0:2}/${cid}.car"
    version_id=$(aws s3api put-object --endpoint-url "$FILONE_ENDPOINT" --bucket "$FILONE_BUCKET" \
        --key "$key" --body "$car" --metadata "cid=$cid,sha256=$sha256" \
        --query 'VersionId' --output text)
    remote_sha=$(aws s3api head-object --endpoint-url "$FILONE_ENDPOINT" --bucket "$FILONE_BUCKET" \
        --key "$key" --query 'Metadata.sha256' --output text)
    [ "$remote_sha" = "$sha256" ] || { echo "archive checksum mismatch: $cid" >&2; return 1; }
    printf '%s\t%s\t%s\t%s\t%s\n' "$cid" "$key" "$sha256" "$version_id" "$(date -u +%FT%TZ)" >> "$ARCHIVED_STATE"
    trap - RETURN
    rm -f "$car"
}

connected=false
pin_one() {
    local cid="$1"
    if [ "$connected" = false ] && [ -n "$SOURCE_PEER" ]; then
        ipfs swarm connect "$SOURCE_PEER" >/dev/null 2>&1 || true
        connected=true
    fi
    if ! is_pinned "$cid"; then
        echo "[pin] $cid"
        ipfs pin add --timeout="$PIN_TIMEOUT" "$cid" >/dev/null
    fi
    grep -qxF "$cid" "$PINNED_STATE" 2>/dev/null || printf '%s\n' "$cid" >> "$PINNED_STATE"
    if [ "$mode" = "archive" ]; then
        echo "[archive] $cid"
        archive_one "$cid"
    fi
}

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
scan_cids | grep -E '^b[a-z2-7]+$' | sort -u > "$tmp"
while read -r cid; do
    [ -n "$cid" ] && pin_one "$cid"
done < "$tmp"
