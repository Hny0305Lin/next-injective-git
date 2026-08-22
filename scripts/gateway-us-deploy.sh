#!/usr/bin/env bash
# Compatibility shim — moved to scripts/deploy/gateway-us-deploy.sh (remove after one release cycle)
echo "[DEPRECATED] gateway-us-deploy.sh has moved to scripts/deploy/gateway-us-deploy.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/deploy/gateway-us-deploy.sh" "$@"
