#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ci/replication-config-check-test.sh (remove after one release cycle)
echo "[DEPRECATED] replication-config-check-test.sh has moved to scripts/ci/replication-config-check-test.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ci/replication-config-check-test.sh" "$@"
