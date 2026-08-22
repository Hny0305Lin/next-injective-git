#!/usr/bin/env bash
# pin-indexer: poll the repo-registry contract for update_ref txs, extract
# pack_uris, then pin each CID locally and replicate to Filebase (CAR import
# via S3 API preserves the original CID; free tier compatible).
#
# Credentials (optional, enables Filebase replication): ~/.igit/filebase.env
#   FILEBASE_ACCESS_KEY=...
#   FILEBASE_SECRET_KEY=...
#   FILEBASE_BUCKET=...
#
# Usage: pin-indexer.sh [--once]   (default: loop every $INTERVAL seconds)
set -uo pipefail

LCD="${LCD:-https://testnet.sentry.lcd.injective.network:443}"
CONTRACT="${CONTRACT:-inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh}"
INTERVAL="${INTERVAL:-60}"
IGIT_HOME="${IGIT_HOME:-$HOME/.igit}"
STATE="$IGIT_HOME/pinned.list"
ENV_FILE="$IGIT_HOME/filebase.env"

# shellcheck disable=SC1090
[ -f "$ENV_FILE" ] && . "$ENV_FILE"

# Upload a CAR to Filebase so it pins under the ORIGINAL CID. Prefer awscli
# (works regardless of curl version; Debian 11 curl lacks --aws-sigv4), else
# fall back to curl's SigV4.
fb_upload_car() {
  local cid="$1" car="$2"
  if command -v aws >/dev/null 2>&1; then
    AWS_ACCESS_KEY_ID="$FILEBASE_ACCESS_KEY" \
    AWS_SECRET_ACCESS_KEY="$FILEBASE_SECRET_KEY" \
    AWS_DEFAULT_REGION=us-east-1 AWS_EC2_METADATA_DISABLED=true \
    aws s3api put-object --endpoint-url https://s3.filebase.com \
      --bucket "$FILEBASE_BUCKET" --key "${cid}.car" \
      --body "$car" --metadata import=car >/dev/null 2>&1
  else
    curl -sS --http1.1 -m 300 -X PUT --aws-sigv4 aws:amz:us-east-1:s3 \
      --user "${FILEBASE_ACCESS_KEY}:${FILEBASE_SECRET_KEY}" \
      -H "x-amz-meta-import: car" --upload-file "$car" -o /dev/null \
      "https://s3.filebase.com/${FILEBASE_BUCKET}/${cid}.car"
  fi
}

pin_one() {
  local cid="$1"
  grep -qxF "$cid" "$STATE" 2>/dev/null && return 0
  echo "[pin] $cid"
  if ! ipfs pin add --timeout=120s "$cid" >/dev/null; then
    echo "[warn] local pin failed (provider offline?): $cid" >&2
    return 1
  fi
  if [ -n "${FILEBASE_ACCESS_KEY:-}" ]; then
    local car
    car=$(mktemp --suffix=.car)
    if ipfs dag export "$cid" > "$car" 2>/dev/null; then
      if fb_upload_car "$cid" "$car"; then
        echo "[filebase] replicated $cid"
      else
        echo "[warn] filebase upload failed: $cid" >&2
      fi
    fi
    rm -f "$car"
  fi
  echo "$cid" >> "$STATE"
}

scan_once() {
  # Events carry no pack_uris, so parse them out of the tx messages instead.
  curl -s -G "${LCD}/cosmos/tx/v1beta1/txs" \
    --data-urlencode "query=wasm._contract_address='${CONTRACT}' AND wasm.action='update_ref'" \
    --data-urlencode "pagination.limit=100" \
    --data-urlencode "order_by=ORDER_BY_DESC" \
  | jq -r '.tx_responses[]?.tx.body.messages[]?
      | (.msg.update_ref.pack_uris // [])[]' 2>/dev/null \
  | sed 's|^ipfs://||' | sort -u \
  | while read -r cid; do
      [ -n "$cid" ] && pin_one "$cid"
    done
}

mkdir -p "$IGIT_HOME"
touch "$STATE"

if [ "${1:-}" = "--once" ]; then
  scan_once
  exit 0
fi
echo "pin-indexer: watching ${CONTRACT} every ${INTERVAL}s (state: ${STATE})"
while true; do
  scan_once
  sleep "$INTERVAL"
done
