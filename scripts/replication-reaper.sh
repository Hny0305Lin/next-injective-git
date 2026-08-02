#!/usr/bin/env bash
# Reclaim US pins that were authorized but never referenced by update_ref.
# Run only on the US host; Kubo API remains loopback-only.
set -euo pipefail

LCD="${LCD:-https://testnet.sentry.lcd.injective.network:443}"
CONTRACT="${CONTRACT:?set CONTRACT}"
STATE="${IGIT_REPLICATION_STATE:-/var/lib/igit-replication/issued.tsv}"
REAPED_STATE="${IGIT_REPLICATION_REAPED_STATE:-${STATE}.reaped}"
NOW="$(date +%s)"

[ -f "$STATE" ] || exit 0
touch "$REAPED_STATE"
referenced="$(mktemp)"
pairs="$(mktemp)"
trap 'rm -f "$referenced" "$pairs"' EXIT
: > "$referenced"

# A malformed record must never be treated as an unreferenced upload.
awk -F '\t' 'NF >= 2 && $2 != "" && (NF < 7 || $5 == "" || $6 == "") { bad = 1 } END { exit bad }' "$STATE" || {
  echo "replication reaper: malformed state record; refusing to GC" >&2
  exit 1
}

# Resolve the contract's current state, not a bounded slice of historical
# transactions. A ref can remain live for months without a new update_ref;
# querying the current list is the only safe basis for TTL deletion. Any
# failed query aborts before a single pin is removed (fail closed).
awk -F '\t' 'NF >= 7 && $5 != "" && $6 != "" { print $5 "\t" $6 }' "$STATE" | sort -u > "$pairs"
while IFS=$'\t' read -r owner repo; do
  [ -n "$owner" ] || continue
  start_after=""
  while :; do
    query="$(jq -cn --arg owner "$owner" --arg repo "$repo" --arg start "$start_after" \
      '{list_refs:{owner:$owner,repo:$repo,start_after:(if $start == "" then null else $start end),limit:100}}')"
    encoded="$(printf '%s' "$query" | base64 -w0 | jq -sRr @uri)"
    response="$(curl -fsS "${LCD}/cosmwasm/wasm/v1/contract/${CONTRACT}/smart/${encoded}")" || {
      echo "replication reaper: failed to query current refs for ${owner}/${repo}; refusing to GC" >&2
      exit 1
    }
    refs="$(printf '%s' "$response" | jq -e '.data.refs | arrays')" || {
      echo "replication reaper: malformed current-ref response for ${owner}/${repo}; refusing to GC" >&2
      exit 1
    }
    printf '%s' "$refs" | jq -r '.[]?.pack_uris[]?' | sed 's|^ipfs://||' >> "$referenced"
    count="$(printf '%s' "$refs" | jq 'length')"
    [ "$count" -lt 100 ] && break
    start_after="$(printf '%s' "$refs" | jq -r '.[-1].ref_name')"
    [ -n "$start_after" ] || {
      echo "replication reaper: missing pagination cursor for ${owner}/${repo}; refusing to GC" >&2
      exit 1
    }
  done
done < "$pairs"
sort -u "$referenced" -o "$referenced"

while IFS=$'\t' read -r jti cid expires subject owner repo ref size; do
  [ -n "$cid" ] && [ "$expires" -lt "$NOW" ] || continue
  grep -qxF "$cid" "$REAPED_STATE" && continue
  if grep -qxF "$cid" "$referenced"; then continue; fi
  if ipfs pin rm "$cid" >/dev/null 2>&1; then
    printf '%s\n' "$cid" >> "$REAPED_STATE"
    logger -t igit-replication-reaper "reclaimed unreferenced cid=$cid subject=$subject repo=$owner/$repo ref=$ref size=$size"
  fi
done < "$STATE"
ipfs repo gc >/dev/null
