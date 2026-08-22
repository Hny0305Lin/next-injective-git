#!/usr/bin/env bash
# Offline regression fixture for v1-export-snapshot.sh. The fake LCD forces
# pagination and verifies that every smart query is pinned to one height.
set -euo pipefail

ARCHIVE_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"

cat > "$TMP/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
url=""
for arg in "$@"; do
  [[ "$arg" == http://* || "$arg" == https://* ]] && url="$arg"
done
printf '%s\n' "$*" >> "${FAKE_CURL_LOG:?}"

if [[ "$url" == */cosmos/base/tendermint/v1beta1/blocks/latest ]]; then
  printf '%s\n' '{"block_id":{"hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"block":{"header":{"chain_id":"injective-888","height":"4242"}}}'
  exit 0
fi
if [[ "$url" == */cosmos/base/tendermint/v1beta1/blocks/4242 ]]; then
  printf '%s\n' '{"block_id":{"hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},"block":{"header":{"chain_id":"injective-888","height":"4242"}}}'
  exit 0
fi

encoded=${url##*/}
encoded=$(printf '%b' "${encoded//%/\\x}")
message=$(printf '%s' "$encoded" | base64 -d)
kind=$(jq -r 'keys[0]' <<<"$message")
start=$(jq -r ".${kind}.start_after // empty" <<<"$message")
repo=$(jq -r ".${kind}.repo // empty" <<<"$message")

mode=normal
if [[ -v FAKE_CURL_MODE ]]; then
  mode="$FAKE_CURL_MODE"
fi
if [[ "$mode" == "malformed-json" && "$kind" == "config" ]]; then
  printf '%s\n' '{"data":'
  exit 0
fi

repo_item() {
  jq -cn --arg name "$1" '{owner:"inj1owner",name:$name,description:("repo "+$name),default_branch:"main",created_at:1,updated_at:2,moderation_status:"active",forked_from:null}'
}
badge_item() {
  jq -cn --argjson id "$1" '{id:$id,repo_owner:"inj1owner",repo_name:"alpha",recipient:"inj1recipient",reason:"fixture",awarded_by:"inj1owner",awarded_at:3}'
}

case "$kind" in
  config)
    printf '%s\n' '{"data":{"admin":"inj1admin","moderation_committee":null,"treasury":"inj1treasury","platform_fee_bps":300,"username_deposit":{"denom":"inj","amount":"1"},"username_fee":{"denom":"inj","amount":"1"},"reserved_usernames":[]}}'
    ;;
  upgrade_security)
    printf '%s\n' '{"data":{"proposal":null,"timelock_seconds":1209600}}'
    ;;
  list_repos)
    if [[ -z "$start" ]]; then
      a=$(repo_item alpha); b=$(repo_item beta)
      jq -cn --argjson a "$a" --argjson b "$b" '{data:{repos:[$a,$b]}}'
    elif [[ "$mode" == "repo-stall" ]]; then
      a=$(repo_item alpha); b=$(repo_item beta)
      jq -cn --argjson a "$a" --argjson b "$b" '{data:{repos:[$a,$b]}}'
    else
      c=$(repo_item gamma)
      jq -cn --argjson c "$c" '{data:{repos:[$c]}}'
    fi
    ;;
  list_refs)
    if [[ "$repo" != alpha ]]; then
      printf '%s\n' '{"data":{"refs":[]}}'
    elif [[ -z "$start" ]]; then
      printf '%s\n' '{"data":{"refs":[{"ref_name":"refs/heads/a","commit_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","pack_uris":["ipfs://a"],"updated_at":1,"updated_by":"inj1owner"},{"ref_name":"refs/heads/b","commit_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","pack_uris":["ipfs://b"],"updated_at":2,"updated_by":"inj1owner"}]}}'
    elif [[ "$mode" == "ref-stall" ]]; then
      printf '%s\n' '{"data":{"refs":[{"ref_name":"refs/heads/a","commit_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","pack_uris":["ipfs://a"],"updated_at":1,"updated_by":"inj1owner"},{"ref_name":"refs/heads/b","commit_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","pack_uris":["ipfs://b"],"updated_at":2,"updated_by":"inj1owner"}]}}'
    else
      printf '%s\n' '{"data":{"refs":[{"ref_name":"refs/heads/c","commit_sha":"cccccccccccccccccccccccccccccccccccccccc","pack_uris":["ipfs://c"],"updated_at":3,"updated_by":"inj1owner"}]}}'
    fi
    ;;
  list_collaborators)
    if [[ "$repo" != alpha ]]; then
      printf '%s\n' '{"data":{"collaborators":[]}}'
    elif [[ -z "$start" ]]; then
      printf '%s\n' '{"data":{"collaborators":[{"address":"inj1a","role":"maintainer"},{"address":"inj1b","role":"reader"}]}}'
    else
      printf '%s\n' '{"data":{"collaborators":[{"address":"inj1c","role":"reader"}]}}'
    fi
    ;;
  revenue_splits)
    jq -cn --arg repo "$repo" '{data:{owner:"inj1owner",repo:$repo,splits:[]}}'
    ;;
  sponsor_totals)
    jq -cn --arg repo "$repo" '{data:{owner:"inj1owner",repo:$repo,totals:[]}}'
    ;;
  ownership_security)
    printf '%s\n' '{"data":{"transfer":null,"recovery":null,"guardians":[],"guardian_threshold":0}}'
    ;;
  badges_by_repo)
    if [[ "$repo" != alpha ]]; then
      printf '%s\n' '{"data":{"badges":[]}}'
    elif [[ -z "$start" ]]; then
      a=$(badge_item 1); b=$(badge_item 2)
      jq -cn --argjson a "$a" --argjson b "$b" '{data:{badges:[$a,$b]}}'
    else
      c=$(badge_item 3)
      jq -cn --argjson c "$c" '{data:{badges:[$c]}}'
    fi
    ;;
  moderation_report)
    printf '%s\n' '{"data":{"id":7,"owner":"inj1owner","repo":"alpha","reporter":"inj1reporter","reason_hash":"hash","status":"open","resolution":null,"resolution_hash":null,"appeal_hash":null,"created_at":1,"updated_at":1}}'
    ;;
  resolve_username)
    printf '%s\n' '{"data":{"name":"alice","owner":"inj1owner","registered_at":1}}'
    ;;
  badges_by_recipient)
    if [[ -z "$start" ]]; then
      a=$(badge_item 1); b=$(badge_item 2)
      jq -cn --argjson a "$a" --argjson b "$b" '{data:{badges:[$a,$b]}}'
    else
      c=$(badge_item 3)
      jq -cn --argjson c "$c" '{data:{badges:[$c]}}'
    fi
    ;;
  release_artifacts)
    printf '%s\n' '{"data":{"version":"v1.0.0","artifacts":[{"version":"v1.0.0","platform":"linux-amd64","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","registered_by":"inj1admin","registered_at":1}]}}'
    ;;
  *) echo "unexpected query: $message" >&2; exit 1 ;;
esac
EOF
chmod 700 "$TMP/bin/curl"

printf '%s\n' '# owners' inj1owner > "$TMP/owners.txt"
printf '%s\n' 7 > "$TMP/reports.txt"
printf '%s\n' alice > "$TMP/usernames.txt"
printf '%s\n' inj1recipient > "$TMP/recipients.txt"
printf '%s\n' v1.0.0 > "$TMP/releases.txt"

export FAKE_CURL_LOG="$TMP/curl.log"
run_export() {
  local output="$1" hash_output="$2" owners="$TMP/owners.txt"
  shift 2
  if [[ -v IGIT_TEST_OWNERS_FILE ]]; then
    owners="$IGIT_TEST_OWNERS_FILE"
  fi
  PATH="$TMP/bin:$PATH" IGIT_SNAPSHOT_PAGE_LIMIT=2 \
    bash "$ARCHIVE_ROOT/scripts/v1-export-snapshot.sh" \
      --lcd https://lcd.example.invalid \
      --contract inj1contract \
      --owners-file "$owners" \
      --reports-file "$TMP/reports.txt" \
      --usernames-file "$TMP/usernames.txt" \
      --badge-recipients-file "$TMP/recipients.txt" \
      --releases-file "$TMP/releases.txt" \
      --output "$output" \
      --hash-output "$hash_output" \
      --exported-at 2026-08-11T00:00:00Z \
      "$@"
}

validate_snapshot() {
  bash "$ARCHIVE_ROOT/scripts/v1-export-snapshot.sh" --validate "$1" --hash-file "$2"
}

expect_failure() {
  local label="$1" needle="$2" log="$TMP/$1.log"
  shift 2
  if "$@" >"$log" 2>&1; then
    echo "FAIL: $label unexpectedly succeeded" >&2
    cat "$log" >&2
    exit 1
  fi
  if [[ -n "$needle" ]] && ! grep -Fq "$needle" "$log"; then
    echo "FAIL: $label did not report '$needle'" >&2
    cat "$log" >&2
    exit 1
  fi
}

write_hash() {
  local snapshot="$1" hash_output="$2" digest
  digest="$(sha256sum -- "$snapshot" | awk '{print $1}')"
  printf '%s  %s\n' "$digest" "$(basename "$snapshot")" >"$hash_output"
}

run_export "$TMP/snapshot.json" "$TMP/snapshot.sha256"

jq -e '
  .schema == "igit.cosmwasm-v1.snapshot.v1" and
  .source.chain_id == "injective-888" and
  .source.height == "4242" and
  .source.block_hash == "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" and
  (.repositories | length) == 3 and
  ([.refs[] | select(.repo == "alpha")][0].refs | length) == 3 and
  ([.collaborators[] | select(.repo == "alpha")][0].collaborators | length) == 3 and
  ([.repo_extensions[] | select(.repo == "alpha")][0].badges | length) == 3 and
  (.moderation_reports | length) == 1 and
  (.usernames | length) == 1 and
  (.badges_by_recipient[0].badges | length) == 3 and
  (.releases[0].artifacts | length) == 1
' "$TMP/snapshot.json" >/dev/null

grep -q 'x-cosmos-block-height: 4242' "$TMP/curl.log"
grep -q '%3D' "$TMP/curl.log"
(cd "$TMP" && sha256sum --check snapshot.sha256 >/dev/null)

run_export "$TMP/snapshot-repeat.json" "$TMP/snapshot-repeat.sha256"
cmp -s "$TMP/snapshot.json" "$TMP/snapshot-repeat.json"
test "$(awk 'NR == 1 { print $1 }' "$TMP/snapshot.sha256")" = \
  "$(awk 'NR == 1 { print $1 }' "$TMP/snapshot-repeat.sha256")"
offline_curl_lines="$(wc -l <"$TMP/curl.log")"
PATH="$TMP/bin:$PATH" validate_snapshot "$TMP/snapshot.json" "$TMP/snapshot.sha256" >/dev/null
test "$offline_curl_lines" = "$(wc -l <"$TMP/curl.log")"

jq -S 'del(.source.height)' "$TMP/snapshot.json" >"$TMP/bad-schema.json"
write_hash "$TMP/bad-schema.json" "$TMP/bad-schema.json.sha256"
expect_failure bad-schema "top-level schema and required fields" \
  validate_snapshot "$TMP/bad-schema.json" "$TMP/bad-schema.json.sha256"

jq -S 'del(.refs[0])' "$TMP/snapshot.json" >"$TMP/missing-ref-group.json"
write_hash "$TMP/missing-ref-group.json" "$TMP/missing-ref-group.json.sha256"
expect_failure missing-ref-group "repository group references" \
  validate_snapshot "$TMP/missing-ref-group.json" "$TMP/missing-ref-group.json.sha256"

jq -S '.repositories += [.repositories[0]]' "$TMP/snapshot.json" >"$TMP/duplicate-repo.json"
write_hash "$TMP/duplicate-repo.json" "$TMP/duplicate-repo.json.sha256"
expect_failure duplicate-repo "repository and ref duplicate detection" \
  validate_snapshot "$TMP/duplicate-repo.json" "$TMP/duplicate-repo.json.sha256"

jq -S '.refs[0].refs += [.refs[0].refs[0]]' "$TMP/snapshot.json" >"$TMP/duplicate-ref.json"
write_hash "$TMP/duplicate-ref.json" "$TMP/duplicate-ref.json.sha256"
expect_failure duplicate-ref "repository and ref duplicate detection" \
  validate_snapshot "$TMP/duplicate-ref.json" "$TMP/duplicate-ref.json.sha256"

jq -S 'del(.refs[0].refs[0].commit_sha)' "$TMP/snapshot.json" >"$TMP/bad-field.json"
write_hash "$TMP/bad-field.json" "$TMP/bad-field.json.sha256"
expect_failure bad-field "repository and ref field completeness" \
  validate_snapshot "$TMP/bad-field.json" "$TMP/bad-field.json.sha256"

jq -S 'del(.repositories[0].forked_from)' "$TMP/snapshot.json" >"$TMP/missing-nullable-field.json"
write_hash "$TMP/missing-nullable-field.json" "$TMP/missing-nullable-field.json.sha256"
expect_failure missing-nullable-field "repository and ref field completeness" \
  validate_snapshot "$TMP/missing-nullable-field.json" "$TMP/missing-nullable-field.json.sha256"

printf '{"schema":' >"$TMP/bad-json.json"
write_hash "$TMP/bad-json.json" "$TMP/bad-json.json.sha256"
expect_failure bad-json "snapshot is not valid JSON" \
  validate_snapshot "$TMP/bad-json.json" "$TMP/bad-json.json.sha256"

cp "$TMP/snapshot.json" "$TMP/tampered.json"
jq -S '.source.chain_id = "injective-999"' "$TMP/tampered.json" >"$TMP/tampered.tmp"
mv "$TMP/tampered.tmp" "$TMP/tampered.json"
printf '%s  tampered.json\n' "$(awk 'NR == 1 { print $1 }' "$TMP/snapshot.sha256")" >"$TMP/tampered.json.sha256"
expect_failure hash-mismatch "snapshot hash mismatch" \
  validate_snapshot "$TMP/tampered.json" "$TMP/tampered.json.sha256"

curl_lines_before="$(wc -l <"$TMP/curl.log")"
printf '%s\n' inj1owner inj1owner >"$TMP/duplicate-owners.txt"
IGIT_TEST_OWNERS_FILE="$TMP/duplicate-owners.txt" \
  expect_failure duplicate-input "duplicate values" run_export "$TMP/duplicate-input.json" "$TMP/duplicate-input.sha256"

printf '%s\n' 'inj1owner extra-column' >"$TMP/ambiguous-owners.txt"
IGIT_TEST_OWNERS_FILE="$TMP/ambiguous-owners.txt" \
  expect_failure ambiguous-input "exactly one value per line" run_export "$TMP/ambiguous-input.json" "$TMP/ambiguous-input.sha256"
printf '%s\n' '# no owners' >"$TMP/empty-owners.txt"
IGIT_TEST_OWNERS_FILE="$TMP/empty-owners.txt" \
  expect_failure empty-input "contains no values" run_export "$TMP/empty-input.json" "$TMP/empty-input.sha256"
expect_failure aliased-output "snapshot and hash output must differ" \
  run_export "$TMP/aliased-output.json" "$TMP/./aliased-output.json"
test "$curl_lines_before" = "$(wc -l <"$TMP/curl.log")"
test ! -e "$TMP/duplicate-input.json"
test ! -e "$TMP/ambiguous-input.json"
test ! -e "$TMP/empty-input.json"

printf 'sentinel snapshot\n' >"$TMP/stalled.json"
printf 'sentinel hash\n' >"$TMP/stalled.json.sha256"
export FAKE_CURL_MODE=repo-stall
expect_failure pagination-stall "pagination did not advance" \
  run_export "$TMP/stalled.json" "$TMP/stalled.json.sha256"
unset FAKE_CURL_MODE
grep -Fxq 'sentinel snapshot' "$TMP/stalled.json"
grep -Fxq 'sentinel hash' "$TMP/stalled.json.sha256"

printf 'sentinel ref snapshot\n' >"$TMP/ref-stalled.json"
printf 'sentinel ref hash\n' >"$TMP/ref-stalled.json.sha256"
export FAKE_CURL_MODE=ref-stall
expect_failure ref-pagination-stall "ref pagination did not advance" \
  run_export "$TMP/ref-stalled.json" "$TMP/ref-stalled.json.sha256"
unset FAKE_CURL_MODE
grep -Fxq 'sentinel ref snapshot' "$TMP/ref-stalled.json"
grep -Fxq 'sentinel ref hash' "$TMP/ref-stalled.json.sha256"

printf 'sentinel malformed\n' >"$TMP/malformed.json"
printf 'sentinel malformed hash\n' >"$TMP/malformed.json.sha256"
export FAKE_CURL_MODE=malformed-json
expect_failure malformed-response "LCD smart query response is invalid or missing data" \
  run_export "$TMP/malformed.json" "$TMP/malformed.json.sha256"
unset FAKE_CURL_MODE
grep -Fxq 'sentinel malformed' "$TMP/malformed.json"
grep -Fxq 'sentinel malformed hash' "$TMP/malformed.json.sha256"

echo "PASS: V1 snapshot schema, fields, pagination, duplicate rejection, deterministic hash, and fail-closed errors"
