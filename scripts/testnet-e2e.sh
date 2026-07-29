#!/usr/bin/env bash
# End-to-end test against Injective testnet + local Kubo.
# Requires: igit + git-remote-inj installed, ipfs daemon running,
#           igit config with contract_address/key_name, funded key.
set -uo pipefail

WORK=$(mktemp -d /tmp/igit-e2e.XXXX)
REPO="e2e-$(date +%s)"
PASS=0
FAIL=0

step() { echo; echo "===== $* ====="; }
ok()   { echo "PASS: $*"; PASS=$((PASS+1)); }
bad()  { echo "FAIL: $*"; FAIL=$((FAIL+1)); }

git config --global user.email >/dev/null 2>&1 || git config --global user.email e2e@example.com
git config --global user.name  >/dev/null 2>&1 || git config --global user.name  igit-e2e
git config --global init.defaultBranch main

OWNER=$(injectived keys show igit-dev -a --keyring-backend test)
URL="inj://${OWNER}/${REPO}"
echo "owner=${OWNER} repo=${REPO}"

step "1. igit init (CreateRepo on chain)"
if igit init "${REPO}" "e2e test repo"; then ok "igit init"; else bad "igit init"; fi
sleep 4

step "2. first push (pack -> IPFS -> update_ref)"
mkdir -p "${WORK}/src" && cd "${WORK}/src"
git init -q
echo "hello injective git" > README.md
git add . && git commit -qm "c1: initial"
git remote add inj "${URL}"
if git push inj main 2>&1; then ok "first push"; else bad "first push"; fi
sleep 4

step "3. clone into fresh dir"
if git clone -q "${URL}" "${WORK}/clone1" 2>&1; then
  if diff -q "${WORK}/src/README.md" "${WORK}/clone1/README.md" >/dev/null; then
    ok "clone content matches"
  else
    bad "clone content mismatch"
  fi
else
  bad "clone failed"
fi

step "4. incremental push (2nd commit appends CID)"
cd "${WORK}/src"
echo "line two" >> README.md
git add . && git commit -qm "c2: increment"
if git push inj main 2>&1; then ok "incremental push"; else bad "incremental push"; fi
sleep 4
CIDS=$(igit refs "${OWNER}" "${REPO}" | grep 'refs/heads/main' | grep -o 'packfiles:[0-9]*' | cut -d: -f2)
if [ "${CIDS:-0}" -ge 2 ]; then ok "CID list appended (packfiles=${CIDS})"; else bad "CID list not appended (packfiles=${CIDS:-?})"; fi

step "5. pull into clone1 (incremental fetch)"
cd "${WORK}/clone1"
if git pull -q inj main 2>&1 || git pull -q origin main 2>&1; then
  grep -q "line two" README.md && ok "incremental fetch" || bad "pulled but content missing"
else
  bad "pull failed"
fi

step "6. stale push rejected (optimistic concurrency)"
cd "${WORK}/src"
git reset -q --hard HEAD~1          # locally forget c2 -> stale view
echo "conflicting" >> README.md
git add . && git commit -qm "c3: conflicting from stale tip"
if git push inj main 2>&1; then
  bad "stale push was accepted (should be rejected)"
else
  ok "stale push rejected"
fi
sleep 2

step "7. force push replaces history"
if git push -f inj main 2>&1; then ok "force push"; else bad "force push"; fi
sleep 4

step "8. fresh clone reflects forced history"
if git clone -q "${URL}" "${WORK}/clone2" 2>&1; then
  if grep -q "conflicting" "${WORK}/clone2/README.md" && ! grep -q "line two" "${WORK}/clone2/README.md"; then
    ok "forced history cloned"
  else
    bad "clone2 content unexpected"
  fi
else
  bad "clone2 failed"
fi

step "9. branch + tag push"
cd "${WORK}/src"
git checkout -qb feature/x
echo "feat" > feat.txt
git add . && git commit -qm "c4: feature"
git tag v0.0.1
R1=0; git push inj feature/x 2>&1 && R1=1
sleep 4
R2=0; git push inj v0.0.1 2>&1 && R2=1
sleep 4
[ "$R1" = 1 ] && ok "branch push" || bad "branch push"
[ "$R2" = 1 ] && ok "tag push" || bad "tag push"
igit refs "${OWNER}" "${REPO}"

step "10. delete remote branch"
if git push inj :feature/x 2>&1; then ok "delete ref"; else bad "delete ref"; fi
sleep 4
if igit refs "${OWNER}" "${REPO}" | grep -q 'feature/x'; then bad "ref still on chain"; else ok "ref gone on chain"; fi

echo
echo "==================== E2E RESULT ===================="
echo "repo: ${URL}"
echo "PASS=${PASS} FAIL=${FAIL}"
echo "workdir: ${WORK} (kept for inspection)"
[ "${FAIL}" = 0 ]
