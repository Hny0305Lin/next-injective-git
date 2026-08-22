#!/usr/bin/env bash
# Badge smoke: award to bob on demo-showcase, then list his trophy wall.
set -uo pipefail
BOB=$(injectived keys show collab-bob -a --keyring-backend test)
PASS=0; FAIL=0
ok()  { echo "PASS: $*"; PASS=$((PASS+1)); }
bad() { echo "FAIL: $*"; FAIL=$((FAIL+1)); }

igit badge award demo-showcase "${BOB}" "early tester and collaborator" && ok "badge award" || bad "badge award"
sleep 4
OUT=$(igit badge list "${BOB}")
echo "${OUT}"
echo "${OUT}" | grep -q "early tester" && ok "trophy wall shows badge" || bad "badge missing from wall"

# self-award must fail
if igit badge award demo-showcase "$(injectived keys show igit-dev -a --keyring-backend test)" self 2>/dev/null; then
  bad "self-award accepted"
else
  ok "self-award rejected"
fi

echo "PASS=${PASS} FAIL=${FAIL}"
[ "${FAIL}" = 0 ]
