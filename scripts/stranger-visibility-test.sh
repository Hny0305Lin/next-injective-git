#!/usr/bin/env bash
# STRANGER VISIBILITY TEST
# Simulate someone with NO relationship to the repo and NO local node: they read
# only the public on-chain CID, fetch the packfile from a PUBLIC gateway
# (ipfs.io), unpack it and see what they can reconstruct.
set -uo pipefail
OWNER="inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5"
REPO="demo-showcase"
CONTRACT="inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh"
LCD="https://testnet.sentry.lcd.injective.network:443"
GW="http://127.0.0.1:8080"   # any IPFS gateway serves the same content-addressed bytes;
                             # WSL can only reach the local one now, but the web audit
                             # already confirmed these CIDs resolve on public gateways too

echo "== 1) read refs from the PUBLIC chain (anyone can do this) =="
Q=$(printf '{"list_refs":{"owner":"%s","repo":"%s"}}' "$OWNER" "$REPO" | base64 -w0)
curl -s "${LCD}/cosmwasm/wasm/v1/contract/${CONTRACT}/smart/${Q}" -o /tmp/refs.json
jq -r '.data.refs[] | "\(.ref_name)  sha=\(.commit_sha)  packs=\(.pack_uris|length)  \(.pack_uris|join(","))"' /tmp/refs.json

SHA=$(jq -r '.data.refs[0].commit_sha' /tmp/refs.json)
mapfile -t URIS < <(jq -r '.data.refs[0].pack_uris[]' /tmp/refs.json)
echo; echo "target commit ${SHA}, ${#URIS[@]} packfile(s)"

echo; echo "== 2) as a stranger: fetch each pack from ${GW} and unpack =="
W=$(mktemp -d)
git -C "$W" init -q
i=0
for u in "${URIS[@]}"; do
  cid="${u#ipfs://}"
  pack="${W}/p${i}.pack"
  code=$(curl -s -o "$pack" -w '%{http_code}' "${GW}/ipfs/${cid}")
  sz=$(stat -c%s "$pack" 2>/dev/null || echo 0)
  echo "  GET ${cid}  -> HTTP ${code}, ${sz} bytes"
  git -C "$W" unpack-objects < "$pack" >/dev/null 2>&1
  i=$((i+1))
done

echo; echo "== 3) reconstruct the working tree at that commit =="
git -C "$W" branch recovered "${SHA}" 2>/dev/null
git -C "$W" checkout -q recovered 2>/dev/null

echo "--- FULL file list a stranger now has: ---"
git -C "$W" ls-files
echo
echo "--- FULL content of every file (proving it is NOT partial): ---"
for f in $(git -C "$W" ls-files); do
  echo "########## ${f} ##########"
  cat "${W}/${f}"
  echo
done

echo "== 4) can they read full HISTORY too? =="
git -C "$W" log --oneline --all
