#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/archive-indexer.sh (remove after one release cycle)
echo "[DEPRECATED] archive-indexer.sh has moved to scripts/ops/archive-indexer.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/archive-indexer.sh" "$@"
