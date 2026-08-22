#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/durable-cid-receiver-install.sh (remove after one release cycle)
echo "[DEPRECATED] durable-cid-receiver-install.sh has moved to scripts/ops/durable-cid-receiver-install.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/durable-cid-receiver-install.sh" "$@"
