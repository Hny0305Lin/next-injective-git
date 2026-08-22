#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/receive-durable-cids.sh (remove after one release cycle)
echo "[DEPRECATED] receive-durable-cids.sh has moved to scripts/ops/receive-durable-cids.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/receive-durable-cids.sh" "$@"
