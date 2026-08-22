#!/usr/bin/env bash
# Compatibility shim — moved to scripts/deploy/verify-release-assets.sh (remove after one release cycle)
echo "[DEPRECATED] verify-release-assets.sh has moved to scripts/deploy/verify-release-assets.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/deploy/verify-release-assets.sh" "$@"
