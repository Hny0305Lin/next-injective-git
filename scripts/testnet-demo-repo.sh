#!/usr/bin/env bash
# Create a demo repo with README + code + multiple commits for web UI testing.
set -euo pipefail
REPO="demo-showcase"
OWNER=$(injectived keys show igit-dev -a --keyring-backend test)

igit init "${REPO}" "igit demo: README, code, history" || true
sleep 3

W=$(mktemp -d); cd "$W"
git init -q && git config user.email demo@igit.dev && git config user.name "igit demo"

cat > README.md <<'EOF'
# demo-showcase

A tiny repository living **entirely on-chain**: refs on [Injective](https://injective.com),
packfiles on IPFS.

## Features

- `git push` / `git clone` via the `inj://` remote helper
- Sponsorships with instant revenue splits
- Username registry — clone as `inj://alice/demo-showcase`

## Quick start

```bash
igit init demo "hello chain"
git remote add inj inj://<owner>/demo
git push inj main
```

> Decentralized code hosting without giving up the git workflow.
EOF

cat > main.go <<'EOF'
package main

import "fmt"

func main() {
	fmt.Println("hello from the blockchain")
}
EOF
git add . && git commit -qm "feat: initial demo with README and main.go"
git remote add inj "inj://${OWNER}/${REPO}"
git push inj main

mkdir -p src
cat > src/lib.rs <<'EOF'
/// Adds two numbers. Deployed via git push, stored on IPFS.
pub fn add(a: u64, b: u64) -> u64 {
    a + b
}

#[cfg(test)]
mod tests {
    #[test]
    fn adds() {
        assert_eq!(super::add(2, 2), 4);
    }
}
EOF
sed -i 's/hello from the blockchain/hello from Injective/' main.go
git add . && git commit -qm "feat: add rust lib, tweak greeting"
git push inj main

echo "greetings > /dev/chain" >> README.md
git add . && git commit -qm "docs: extend README"
git push inj main

echo "done: inj://${OWNER}/${REPO}"
