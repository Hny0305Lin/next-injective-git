#!/usr/bin/env bash
# Compatibility shim — moved to scripts/deploy/testnet-demo-repo.sh (remove after one release cycle)
echo "[DEPRECATED] testnet-demo-repo.sh has moved to scripts/deploy/testnet-demo-repo.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/deploy/testnet-demo-repo.sh" "$@"
