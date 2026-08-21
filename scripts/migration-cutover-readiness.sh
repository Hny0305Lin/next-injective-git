#!/usr/bin/env bash
# Compatibility shim — moved to scripts/migration/migration-cutover-readiness.sh (remove after one release cycle)
echo "[DEPRECATED] migration-cutover-readiness.sh has moved to scripts/migration/migration-cutover-readiness.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/migration/migration-cutover-readiness.sh" "$@"
