#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/replication-monitor.sh (remove after one release cycle)
echo "[DEPRECATED] replication-monitor.sh has moved to scripts/ops/replication-monitor.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/replication-monitor.sh" "$@"
