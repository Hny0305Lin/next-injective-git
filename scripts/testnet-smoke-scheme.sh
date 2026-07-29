#!/usr/bin/env bash
# Verify the igit:// scheme + igit push/clone wrappers, and inj:// compat.
set -uo pipefail
OWNER=$(injectived keys show igit-dev -a --keyring-backend test)
REPO="scheme-$(date +%s)"
PASS=0; FAIL=0
ok()  { echo "PASS: $*"; PASS=$((PASS+1)); }
bad() { echo "FAIL: $*"; FAIL=$((FAIL+1)); }

git config --global init.defaultBranch main >/dev/null 2>&1
git config --global user.email s@e.c >/dev/null 2>&1
git config --global user.name scheme >/dev/null 2>&1

igit init "${REPO}" "scheme test" >/dev/null && ok "igit init" || bad "igit init"
sleep 4

W=$(mktemp -d); cd "$W"
git init -q && echo hello > f && git add f && git commit -qm c1

echo "== igit push (wrapper) with igit:// remote =="
git remote add inj "igit://${OWNER}/${REPO}"
if igit push inj main 2>&1 | grep -qE 'new branch|main -> main'; then ok "igit push via igit://"; else bad "igit push via igit://"; fi
sleep 3

echo "== igit clone (wrapper, bare owner/repo) =="
cd "$W"
igit clone "${OWNER}/${REPO}" c-bare >/dev/null 2>&1
[ -f "$W/c-bare/f" ] && ok "igit clone bare owner/repo" || bad "igit clone bare owner/repo"

echo "== igit clone with explicit igit:// URL =="
if igit clone "igit://${OWNER}/${REPO}" c-igit >/dev/null 2>&1 && [ -f "$W/c-igit/f" ]; then ok "igit clone igit://"; else bad "igit clone igit://"; fi

echo "== backward compat: plain git clone inj:// (git-remote-inj) =="
if git clone -q "inj://${OWNER}/${REPO}" c-inj >/dev/null 2>&1 && [ -f "$W/c-inj/f" ]; then ok "git clone inj:// compat"; else bad "git clone inj:// compat"; fi

echo "== backward compat: plain git clone igit:// (git-remote-igit) =="
if git clone -q "igit://${OWNER}/${REPO}" c-igit2 >/dev/null 2>&1 && [ -f "$W/c-igit2/f" ]; then ok "git clone igit:// compat"; else bad "git clone igit:// compat"; fi

echo; echo "PASS=${PASS} FAIL=${FAIL}"
[ "${FAIL}" = 0 ]
