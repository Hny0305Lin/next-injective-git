#!/usr/bin/env bash
# Build/install igit + helper, then smoke-test `igit import` and confirm
# the legacy inj:// scheme is gone.
set -uo pipefail
cd /mnt/d/next-injective-git/cli
go build -o /usr/local/bin/igit ./cmd/igit || exit 1
go build -o /usr/local/bin/git-remote-igit ./cmd/git-remote-igit || exit 1
rm -f /usr/local/bin/git-remote-inj    # legacy helper removed
echo "BUILT; helpers:"; ls /usr/local/bin/git-remote-* 2>/dev/null

OWNER=$(injectived keys show igit-dev -a --keyring-backend test)
NAME="hello-world-$(date +%s | tail -c 5)"
PASS=0; FAIL=0
ok()  { echo "PASS: $*"; PASS=$((PASS+1)); }
bad() { echo "FAIL: $*"; FAIL=$((FAIL+1)); }

echo; echo "== igit import github.com/octocat/Hello-World =="
if igit import github.com/octocat/Hello-World "${NAME}"; then ok "import ran"; else bad "import ran"; fi
sleep 4

echo; echo "== verify refs on chain =="
igit refs "${OWNER}" "${NAME}"
igit refs "${OWNER}" "${NAME}" | grep -q 'refs/heads/master' && ok "master branch on chain" || bad "master branch missing"

echo; echo "== clone the mirror back =="
W=$(mktemp -d)
igit clone "${OWNER}/${NAME}" "${W}/m" >/dev/null 2>&1
[ -f "${W}/m/README" ] && ok "clone mirror has README" || bad "clone mirror missing content"

echo; echo "== legacy inj:// rejected =="
if git clone -q "inj://${OWNER}/${NAME}" "${W}/inj" >/dev/null 2>&1; then bad "inj:// still works"; else ok "inj:// rejected"; fi

echo; echo "PASS=${PASS} FAIL=${FAIL}"
[ "${FAIL}" = 0 ]
