#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/replication-acceptance.sh (remove after one release cycle)
echo "[DEPRECATED] replication-acceptance.sh has moved to scripts/ops/replication-acceptance.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/replication-acceptance.sh" "$@"
