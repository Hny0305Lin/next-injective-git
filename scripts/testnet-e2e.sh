#!/usr/bin/env bash
# Compatibility shim — moved to scripts/deploy/testnet-e2e.sh (remove after one release cycle)
echo "[DEPRECATED] testnet-e2e.sh has moved to scripts/deploy/testnet-e2e.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/deploy/testnet-e2e.sh" "$@"
