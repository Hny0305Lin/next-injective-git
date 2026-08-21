#!/usr/bin/env bash
# Keep recent, popular, and explicitly important CIDs pinned on the HK Kubo.
# Pruning is opt-in and only accepts CIDs confirmed durable by the US archive.
set -euo pipefail

LCD="${LCD:-https://testnet.sentry.lcd.injective.network:443}"
CONTRACT="${CONTRACT:-inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh}"
NEW_DAYS="${NEW_DAYS:-14}"
HOT_WINDOW_DAYS="${HOT_WINDOW_DAYS:-30}"
HOT_MIN_HITS="${HOT_MIN_HITS:-3}"
IGIT_HOME="${IGIT_HOME:-/var/lib/igit}"
IMPORTANT_CIDS="${IMPORTANT_CIDS:-/etc/igit/important-cids.list}"
DURABLE_CIDS="${DURABLE_CIDS:-/var/lib/igit/durable-cids.list}"
ALLOW_UNPIN="${ALLOW_UNPIN:-false}"

mkdir -p "$IGIT_HOME"
touch "$IMPORTANT_CIDS" "$DURABLE_CIDS"

scan_recent_cids() {
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
        printf '%s' "$response" | jq -r '.tx_responses[]? as $tx | $tx.timestamp as $time | $tx.tx.body.messages[]? | (.msg.update_ref.pack_uris // [])[] | [$time, sub("^ipfs://"; "")] | @tsv'
        next_key=$(printf '%s' "$response" | jq -r '.pagination.next_key // empty')
        [ -n "$next_key" ] || break
    done
}

recent_cids() {
    local cutoff timestamp cid epoch
    cutoff=$(date -u -d "${NEW_DAYS} days ago" +%s)
    while IFS=$'\t' read -r timestamp cid; do
        epoch=$(date -u -d "$timestamp" +%s 2>/dev/null || echo 0)
        if [ "$epoch" -ge "$cutoff" ] && [[ "$cid" =~ ^b[a-z2-7]+$ ]]; then
            printf '%s\n' "$cid"
        fi
    done
}

popular_cids() {
    find /var/log/nginx -maxdepth 1 -type f -name 'access.log*' -mtime "-$HOT_WINDOW_DAYS" -print0 \
        | xargs -0r zcat -f 2>/dev/null \
        | awk '$7 ~ /^\/ipfs\/b/ && $9 ~ /^2/ { sub("^/ipfs/", "", $7); print $7 }' \
        | sort | uniq -c | awk -v min="$HOT_MIN_HITS" '$1 >= min { print $2 }'
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
scan_recent_cids | recent_cids > "$tmp/recent"
popular_cids > "$tmp/popular"
grep -E '^b[a-z2-7]+$' "$IMPORTANT_CIDS" > "$tmp/important" || true
cat "$tmp/recent" "$tmp/popular" "$tmp/important" | sort -u > "$tmp/desired"

while read -r cid; do
    [ -n "$cid" ] || continue
    if ! ipfs pin ls --type=recursive "$cid" >/dev/null 2>&1; then
        echo "[pin] $cid"
        ipfs pin add --timeout=120s "$cid" >/dev/null
    fi
done < "$tmp/desired"

ipfs pin ls --type=recursive | awk '{print $1}' | sort -u > "$tmp/current"
comm -23 "$tmp/current" "$tmp/desired" > "$tmp/candidates"

if [ "$ALLOW_UNPIN" != true ]; then
    count=$(wc -l < "$tmp/candidates")
    echo "[dry-run] $count cold CIDs would be eligible for unpin after durable confirmation"
    exit 0
fi

while read -r cid; do
    [ -n "$cid" ] || continue
    if grep -qxF "$cid" "$DURABLE_CIDS"; then
        echo "[unpin] $cid"
        ipfs pin rm "$cid" >/dev/null
    else
        echo "[skip] not durable on US + Fil.one: $cid"
    fi
done < "$tmp/candidates"
