#!/usr/bin/env bash
# Smoke test for igit collab/transfer/repo edit/mod against Injective testnet.
set -uo pipefail

OWNER=$(injectived keys show igit-dev -a --keyring-backend test)
BOB=$(injectived keys show collab-bob -a --keyring-backend test)
REPO="${1:?usage: testnet-smoke-admin.sh <repo>}"
PASS=0; FAIL=0
ok()  { echo "PASS: $*"; PASS=$((PASS+1)); }
bad() { echo "FAIL: $*"; FAIL=$((FAIL+1)); }

echo "owner=${OWNER} bob=${BOB} repo=${REPO}"

echo; echo "== collab add =="
igit collab add "${REPO}" "${BOB}" maintainer && ok "collab add" || bad "collab add"
sleep 4
igit collab list "${OWNER}" "${REPO}" | grep -qi "maintainer.*${BOB}" && ok "collab list shows bob" || bad "collab list missing bob"

echo; echo "== collab remove =="
igit collab remove "${REPO}" "${BOB}" && ok "collab remove" || bad "collab remove"
sleep 4
igit collab list "${OWNER}" "${REPO}" | grep -q "${BOB}" && bad "bob still listed" || ok "bob removed"

echo; echo "== repo edit =="
igit repo edit "${REPO}" description smoke-updated description && ok "repo edit description" || bad "repo edit description"
sleep 4
igit repos "${OWNER}" | grep -q "smoke-updated" && ok "description updated on chain" || bad "description not updated"

echo; echo "== mod freeze -> push rejected -> unfreeze =="
igit mod "${OWNER}" "${REPO}" frozen cafebabe && ok "mod frozen" || bad "mod frozen"
sleep 4
# a push against a frozen repo must fail (use a throwaway commit)
W=$(mktemp -d); cd "$W"
git init -q && git config user.email s@e.c && git config user.name s
echo x > f && git add f && git commit -qm smoke
git remote add inj "inj://${OWNER}/${REPO}"
if OUT=$(git push inj main:refs/heads/smoke-frozen 2>&1); then
  echo "$OUT"
  bad "push not rejected while frozen"
else
  echo "$OUT" | tail -3
  echo "$OUT" | grep -qi "frozen" && ok "push rejected while frozen" || bad "push failed but not due to freeze"
fi
igit mod "${OWNER}" "${REPO}" active && ok "mod active (unfreeze)" || bad "mod active"
sleep 4
if git push inj main:refs/heads/smoke-frozen 2>&1 | grep -q "new branch"; then
  ok "push works after unfreeze"
else
  bad "push still failing after unfreeze"
fi
# cleanup the smoke branch
git push inj :refs/heads/smoke-frozen >/dev/null 2>&1

echo; echo "PASS=${PASS} FAIL=${FAIL}"
[ "${FAIL}" = 0 ]
