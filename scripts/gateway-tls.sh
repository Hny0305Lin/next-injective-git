#!/usr/bin/env bash
# Compatibility shim — moved to scripts/deploy/gateway-tls.sh (remove after one release cycle)
echo "[DEPRECATED] gateway-tls.sh has moved to scripts/deploy/gateway-tls.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/deploy/gateway-tls.sh" "$@"
