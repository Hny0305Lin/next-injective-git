#!/usr/bin/env bash
# Root-only forced command for the US archive sync account on the HK host.
set -euo pipefail

target=/var/lib/igit/durable-cids.list
tmp=$(mktemp /var/lib/igit/.durable-cids.XXXXXX)
trap 'rm -f "$tmp" "$tmp.sorted"' EXIT
cat > "$tmp"
test -s "$tmp"
grep -Ev '^b[a-z2-7]+$' "$tmp" >/dev/null && {
    echo "invalid CID list" >&2
    exit 1
}
sort -u "$tmp" > "$tmp.sorted"
install -o root -g root -m 600 "$tmp.sorted" "$target"
