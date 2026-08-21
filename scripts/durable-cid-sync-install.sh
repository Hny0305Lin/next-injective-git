#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/durable-cid-sync-install.sh (remove after one release cycle)
echo "[DEPRECATED] durable-cid-sync-install.sh has moved to scripts/ops/durable-cid-sync-install.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/durable-cid-sync-install.sh" "$@"
