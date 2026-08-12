#!/usr/bin/env bash
# Explicit opt-in wrapper for the mutating CosmWasm V1 testnet regression.
# V2 checks must use scripts/evm-v2-check.sh and migration-readiness.sh.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
export IGIT_ALLOW_LEGACY_V1=1
exec "$ROOT/scripts/testnet-e2e.sh" "$@"
