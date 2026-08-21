#!/usr/bin/env bash
# Compatibility shim — moved to scripts/deploy/verify-metamask-tx.sh (remove after one release cycle)
echo "[DEPRECATED] verify-metamask-tx.sh has moved to scripts/deploy/verify-metamask-tx.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/deploy/verify-metamask-tx.sh" "$@"
