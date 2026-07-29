#!/usr/bin/env bash
# Inspect @injectivelabs/sdk-ts registry metadata to pick a safe pinned version.
set -uo pipefail
curl -s https://registry.npmjs.org/@injectivelabs%2Fsdk-ts -o /tmp/sdk.json
echo "latest dist-tag:"
jq -r '.["dist-tags"].latest' /tmp/sdk.json
echo
echo "is 1.20.21 deprecated?"
jq -r '.versions["1.20.21"].deprecated // "NOT deprecated / absent"' /tmp/sdk.json
echo
echo "most recent 8 published versions (version  time):"
jq -r '.time | to_entries
  | map(select(.key | test("^[0-9]+\\.[0-9]+\\.[0-9]+$")))
  | sort_by(.value) | reverse | .[:8][]
  | .key + "  " + .value' /tmp/sdk.json
echo
echo "latest version publish time:"
L=$(jq -r '.["dist-tags"].latest' /tmp/sdk.json)
jq -r --arg L "$L" '.time[$L]' /tmp/sdk.json
