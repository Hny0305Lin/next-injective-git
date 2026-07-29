#!/usr/bin/env bash
# Discover the current @injectivelabs wallet package names + latest versions.
set -uo pipefail
for pkg in wallet-ts wallet-strategy wallet-core wallet-evm wallet-cosmos sdk-ts; do
  enc="@injectivelabs%2F${pkg}"
  latest=$(curl -s "https://registry.npmjs.org/${enc}" | jq -r '.["dist-tags"].latest // "ABSENT"')
  echo "@injectivelabs/${pkg}  latest=${latest}"
done
echo "--- sdk-ts 1.20.27 eip712 exports (grep) ---"
curl -s "https://registry.npmjs.org/@injectivelabs%2Fsdk-ts/1.20.27" | jq -r '.dependencies | keys[]' 2>/dev/null | grep -i wallet || echo "(sdk-ts has no wallet dep)"
