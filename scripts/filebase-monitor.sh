#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/filebase-monitor.sh (remove after one release cycle)
echo "[DEPRECATED] filebase-monitor.sh has moved to scripts/ops/filebase-monitor.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/filebase-monitor.sh" "$@"
