#!/usr/bin/env bash
# Compatibility shim — moved to scripts/storage/bootstrap-push.sh (remove after one release cycle)
echo "[DEPRECATED] bootstrap-push.sh has moved to scripts/storage/bootstrap-push.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/storage/bootstrap-push.sh" "$@"
