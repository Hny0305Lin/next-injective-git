#!/usr/bin/env bash
set -euo pipefail

release_dir=${1:?usage: verify-release-assets.sh RELEASE_DIR VERSION [IGIT_BINARY]}
expected_version=${2:?usage: verify-release-assets.sh RELEASE_DIR VERSION [IGIT_BINARY]}
version_binary=${3:-"$release_dir/igit-linux-amd64"}

assets=(
  igit-linux-amd64
  igit-linux-arm64
  igit-darwin-amd64
  igit-darwin-arm64
  igit-windows-amd64.exe
  git-remote-igit-linux-amd64
  git-remote-igit-linux-arm64
  git-remote-igit-darwin-amd64
  git-remote-igit-darwin-arm64
  git-remote-igit-windows-amd64.exe
  repo-registry.wasm
)

for asset in "${assets[@]}"; do
  if [[ ! -s "$release_dir/$asset" ]]; then
    echo "missing or empty release asset: $asset" >&2
    exit 1
  fi
done

if [[ ! -f "$version_binary" ]]; then
  echo "version validation binary does not exist: $version_binary" >&2
  exit 1
fi
actual_version=$("$version_binary" version)
expected_output="igit $expected_version"
if [[ "$actual_version" != "$expected_output" ]]; then
  echo "version output = $actual_version; want $expected_output" >&2
  exit 1
fi

wasm_magic=$(od -An -tx1 -N4 "$release_dir/repo-registry.wasm" | tr -d '[:space:]')
if [[ "$wasm_magic" != "0061736d" ]]; then
  echo "unexpected wasm magic: $wasm_magic" >&2
  exit 1
fi

checksums="$release_dir/checksums.txt"
if [[ ! -s "$checksums" ]]; then
  echo "missing or empty checksum file" >&2
  exit 1
fi
expected_checksums=$(cd "$release_dir" && LC_ALL=C sha256sum -- "${assets[@]}")
actual_checksums=$(<"$checksums")
if [[ "$actual_checksums" != "$expected_checksums" ]]; then
  echo "checksums.txt does not match the release assets" >&2
  diff -u <(printf '%s\n' "$expected_checksums") <(printf '%s\n' "$actual_checksums") >&2 || true
  exit 1
fi

(cd "$release_dir" && sha256sum --check --strict checksums.txt)
