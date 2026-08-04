#!/usr/bin/env bash
# Offline regression tests for replication-reaper.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REAPER="$ROOT/scripts/replication-reaper.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAKEBIN="$TMP/bin"
mkdir -p "$FAKEBIN"

cat > "$FAKEBIN/curl" <<'EOF'
#!/usr/bin/env bash
if [[ "${FAKE_CURL_FAIL:-0}" == "1" ]]; then
  exit 22
fi
printf '%s\n' '{"data":{"refs":[{"ref_name":"refs/heads/main","pack_uris":["ipfs://live-cid"]}]}}'
EOF

cat > "$FAKEBIN/ipfs" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$FAKE_IPFS_LOG"
case "$*" in
  "pin rm "*) exit 0 ;;
  "repo gc") exit 0 ;;
  *) exit 1 ;;
esac
EOF

cat > "$FAKEBIN/logger" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$FAKE_LOGGER"
EOF

chmod 700 "$FAKEBIN/curl" "$FAKEBIN/ipfs" "$FAKEBIN/logger"

OWNER="inj1$(printf 'a%.0s' {1..38})"
CONTRACT="inj1$(printf 'c%.0s' {1..38})"
STATE="$TMP/issued.tsv"
REAPED="$TMP/reaped.tsv"
IPFS_LOG="$TMP/ipfs.log"
LOGGER_LOG="$TMP/logger.log"
printf 'jti-stale\tstale-cid\t1\talice\t%s\tdemo\trefs/heads/main\t10\n' "$OWNER" > "$STATE"
printf 'jti-live\tlive-cid\t1\talice\t%s\tdemo\trefs/heads/main\t10\n' "$OWNER" >> "$STATE"

PATH="$FAKEBIN:$PATH" \
  LCD=https://lcd.invalid \
  CHAIN_ID=injective-1 \
  CONTRACT="$CONTRACT" \
  IGIT_REPLICATION_STATE="$STATE" \
  IGIT_REPLICATION_REAPED_STATE="$REAPED" \
  FAKE_IPFS_LOG="$IPFS_LOG" \
  FAKE_LOGGER="$LOGGER_LOG" \
  bash "$REAPER"

grep -Fq 'pin rm stale-cid' "$IPFS_LOG"
! grep -Fq 'pin rm live-cid' "$IPFS_LOG"
grep -Fxq 'stale-cid' "$REAPED"
grep -Fq 'repo gc' "$IPFS_LOG"
grep -Fq 'reclaimed unreferenced cid=stale-cid' "$LOGGER_LOG"

printf 'malformed\n' > "$TMP/malformed.tsv"
if PATH="$FAKEBIN:$PATH" \
  LCD=https://lcd.invalid \
  CHAIN_ID=injective-1 \
  CONTRACT="$CONTRACT" \
  IGIT_REPLICATION_STATE="$TMP/malformed.tsv" \
  IGIT_REPLICATION_REAPED_STATE="$TMP/malformed.reaped" \
  FAKE_IPFS_LOG="$TMP/malformed.ipfs.log" \
  FAKE_LOGGER="$TMP/malformed.logger.log" \
  bash "$REAPER" >/dev/null 2>&1; then
  echo "replication reaper test: malformed state unexpectedly succeeded" >&2
  exit 1
fi
test ! -s "$TMP/malformed.ipfs.log"

if PATH="$FAKEBIN:$PATH" \
  FAKE_CURL_FAIL=1 \
  LCD=https://lcd.invalid \
  CHAIN_ID=injective-1 \
  CONTRACT="$CONTRACT" \
  IGIT_REPLICATION_STATE="$STATE" \
  IGIT_REPLICATION_REAPED_STATE="$TMP/query-failure.reaped" \
  FAKE_IPFS_LOG="$TMP/query-failure.ipfs.log" \
  FAKE_LOGGER="$TMP/query-failure.logger.log" \
  bash "$REAPER" >/dev/null 2>&1; then
  echo "replication reaper test: failed ref query unexpectedly succeeded" >&2
  exit 1
fi
test ! -s "$TMP/query-failure.ipfs.log"

echo "replication reaper test: live CID preservation, stale CID reclaim, malformed-state fail-closed, and query-failure fail-closed checks passed"
