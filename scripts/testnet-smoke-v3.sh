#!/usr/bin/env bash
# v3 smoke: usernames, username URLs, revenue splits, sponsorship.
set -uo pipefail

OWNER=$(injectived keys show igit-dev -a --keyring-backend test)
BOB=$(injectived keys show collab-bob -a --keyring-backend test)
LCD="https://testnet.sentry.lcd.injective.network:443"
NAME="igit-dev-$(date +%s | tail -c 5)"
REPO="v3-$(date +%s)"
PASS=0; FAIL=0
ok()  { echo "PASS: $*"; PASS=$((PASS+1)); }
bad() { echo "FAIL: $*"; FAIL=$((FAIL+1)); }
inj_balance() { curl -s "${LCD}/cosmos/bank/v1beta1/balances/$1" | jq -r '.balances[] | select(.denom=="inj") | .amount' ; }

echo "owner=${OWNER} bob=${BOB} name=${NAME} repo=${REPO}"

echo; echo "== username register =="
igit username register "${NAME}" && ok "register" || bad "register"
igit username show "${NAME}" | grep -q "${OWNER}" && ok "resolve name->addr" || bad "resolve name->addr"
igit username show "${OWNER}" | grep -q "${NAME}" && ok "reverse addr->name" || bad "reverse addr->name"

echo; echo "== repo via username URL =="
igit init "${REPO}" "v3 smoke repo" || bad "igit init"
sleep 2
W=$(mktemp -d); cd "$W"
git init -q && git config user.email v3@e.c && git config user.name v3
echo v3 > f && git add f && git commit -qm v3
git remote add inj "inj://${NAME}/${REPO}"      # username, not address!
git push inj main 2>&1 | grep -q "new branch" && ok "push via username URL" || bad "push via username URL"
git clone -q "inj://${NAME}/${REPO}" "${W}/c1" 2>/dev/null && ok "clone via username URL" || bad "clone via username URL"

echo; echo "== revenue splits =="
igit splits set "${REPO}" "${BOB}:2000" && ok "splits set (bob 20%)" || bad "splits set"
igit splits show "${NAME}" "${REPO}" | grep -q "20.0%" && ok "splits show" || bad "splits show"

echo; echo "== sponsor 0.05 INJ =="
B0=$(inj_balance "${BOB}"); B0=${B0:-0}
igit sponsor "${NAME}" "${REPO}" 0.05 "v3 smoke sponsorship" && ok "sponsor tx" || bad "sponsor tx"
sleep 3
B1=$(inj_balance "${BOB}"); B1=${B1:-0}
# post-fee 3%: 0.05 INJ -> distributable 4.85e16, bob 20% = 9.7e15
EXPECT=9700000000000000
GOT=$((B1 - B0))
if [ "${GOT}" = "${EXPECT}" ]; then ok "bob received exact 20% post-fee share (${GOT})"; else bad "bob share wrong: got ${GOT}, want ${EXPECT}"; fi

echo; echo "== sponsor totals on chain =="
CONTRACT=$(jq -r .contract_address /root/.igit/config.json)
TOT=$(injectived query wasm contract-state smart "${CONTRACT}" "{\"sponsor_totals\":{\"owner\":\"${OWNER}\",\"repo\":\"${REPO}\"}}" --node https://testnet.sentry.tm.injective.network:443 --output json | jq -r '.data.totals[0].amount')
[ "${TOT}" = "50000000000000000" ] && ok "sponsor totals recorded (${TOT})" || bad "sponsor totals wrong: ${TOT}"

echo; echo "== username release =="
igit username release && ok "release" || bad "release"
if git clone -q "inj://${NAME}/${REPO}" "${W}/c2" 2>/dev/null; then bad "clone via released name should fail"; else ok "released name no longer resolves"; fi

echo; echo "PASS=${PASS} FAIL=${FAIL}"
[ "${FAIL}" = 0 ]
