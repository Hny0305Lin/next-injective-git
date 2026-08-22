#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/archive-monitor-install.sh (remove after one release cycle)
echo "[DEPRECATED] archive-monitor-install.sh has moved to scripts/ops/archive-monitor-install.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/archive-monitor-install.sh" "$@"
