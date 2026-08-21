#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ci/replication-reaper-test.sh (remove after one release cycle)
echo "[DEPRECATED] replication-reaper-test.sh has moved to scripts/ci/replication-reaper-test.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ci/replication-reaper-test.sh" "$@"
