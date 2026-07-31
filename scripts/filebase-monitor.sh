#!/usr/bin/env bash
# filebase-monitor: check Filebase free-tier usage (object count + total size)
# and email an alert via Resend when either crosses a usage band.
# Bands are configurable via ALERT_BANDS (default "60 80 90 100", percent).
# Designed to run hourly on the HK node via a systemd timer.
#
# Config (env file, 600 perms, NOT committed): ${MONITOR_ENV:-/etc/igit/monitor.env}
#   FILEBASE_ACCESS_KEY=...
#   FILEBASE_SECRET_KEY=...
#   FILEBASE_BUCKET=...
#   RESEND_API_KEY=...
#   ALERT_FROM="Filebase Monitor <onboarding@resend.dev>"
#   ALERT_TO=you@example.com
set -euo pipefail

MONITOR_ENV="${MONITOR_ENV:-/etc/igit/monitor.env}"
STATE_DIR="${STATE_DIR:-/var/lib/filebase-monitor}"
MAX_FILES="${MAX_FILES:-1000}"        # free-tier file cap
MAX_BYTES="${MAX_BYTES:-5000000000}"  # free-tier 5 GB (conservative decimal GB)
ALERT_BANDS="${ALERT_BANDS:-60 80 90 100}"  # % thresholds, ascending
ALERT_INTERVAL_HOURS="${ALERT_INTERVAL_HOURS:-8}"  # re-alert cooldown per band (hours)

# shellcheck disable=SC1090
[ -f "$MONITOR_ENV" ] && . "$MONITOR_ENV"
mkdir -p "$STATE_DIR"

: "${FILEBASE_ACCESS_KEY:?missing in $MONITOR_ENV}"
: "${FILEBASE_SECRET_KEY:?missing in $MONITOR_ENV}"
: "${FILEBASE_BUCKET:?missing in $MONITOR_ENV}"

# ---- 1. tally the bucket via awscli ----
# NOTE: Debian 11's curl (7.74) lacks --aws-sigv4, so we use awscli, which
# also auto-paginates ListObjectsV2 and handles SigV4 against Filebase's S3.
export AWS_ACCESS_KEY_ID="$FILEBASE_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$FILEBASE_SECRET_KEY"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_EC2_METADATA_DISABLED=true
FILEBASE_ENDPOINT="${FILEBASE_ENDPOINT:-https://s3.filebase.com}"

summary=$(aws s3 ls "s3://${FILEBASE_BUCKET}" --recursive --summarize \
  --endpoint-url "$FILEBASE_ENDPOINT" | tail -5)
count=$(echo "$summary" | awk -F: '/Total Objects/{gsub(/ /,"",$2); print $2}')
bytes=$(echo "$summary" | awk -F: '/Total Size/{gsub(/ /,"",$2); print $2}')
count=${count:-0}
bytes=${bytes:-0}

# ---- 2. compute percentage + band for each metric ----
band() { # $1=pct -> highest ALERT_BANDS threshold <= pct, else 0
  local b=0 t
  for t in $ALERT_BANDS; do
    if [ "$1" -ge "$t" ]; then b=$t; fi
  done
  echo "$b"
}
file_pct=$(( count * 100 / MAX_FILES ))
size_pct=$(( bytes * 100 / MAX_BYTES ))
file_band=$(band "$file_pct")
size_band=$(band "$size_pct")
size_mb=$(awk "BEGIN{printf \"%.1f\", $bytes/1048576}")

echo "files: $count/$MAX_FILES (${file_pct}%, band ${file_band})"
echo "size : ${size_mb} MiB / $((MAX_BYTES/1000000)) MB (${size_pct}%, band ${size_band})"

# ---- 3. decide what to alert (re-alert a band at most every N hours) ----
now=$(date +%s)
cooldown=$(( ALERT_INTERVAL_HOURS * 3600 ))
alerts=""
maybe_alert() { # $1=metric label $2=band $3=detail line
  [ "$2" -eq 0 ] && return 0
  local stamp="$STATE_DIR/${1}-${2}" last
  last=$(cat "$stamp" 2>/dev/null || echo 0)
  case "$last" in ''|*[!0-9]*) last=0;; esac
  if [ $(( now - last )) -lt "$cooldown" ]; then return 0; fi
  alerts="${alerts}${3}\n"
  echo "$now" > "$stamp"
}
maybe_alert "files" "$file_band" "文件数 ${count}/${MAX_FILES} 已达 ${file_pct}%（预警档 ${file_band}%）"
maybe_alert "size"  "$size_band" "存储 ${size_mb} MiB 已达 ${size_pct}%（预警档 ${size_band}%）"

if [ -z "$alerts" ]; then
  echo "no band due for alert (within ${ALERT_INTERVAL_HOURS}h cooldown); nothing to send."
  exit 0
fi

# ---- 4. send via Resend ----
: "${RESEND_API_KEY:?missing in $MONITOR_ENV}"
: "${ALERT_TO:?missing in $MONITOR_ENV}"
ALERT_FROM="${ALERT_FROM:-Filebase Monitor <onboarding@resend.dev>}"
subject="⚠️ Filebase 用量预警：files ${file_pct}% / storage ${size_pct}%"
body=$(printf 'Filebase bucket %s 用量预警：\n\n%b\n限额：文件 %s 个 / 存储 %s MB（免费档）\n请及时清理或升级付费档。\n\n— igit filebase-monitor @ %s' \
  "$FILEBASE_BUCKET" "$alerts" "$MAX_FILES" "$((MAX_BYTES/1000000))" "$(hostname)")

payload=$(jq -n --arg from "$ALERT_FROM" --arg to "$ALERT_TO" \
  --arg subject "$subject" --arg text "$body" \
  '{from:$from, to:[$to], subject:$subject, text:$text}')

resp=$(curl -s -w '\n%{http_code}' -X POST https://api.resend.com/emails \
  -H "Authorization: Bearer ${RESEND_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "$payload")
code=$(echo "$resp" | tail -1)
echo "resend HTTP $code: $(echo "$resp" | head -1)"
[ "$code" = "200" ] || { echo "email send FAILED" >&2; exit 1; }
echo "alert email sent to $ALERT_TO"
