#!/usr/bin/env bash
# Monitor US Kubo, archive replication, Fil.one capacity, and durable-CID sync.
set -euo pipefail

STATE_DIR="${STATE_DIR:-/var/lib/igit-archive-monitor}"
ARCHIVED_STATE="${ARCHIVED_STATE:-/var/lib/igit-archive/archived.tsv}"
MAX_BYTES="${MAX_BYTES:-1000000000000}"
ALERT_BANDS="${ALERT_BANDS:-60 80 90 100}"
ALERT_INTERVAL_HOURS="${ALERT_INTERVAL_HOURS:-8}"
test_mode=false
[ "${1:-}" = "--test" ] && test_mode=true

: "${FILONE_ACCESS_KEY:?missing FILONE_ACCESS_KEY}"
: "${FILONE_SECRET_KEY:?missing FILONE_SECRET_KEY}"
: "${FILONE_BUCKET:?missing FILONE_BUCKET}"
: "${FILONE_ENDPOINT:?missing FILONE_ENDPOINT}"
: "${RESEND_API_KEY:?missing RESEND_API_KEY}"
: "${ALERT_TO:?missing ALERT_TO}"

export AWS_ACCESS_KEY_ID="$FILONE_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$FILONE_SECRET_KEY"
export AWS_DEFAULT_REGION="${FILONE_REGION:-us-east-1}"
export AWS_EC2_METADATA_DISABLED=true
mkdir -p "$STATE_DIR"

alerts=""
add_alert() {
    alerts="${alerts}$1\n"
}

if ! systemctl is-active --quiet kubo.service; then
    add_alert "美国 Kubo 服务未运行"
fi
if ! systemctl is-active --quiet igit-archive-indexer.timer; then
    add_alert "美国 CAR 归档定时器未运行"
fi
if ! systemctl is-active --quiet igit-durable-cid-sync.timer; then
    add_alert "香港 durable CID 同步定时器未运行"
fi

local_count=$(wc -l < "$ARCHIVED_STATE")
pin_count=$(wc -l < /var/lib/igit-archive/pinned.list)
if [ "$local_count" -ne "$pin_count" ]; then
    add_alert "美国 Pin 清单 ${pin_count} 与 CAR 归档清单 ${local_count} 不一致"
fi

remote_count="unavailable"
remote_bytes=0
usage="unavailable"
summary=""
if summary=$(aws s3 ls "s3://${FILONE_BUCKET}/cars/v1/" --recursive --summarize --endpoint-url "$FILONE_ENDPOINT" 2>&1); then
    remote_count=$(printf '%s\n' "$summary" | awk -F: '/Total Objects/{gsub(/ /,"",$2); print $2}')
    remote_bytes=$(printf '%s\n' "$summary" | awk -F: '/Total Size/{gsub(/ /,"",$2); print $2}')
    remote_count=${remote_count:-0}
    remote_bytes=${remote_bytes:-0}
    if [ "$remote_count" -ne "$local_count" ]; then
        add_alert "Fil.one CAR 对象 ${remote_count} 与美国归档清单 ${local_count} 不一致"
    fi
    usage=$(( remote_bytes * 100 / MAX_BYTES ))
    for band in $ALERT_BANDS; do
        if [ "$usage" -ge "$band" ]; then
            add_alert "Fil.one CAR 存储 ${usage}%（${remote_bytes}/${MAX_BYTES} bytes，预警档 ${band}%）"
        fi
    done
else
    add_alert "Fil.one 对象清单查询失败：${summary}"
fi

if [ "$test_mode" = true ]; then
    add_alert "美国归档监控测试：Kubo、Fil.one CAR 与香港同步检查已执行"
fi

vps_disk=$(df -hP / | awk 'NR == 2 {printf "总计 %s，已用 %s，可用 %s（%s）", $2, $3, $4, $5}')
repo_size="unavailable"
repo_max="unavailable"
repo_stat=$(runuser -u ipfs -- env IPFS_PATH=/var/lib/ipfs ipfs repo stat 2>/dev/null || true)
repo_size=$(printf '%s\n' "$repo_stat" | awk '/RepoSize:/{print $2}')
repo_max=$(printf '%s\n' "$repo_stat" | awk '/StorageMax:/{print $2}')
repo_size=${repo_size:-unavailable}
repo_max=${repo_max:-unavailable}
if [ "$remote_count" = unavailable ]; then
    bucket_storage="查询失败"
else
    bucket_mib=$(awk "BEGIN{printf \"%.2f\", $remote_bytes/1048576}")
    bucket_max_gb=$(awk "BEGIN{printf \"%.0f\", $MAX_BYTES/1000000000}")
    bucket_storage="${bucket_mib} MiB / ${bucket_max_gb} GB（${usage}%）"
fi

[ -n "$alerts" ] || exit 0
now=$(date +%s)
stamp="$STATE_DIR/last-alert"
last=$(cat "$stamp" 2>/dev/null || echo 0)
if [ "$test_mode" = false ] && [ $(( now - last )) -lt $(( ALERT_INTERVAL_HOURS * 3600 )) ]; then
    exit 0
fi
printf '%s\n' "$now" > "$stamp"

ALERT_FROM="${ALERT_FROM:-igit Archive Monitor <onboarding@resend.dev>}"
subject="[igit] 美国 IPFS/Fil.one 存储预警"
[ "$test_mode" = true ] && subject="[igit] 美国 IPFS/Fil.one 存储监控测试"
body=$(printf '美国 IPFS/Fil.one 存储预警：\n\n%b\nVPS 根分区：%s\nKubo 仓库：%s / %s\nFilecoin 存储桶：%s\nFilecoin 对象数：%s\n归档 CID：%s\n美国 Pin CID：%s\n\n— igit archive-monitor @ %s\n' \
    "$alerts" "$vps_disk" "$repo_size" "$repo_max" "$bucket_storage" "$remote_count" "$local_count" "$pin_count" "$(hostname)")
payload=$(jq -n --arg from "$ALERT_FROM" --arg to "$ALERT_TO" --arg subject "$subject" --arg text "$body" \
    '{from:$from, to:[$to], subject:$subject, text:$text}')
response=$(curl -sS -w '\n%{http_code}' -X POST https://api.resend.com/emails \
    -H "Authorization: Bearer ${RESEND_API_KEY}" \
    -H 'Content-Type: application/json' -d "$payload")
code=$(printf '%s\n' "$response" | tail -1)
[ "$code" = 200 ] || { echo "Resend HTTP $code" >&2; exit 1; }
