#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/durable-cid-sync.sh (remove after one release cycle)
echo "[DEPRECATED] durable-cid-sync.sh has moved to scripts/ops/durable-cid-sync.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/durable-cid-sync.sh" "$@"
