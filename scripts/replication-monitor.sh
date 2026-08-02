#!/usr/bin/env bash
# Emit simple Prometheus textfile metrics for the US replication control plane.
set -euo pipefail

STATE="${IGIT_REPLICATION_STATE:-/var/lib/igit-replication/issued.tsv}"
REAPED_STATE="${IGIT_REPLICATION_REAPED_STATE:-${STATE}.reaped}"
OUT="${IGIT_REPLICATION_METRICS:-/var/lib/node_exporter/textfile_collector/igit_replication.prom}"
mkdir -p "$(dirname "$OUT")"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

failed=$(journalctl -u igit-replication.service --since '1 hour ago' --no-pager 2>/dev/null | grep -Ec '"event":"(denied|pin_failed|hash_failed|state_failed)"' || true)
issued=0
unreferenced=0
if [ -f "$STATE" ]; then
  issued=$(wc -l < "$STATE")
  now=$(date +%s)
  unreferenced=$(awk -F '\t' -v now="$now" '$3 < now { n++ } END { print n+0 }' "$STATE")
  if [ -f "$REAPED_STATE" ]; then
    reaped_total=$(wc -l < "$REAPED_STATE")
    unreferenced=$(( unreferenced - reaped_total ))
    [ "$unreferenced" -ge 0 ] || unreferenced=0
  fi
fi
reclaimed=$(journalctl -t igit-replication-reaper --since '24 hours ago' --no-pager 2>/dev/null | grep -c 'reclaimed unreferenced' || true)
repo_stat=$(runuser -u ipfs -- env IPFS_PATH=/var/lib/ipfs ipfs repo stat 2>/dev/null || true)
repo_size=$(printf '%s\n' "$repo_stat" | awk '/RepoSize:/{print $2}' | tr -dc '0-9')
repo_max=$(printf '%s\n' "$repo_stat" | awk '/StorageMax:/{print $2}' | tr -dc '0-9')
repo_size=${repo_size:-0}
repo_max=${repo_max:-0}

cat >"$tmp" <<EOF
# HELP igit_replication_upload_failures_last_hour Controlled replication failures observed in journal.
# TYPE igit_replication_upload_failures_last_hour gauge
igit_replication_upload_failures_last_hour ${failed}
# HELP igit_replication_authorized_pins Number of persisted US replication records.
# TYPE igit_replication_authorized_pins gauge
igit_replication_authorized_pins ${issued}
# HELP igit_replication_expired_unreferenced_candidates Expired records awaiting or eligible for TTL reconciliation.
# TYPE igit_replication_expired_unreferenced_candidates gauge
igit_replication_expired_unreferenced_candidates ${unreferenced}
# HELP igit_replication_ttl_reclaimed_last_day Pins reclaimed by the TTL reaper in 24 hours.
# TYPE igit_replication_ttl_reclaimed_last_day gauge
igit_replication_ttl_reclaimed_last_day ${reclaimed}
# HELP igit_us_kubo_repo_bytes Kubo repository bytes used and configured maximum.
# TYPE igit_us_kubo_repo_bytes gauge
igit_us_kubo_repo_bytes{kind="used"} ${repo_size}
igit_us_kubo_repo_bytes{kind="max"} ${repo_max}
EOF
install -m 0644 "$tmp" "$OUT"
