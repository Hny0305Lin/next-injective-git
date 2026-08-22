#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/replication-config-check.sh (remove after one release cycle)
echo "[DEPRECATED] replication-config-check.sh has moved to scripts/ops/replication-config-check.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/replication-config-check.sh" "$@"
