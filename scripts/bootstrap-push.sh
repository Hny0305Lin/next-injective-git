#!/usr/bin/env bash
# Build the igit CLI from this checkout, then install its pinned push runtime.
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
CLI_DIR="$REPO_ROOT/cli"
BIN_DIR="${HOME}/.local/bin"

if [[ ! -f "$CLI_DIR/go.mod" ]]; then
  echo "cannot find cli/go.mod next to this bootstrap script" >&2
  exit 1
fi

run_root() {
  if [[ $(id -u) -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    echo "missing required command and sudo is unavailable: $*" >&2
    exit 1
  fi
}

if ! command -v git >/dev/null 2>&1 || ! command -v go >/dev/null 2>&1; then
  if ! command -v apt-get >/dev/null 2>&1; then
    echo "Git and Go 1.22+ are required; automatic package installation currently supports apt-based Linux only" >&2
    exit 1
  fi
  echo "Installing Git, Go, and CA certificates..."
  run_root apt-get update
  run_root apt-get install -y --no-install-recommends git golang-go ca-certificates
fi

go_version=$(go env GOVERSION | sed 's/^go//')
go_major=${go_version%%.*}
go_rest=${go_version#*.}
go_minor=${go_rest%%.*}
if (( go_major < 1 || (go_major == 1 && go_minor < 22) )); then
  echo "Go 1.22+ is required; found $(go version)" >&2
  exit 1
fi

mkdir -p "$BIN_DIR"
echo "Building igit and git-remote-igit from $CLI_DIR..."
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
(
  cd "$CLI_DIR"
  GOBIN="$BIN_DIR" go install ./cmd/igit ./cmd/git-remote-igit
)

export PATH="$BIN_DIR:$HOME/.igit/bin:$PATH"
profile_line='export PATH="$HOME/.local/bin:$HOME/.igit/bin:$PATH"'
touch "$HOME/.profile"
if ! grep -Fqx "$profile_line" "$HOME/.profile"; then
  printf '\n%s\n' "$profile_line" >> "$HOME/.profile"
fi

echo "CLI installed: $(igit version)"
exec igit setup push "$@"
