#!/usr/bin/env bash
set -euo pipefail

release_dir=${1:?usage: register-release.sh RELEASE_DIR VERSION [IGIT_BINARY]}
version=${2:?usage: register-release.sh RELEASE_DIR VERSION [IGIT_BINARY]}
igit_bin=${3:-igit}

test -d "$release_dir"
test -f "$release_dir/checksums.txt"

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

# Parse the checksum file into a fixed-name map. Do not let an administrator
# accidentally register arbitrary files or a path containing `../`.
declare -A declared=()
while read -r digest artifact extra; do
  test -n "${digest:-}" && test -n "${artifact:-}" && test -z "${extra:-}"
  [[ "$digest" =~ ^[[:xdigit:]]{64}$ ]]
  [[ "$artifact" != */* && "$artifact" != *\\* ]]
  test -z "${declared[$artifact]+set}"
  declared["$artifact"]="${digest,,}"
done < "$release_dir/checksums.txt"

specs=()
for artifact in "${assets[@]}"; do
  digest=${declared[$artifact]-}
  test -n "$digest"
  test -f "$release_dir/$artifact"
  actual=$(sha256sum -- "$release_dir/$artifact" | awk '{print $1}')
  test "$actual" = "$digest"
  specs+=("$artifact=$digest")
done

test "${#declared[@]}" -eq "${#assets[@]}"
"$igit_bin" release register "$version" "${specs[@]}"

echo "registered ${#specs[@]} release artifacts for $version"
