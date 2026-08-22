#!/usr/bin/env bash
# Push only verified US archive CIDs to the HK hot-tier safety allowlist.
set -euo pipefail

ARCHIVED_STATE="${ARCHIVED_STATE:-/var/lib/igit-archive/archived.tsv}"
SSH_KEY="${SSH_KEY:-/etc/igit/archive-sync.key}"
KNOWN_HOSTS="${KNOWN_HOSTS:-/etc/igit/archive-sync.known_hosts}"
HK_TARGET="${HK_TARGET:-igit-sync@45.202.249.80}"

test -s "$ARCHIVED_STATE"
awk -F '\t' '{print $1}' "$ARCHIVED_STATE" | sort -u \
    | ssh -T -i "$SSH_KEY" -o BatchMode=yes -o StrictHostKeyChecking=yes \
        -o UserKnownHostsFile="$KNOWN_HOSTS" "$HK_TARGET"
