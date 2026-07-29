#!/usr/bin/env bash
# Debug: what exactly does a push against a frozen repo print?
set -uo pipefail
OWNER=$(injectived keys show igit-dev -a --keyring-backend test)
REPO="e2e-1785271048"

igit mod "${OWNER}" "${REPO}" frozen debug-run
sleep 4

W=$(mktemp -d); cd "$W"
git init -q && git config user.email d@e.c && git config user.name d
echo y > f && git add f && git commit -qm dbg
git remote add inj "inj://${OWNER}/${REPO}"
echo "----- push output begin -----"
git push inj main:refs/heads/dbg-frozen 2>&1
echo "----- push output end (exit=$?) -----"

igit mod "${OWNER}" "${REPO}" active
