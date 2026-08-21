#!/usr/bin/env bash
# Acceptance probe for the US controlled replication data plane.
#
# Health-only mode is safe to run from any client:
#   REPLICATION_BASE_URL=https://igit-us.example \
#     bash replication-acceptance.sh --health
#
# Full mode is intended for an operator workstation or the US host. It needs
# an operator-issued identity token and a CID that the US Kubo can fetch from
# the temporary local publisher:
#   REPLICATION_BASE_URL=https://igit-us.example \
#   REPLICATION_IDENTITY_TOKEN='...' \
#   REPLICATION_CID=bafy... REPLICATION_OWNER=inj1... \
#   REPLICATION_REPO=demo REPLICATION_REF=refs/heads/main \
#   REPLICATION_PACK_SHA256=... REPLICATION_SIZE=123 \
#     bash replication-acceptance.sh
#
# The token is read only from the environment and is never printed.
set -euo pipefail

BASE_URL="${REPLICATION_BASE_URL:?set REPLICATION_BASE_URL to the US HTTPS endpoint}"
BASE_URL="${BASE_URL%/}"

health() {
  local status
  status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "${BASE_URL}/healthz")"
  test "$status" = "200" || {
    echo "US replication health check returned HTTP ${status}" >&2
    return 1
  }
  echo "US replication health: OK"
}

if [[ "${1:-}" == "--health" ]]; then
  health
  exit 0
fi

TOKEN="${REPLICATION_IDENTITY_TOKEN:?set REPLICATION_IDENTITY_TOKEN for full acceptance}"
CID="${REPLICATION_CID:?set REPLICATION_CID}"
OWNER="${REPLICATION_OWNER:?set REPLICATION_OWNER}"
REPO="${REPLICATION_REPO:?set REPLICATION_REPO}"
REF="${REPLICATION_REF:?set REPLICATION_REF}"
PACK_SHA256="${REPLICATION_PACK_SHA256:?set REPLICATION_PACK_SHA256}"
SIZE="${REPLICATION_SIZE:?set REPLICATION_SIZE}"

command -v jq >/dev/null || { echo 'jq is required' >&2; exit 1; }
[[ "$PACK_SHA256" =~ ^[[:xdigit:]]{64}$ ]] || { echo 'REPLICATION_PACK_SHA256 must be 64 hex characters' >&2; exit 1; }
[[ "$SIZE" =~ ^[1-9][0-9]*$ ]] || { echo 'REPLICATION_SIZE must be a positive integer' >&2; exit 1; }

health

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
request="$(jq -cn \
  --arg cid "$CID" --arg owner "$OWNER" --arg repo "$REPO" --arg ref "$REF" \
  --arg sha "$PACK_SHA256" --argjson size "$SIZE" \
  '{cid:$cid,owner:$owner,repo:$repo,ref:$ref,pack_sha256:$sha,size:$size}')"

status="$(curl -sS --max-time 30 -o "$tmp/authorization.json" -w '%{http_code}' \
  -H 'Content-Type: application/json' -H "Authorization: Bearer ${TOKEN}" \
  --data "$request" "${BASE_URL}/v1/upload-authorizations")"
test "$status" = "201" || {
  echo "upload authorization returned HTTP ${status}" >&2
  jq -c . "$tmp/authorization.json" >&2 || cat "$tmp/authorization.json" >&2
  exit 1
}
ticket="$(jq -er '.authorization' "$tmp/authorization.json")"

status="$(curl -sS --max-time 900 -o "$tmp/first.json" -w '%{http_code}' \
  -H 'Content-Type: application/json' -H "Authorization: Bearer ${ticket}" \
  --data "$request" "${BASE_URL}/v1/replications")"
test "$status" = "201" || {
  echo "first replication returned HTTP ${status}" >&2
  jq -c . "$tmp/first.json" >&2 || cat "$tmp/first.json" >&2
  exit 1
}
jq -e --arg cid "$CID" '.pinned == true and .cid == $cid' "$tmp/first.json" >/dev/null

# Replaying the exact ticket is expected to be a 200 idempotent response. This
# catches both accidental JTI reuse and quota accounting on the retry path.
status="$(curl -sS --max-time 900 -o "$tmp/retry.json" -w '%{http_code}' \
  -H 'Content-Type: application/json' -H "Authorization: Bearer ${ticket}" \
  --data "$request" "${BASE_URL}/v1/replications")"
test "$status" = "200" || {
  echo "idempotent replication retry returned HTTP ${status}" >&2
  jq -c . "$tmp/retry.json" >&2 || cat "$tmp/retry.json" >&2
  exit 1
}
jq -e --arg cid "$CID" '.idempotent == true and .pinned == true and .cid == $cid' "$tmp/retry.json" >/dev/null

echo "US replication acceptance: success + JTI idempotency verified for ${CID}"
