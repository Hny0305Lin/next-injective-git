#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ci/gateway-fallback-acceptance.sh (remove after one release cycle)
echo "[DEPRECATED] gateway-fallback-acceptance.sh has moved to scripts/ci/gateway-fallback-acceptance.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ci/gateway-fallback-acceptance.sh" "$@"
