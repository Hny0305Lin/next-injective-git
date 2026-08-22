#!/usr/bin/env bash
# Export a canonical, hashable CosmWasm V1 state snapshot at one LCD height.
# This command is read-only: it never loads a signer or broadcasts a tx.
set -euo pipefail

LCD="https://testnet.sentry.lcd.injective.network:443"
CONTRACT=""
OWNERS_FILE=""
REPORTS_FILE=""
USERNAMES_FILE=""
BADGE_RECIPIENTS_FILE=""
RELEASES_FILE=""
OUTPUT="v1-snapshot.json"
HASH_OUTPUT=""
HEIGHT=""
EXPORTED_AT=""
VALIDATE_FILE=""
VALIDATE_HASH_FILE=""
PAGE_LIMIT="${IGIT_SNAPSHOT_PAGE_LIMIT:-100}"

usage() {
  cat >&2 <<'EOF'
usage: v1-export-snapshot.sh --owners-file owners.txt --contract inj1... [options]
   or: v1-export-snapshot.sh --validate SNAPSHOT [--hash-file FILE]

Required:
  --owners-file FILE             One repository owner address per line.
  --contract ADDRESS             CosmWasm V1 contract address.

Optional enumerations (one value per line):
  --reports-file FILE            Moderation report IDs.
  --usernames-file FILE          Registered usernames.
  --badge-recipients-file FILE   Badge recipient addresses.
  --releases-file FILE           Release versions.

Output/source options:
  --output FILE                  Snapshot JSON (default: v1-snapshot.json).
  --hash-output FILE             SHA-256 record (default: FILE.sha256).
  --lcd URL                      Cosmos LCD endpoint.
  --height HEIGHT                Pin all queries to an explicit block height.
  --exported-at TIMESTAMP        Override export timestamp (UTC RFC3339).

Offline validation:
  --validate FILE                 Validate an existing snapshot without LCD.
  --hash-file FILE                SHA-256 record for --validate (default:
                                  SNAPSHOT.sha256).

Blank lines and lines beginning with # are ignored. Every other line must
contain exactly one value; duplicate values are rejected instead of silently
being collapsed. The owners/enumeration files are necessary because V1 has no
global enumeration query for those keys.
EOF
}

sha256_digest() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    echo "sha256sum or shasum is required to hash the snapshot" >&2
    return 1
  fi
}

# Validate an on-disk snapshot without contacting LCD. Keeping this mode in
# the exporter makes the schema and duplicate-key rules reusable by migration
# operators and by the offline regression fixture.
validate_snapshot() {
  local snapshot="$1" hash_file canonical record
  local expected_digest expected_name actual snapshot_name failed=0
  hash_file="$snapshot.sha256"
  snapshot_name="$(basename -- "$snapshot")"
  if (( $# >= 2 )); then
    hash_file="$2"
  fi
  if (( $# >= 3 )); then
    snapshot_name="$3"
  fi
  [[ -r "$snapshot" ]] || { echo "snapshot is not readable: $snapshot" >&2; return 1; }
  [[ -r "$hash_file" ]] || { echo "hash file is not readable: $hash_file" >&2; return 1; }
  command -v jq >/dev/null 2>&1 || { echo "jq is required for snapshot validation" >&2; return 1; }
  command -v cmp >/dev/null 2>&1 || { echo "cmp is required for snapshot validation" >&2; return 1; }
  command -v awk >/dev/null 2>&1 || { echo "awk is required for snapshot validation" >&2; return 1; }

  if ! jq -e . "$snapshot" >/dev/null 2>&1; then
    echo "snapshot is not valid JSON: $snapshot" >&2
    return 1
  fi

  check() {
    local label="$1" expression="$2"
    if ! jq -e "$expression" "$snapshot" >/dev/null 2>&1; then
      echo "snapshot validation failed: $label" >&2
      failed=1
    fi
  }

  check "top-level schema and required fields" '
    def nonempty: (type == "string") and (try (length > 0) catch false);
    def positive_string: (type == "string") and (try test("^[1-9][0-9]*$") catch false);
    type == "object" and
    .schema == "igit.cosmwasm-v1.snapshot.v1" and
    (.source | type == "object") and
    (.source.lcd | nonempty) and
    (.source.contract | nonempty) and
    (.source.chain_id | nonempty) and
    (.source.height | positive_string) and
    (.source.block_hash | (type == "string" and (try test("^[0-9a-f]{64}$") catch false))) and
    (.source.exported_at | (type == "string" and (try test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$") catch false))) and
    (.contract_config | type == "object") and
    (.contract_config | has("moderation_committee")) and
    (.upgrade_security | type == "object") and
    (.upgrade_security | has("proposal")) and
    (.repositories | type == "array") and
    (.refs | type == "array") and
    (.collaborators | type == "array") and
    (.repo_extensions | type == "array") and
    (.moderation_reports | type == "array") and
    (.usernames | type == "array") and
    (.badges_by_recipient | type == "array") and
    (.releases | type == "array")
  '

  check "contract configuration fields" '
    def nonempty: (type == "string") and (try (length > 0) catch false);
    def uint: (type == "number") and (try ((. >= 0) and ((floor) == .)) catch false);
    def coin: (type == "object") and (.denom | nonempty) and
      (.amount | (type == "string" and (try test("^[0-9]+$") catch false)));
    (.contract_config | type == "object") and
    (.contract_config.admin | nonempty) and
    (.contract_config.treasury | nonempty) and
    ((.contract_config.moderation_committee == null) or (.contract_config.moderation_committee | nonempty)) and
    (.contract_config.platform_fee_bps | uint) and (.contract_config.platform_fee_bps <= 500) and
    (.contract_config.username_deposit | coin) and
    (.contract_config.username_fee | coin) and
    (.contract_config.reserved_usernames | (type == "array" and all(.[]; nonempty)))
  '

  check "upgrade security fields" '
    def uint: (type == "number") and (try ((. >= 0) and ((floor) == .)) catch false);
    def hex64: (type == "string") and (try test("^[0-9a-fA-F]{64}$") catch false);
    (.upgrade_security | type == "object") and
    (.upgrade_security.timelock_seconds | uint) and
    ((.upgrade_security.proposal == null) or
      ((.upgrade_security.proposal | type == "object") and
       (.upgrade_security.proposal.wasm_sha256 | hex64) and
       (.upgrade_security.proposal.proposed_at | uint) and
       (.upgrade_security.proposal.execute_after | uint)))
  '

  check "repository and ref field completeness" '
    def nonempty: (type == "string") and (try (length > 0) catch false);
    def uint: (type == "number") and (try ((. >= 0) and ((floor) == .)) catch false);
    def valid_repo:
      try (type == "object" and (.owner | nonempty) and (.name | nonempty) and
        (.description | type == "string") and (.default_branch | type == "string") and
        (.created_at | uint) and (.updated_at | uint) and
        (.moderation_status | (. == "active" or . == "delisted" or . == "frozen")) and
        has("forked_from") and ((.forked_from == null) or (.forked_from | nonempty))) catch false;
    def valid_ref:
      try (type == "object" and (.ref_name | nonempty) and
        (.commit_sha | (type == "string" and (test("^([0-9a-fA-F]{40}|[0-9a-fA-F]{64})$")))) and
        (.pack_uris | (type == "array" and all(.[]; nonempty and
          (try test("^[a-z0-9]+://.+$") catch false) and (length <= 512)))) and
        (.updated_at | uint) and (.updated_by | nonempty)) catch false;
    (.repositories | all(.[]?; valid_repo)) and
    (.refs | all(.[]?; try (type == "object" and (.owner | nonempty) and (.repo | nonempty) and
      (.refs | (type == "array" and all(.[]; valid_ref)))) catch false))
  '

  check "repository and ref duplicate detection" '
    (.repositories | map([.owner, .name] | join("\u0000")) | length == (unique | length)) and
    (.refs | map([.owner, .repo] | join("\u0000")) | length == (unique | length)) and
    (.refs | all(.[]?; (.refs | map(.ref_name) | length == (unique | length))))
  '

  # jq variables must not expand in Bash.
  # shellcheck disable=SC2016
  check "repository group references" '
    (.repositories | map([.owner, .name] | join("\u0000"))) as $repos |
    (.refs | map([.owner, .repo] | join("\u0000"))) as $refs |
    (.collaborators | map([.owner, .repo] | join("\u0000"))) as $collaborators |
    (.repo_extensions | map([.owner, .repo] | join("\u0000"))) as $extensions |
    (($repos | sort) == ($refs | sort)) and
    (($repos | sort) == ($collaborators | sort)) and
    (($repos | sort) == ($extensions | sort))
  '

  check "collaborator field completeness and duplicates" '
    def nonempty: (type == "string") and (try (length > 0) catch false);
    (.collaborators | all(.[]?; try (
      type == "object" and (.owner | nonempty) and (.repo | nonempty) and
      (.collaborators | (type == "array" and
        all(.[]; type == "object" and (.address | nonempty) and
          (.role == "maintainer" or .role == "reader")) and
        (map(.address) | length == (unique | length))))
    ) catch false))
  '

  check "repo extension field completeness" '
    def nonempty: (type == "string") and (try (length > 0) catch false);
    def uint: (type == "number") and (try ((. >= 0) and ((floor) == .)) catch false);
    def valid_badge:
      type == "object" and (.id | uint) and (.repo_owner | nonempty) and
      (.repo_name | nonempty) and (.recipient | nonempty) and
      (.reason | type == "string") and (.awarded_by | nonempty) and (.awarded_at | uint);
    def delayed:
      . == null or (type == "object" and (.new_owner | nonempty) and
        (.proposed_at | uint) and (.execute_after | uint));
    (.repo_extensions | all(.[]?; try (
      type == "object" and (.owner | nonempty) and (.repo | nonempty) and
      (.revenue_splits | (type == "array" and all(.[]; type == "object" and
        (.address | nonempty) and (.bps | uint) and (.bps <= 10000)))) and
      (.sponsor_totals | (type == "array" and all(.[]; type == "object" and
        (.denom | nonempty) and (.amount | (type == "string" and (try test("^[0-9]+$") catch false)))))) and
      (.ownership_security | type == "object") and
      (.ownership_security | has("transfer") and has("recovery")) and
      (.ownership_security.guardians | (type == "array" and all(.[]; nonempty))) and
      (.ownership_security.guardian_threshold | uint) and
      (.ownership_security.transfer | delayed) and
      (.ownership_security.recovery | (delayed and
        (. == null or (.approvals | (type == "array" and all(.[]; nonempty)))))) and
      (.badges | (type == "array" and all(.[]; valid_badge)))
    ) catch false))
  '

  check "moderation, username, badge, and release field completeness" '
    def nonempty: (type == "string") and (try (length > 0) catch false);
    def uint: (type == "number") and (try ((. >= 0) and ((floor) == .)) catch false);
    def valid_report:
      type == "object" and (.id | uint) and (.owner | nonempty) and (.repo | nonempty) and
      (.reporter | nonempty) and (.reason_hash | nonempty) and
      (.status | (. == "open" or . == "resolved" or . == "appealed" or . == "appeal_resolved")) and
      has("resolution") and has("resolution_hash") and has("appeal_hash") and
      ((.resolution == null) or (.resolution | nonempty)) and
      ((.resolution_hash == null) or (.resolution_hash | nonempty)) and
      ((.appeal_hash == null) or (.appeal_hash | nonempty)) and
      (.created_at | uint) and (.updated_at | uint);
    def valid_badge:
      type == "object" and (.id | uint) and (.repo_owner | nonempty) and
      (.repo_name | nonempty) and (.recipient | nonempty) and
      (.reason | type == "string") and (.awarded_by | nonempty) and (.awarded_at | uint);
    def valid_release:
      type == "object" and (.version | nonempty) and
      (.artifacts | (type == "array" and all(.[]; type == "object" and
        (.version | nonempty) and (.platform | nonempty) and
        (.sha256 | (type == "string" and (try test("^[0-9a-fA-F]{64}$") catch false))) and
        (.registered_by | nonempty) and (.registered_at | uint))));
    (.moderation_reports | all(.[]?; try valid_report catch false)) and
    (.usernames | all(.[]?; try (type == "object" and (.name | nonempty) and
      (.owner | nonempty) and (.registered_at | uint)) catch false)) and
    (.badges_by_recipient | all(.[]?; try (type == "object" and (.recipient | nonempty) and
      (.badges | (type == "array" and all(.[]; valid_badge)))) catch false)) and
    (.releases | all(.[]?; try valid_release catch false))
  '

  check "secondary duplicate detection" '
    ((.moderation_reports | map(.id) | length == (unique | length)) and
     (.usernames | map(.name) | length == (unique | length)) and
     (.usernames | map(.owner) | length == (unique | length)) and
     (.badges_by_recipient | map(.recipient) | length == (unique | length)) and
     (.releases | map(.version) | length == (unique | length)))
  '

  if ((failed != 0)); then
    return 1
  fi

  canonical="$(mktemp)"
  if ! jq -S . "$snapshot" > "$canonical" || ! cmp -s "$canonical" "$snapshot"; then
    rm -f "$canonical"
    echo "snapshot is not canonical jq -S JSON: $snapshot" >&2
    return 1
  fi
  rm -f "$canonical"

  if ! record="$(awk '
    NF {
      if (++n != 1 || NF != 2) bad = 1
      if (n == 1) { digest = $1; name = $2 }
    }
    END {
      if (n != 1 || bad) exit 1
      printf "%s\t%s", digest, name
    }
  ' "$hash_file")"; then
    echo "hash file must contain exactly one '<sha256>  <basename>' record: $hash_file" >&2
    return 1
  fi
  IFS=$'\t' read -r expected_digest expected_name <<<"$record"
  [[ "$expected_digest" =~ ^[0-9a-f]{64}$ ]] || { echo "hash file contains an invalid SHA-256 digest" >&2; return 1; }
  [[ "$expected_name" == "$snapshot_name" ]] || {
    echo "hash file names $snapshot_name as $expected_name" >&2
    return 1
  }
  actual="$(sha256_digest "$snapshot")"
  [[ "$actual" == "$expected_digest" ]] || {
    echo "snapshot hash mismatch: expected $expected_digest, got $actual" >&2
    return 1
  }
  printf 'snapshot valid: %s\n' "$snapshot"
  printf 'sha256: %s\n' "$actual"
}

while (($#)); do
  case "$1" in
    --lcd) LCD="${2:?--lcd requires a URL}"; shift 2 ;;
    --contract) CONTRACT="${2:?--contract requires an address}"; shift 2 ;;
    --owners-file) OWNERS_FILE="${2:?--owners-file requires a path}"; shift 2 ;;
    --reports-file) REPORTS_FILE="${2:?--reports-file requires a path}"; shift 2 ;;
    --usernames-file) USERNAMES_FILE="${2:?--usernames-file requires a path}"; shift 2 ;;
    --badge-recipients-file) BADGE_RECIPIENTS_FILE="${2:?--badge-recipients-file requires a path}"; shift 2 ;;
    --releases-file) RELEASES_FILE="${2:?--releases-file requires a path}"; shift 2 ;;
    --output) OUTPUT="${2:?--output requires a path}"; shift 2 ;;
    --hash-output) HASH_OUTPUT="${2:?--hash-output requires a path}"; shift 2 ;;
    --height) HEIGHT="${2:?--height requires a value}"; shift 2 ;;
    --exported-at) EXPORTED_AT="${2:?--exported-at requires a timestamp}"; shift 2 ;;
    --validate) VALIDATE_FILE="${2:?--validate requires a snapshot path}"; shift 2 ;;
    --hash-file) VALIDATE_HASH_FILE="${2:?--hash-file requires a path}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ -n "$VALIDATE_FILE" ]]; then
  [[ -z "$OWNERS_FILE" && -z "$CONTRACT" ]] || {
    echo "--validate cannot be combined with --owners-file or --contract" >&2
    exit 2
  }
  if [[ -z "$VALIDATE_HASH_FILE" ]]; then
    VALIDATE_HASH_FILE="$VALIDATE_FILE.sha256"
  fi
  validate_snapshot "$VALIDATE_FILE" "$VALIDATE_HASH_FILE"
  exit $?
fi
[[ -z "$VALIDATE_HASH_FILE" ]] || { echo "--hash-file requires --validate" >&2; exit 2; }

[[ -n "$CONTRACT" && -n "$OWNERS_FILE" ]] || { usage; exit 2; }
[[ -r "$OWNERS_FILE" ]] || { echo "owners file is not readable: $OWNERS_FILE" >&2; exit 1; }
[[ "$CONTRACT" != *[[:space:]]* ]] || { echo "contract address must not contain whitespace" >&2; exit 2; }
[[ "$LCD" != *[[:space:]]* ]] || { echo "LCD endpoint must not contain whitespace" >&2; exit 2; }
if [[ ! "$PAGE_LIMIT" =~ ^[1-9][0-9]*$ ]] || ((PAGE_LIMIT > 100)); then
  echo "IGIT_SNAPSHOT_PAGE_LIMIT must be between 1 and 100" >&2
  exit 2
fi
if [[ -n "$HEIGHT" && ! "$HEIGHT" =~ ^[1-9][0-9]*$ ]]; then
  echo "height must be a positive integer: $HEIGHT" >&2
  exit 2
fi
if [[ -n "$EXPORTED_AT" && ! "$EXPORTED_AT" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
  echo "exported-at must be a UTC RFC3339 timestamp: $EXPORTED_AT" >&2
  exit 2
fi
for tool in curl jq base64 awk sort; do
  command -v "$tool" >/dev/null 2>&1 || { echo "$tool is required" >&2; exit 1; }
done
for optional in "$REPORTS_FILE" "$USERNAMES_FILE" "$BADGE_RECIPIENTS_FILE" "$RELEASES_FILE"; do
  [[ -z "$optional" || -r "$optional" ]] || { echo "input file is not readable: $optional" >&2; exit 1; }
done

# Enumeration files are audited inputs. Reject ambiguous rows and duplicates
# before any network request so a snapshot cannot depend on shell tokenization
# or an accidental repeated key.
validate_values_file() {
  local file="$1" label="$2" values duplicates required=0
  if (( $# >= 3 )); then
    required="$3"
  fi
  [[ -n "$file" ]] || return 0
  if ! awk -v label="$label" '
    /^[[:space:]]*#/ || NF == 0 { next }
    NF != 1 {
      printf "%s must contain exactly one value per line (line %d)\n", label, NR > "/dev/stderr"
      bad = 1
    }
    END { exit bad ? 1 : 0 }
  ' "$file"; then
    exit 2
  fi
  values="$(awk 'NF && $1 !~ /^#/ { print $1 }' "$file" | LC_ALL=C sort)"
  if ((required != 0)) && [[ -z "$values" ]]; then
    echo "$label contains no values" >&2
    exit 2
  fi
  duplicates="$(printf '%s\n' "$values" | awk 'NR > 1 && $0 == previous { print; seen = 1 } { previous = $0 } END { exit seen ? 0 : 1 }' || true)"
  if [[ -n "$duplicates" ]]; then
    printf '%s contains duplicate values:\n%s\n' "$label" "$duplicates" >&2
    exit 2
  fi
}

validate_values_file "$OWNERS_FILE" "owners file" 1
validate_values_file "$REPORTS_FILE" "reports file"
validate_values_file "$USERNAMES_FILE" "usernames file"
validate_values_file "$BADGE_RECIPIENTS_FILE" "badge recipients file"
validate_values_file "$RELEASES_FILE" "releases file"

LCD="${LCD%/}"
HASH_OUTPUT="${HASH_OUTPUT:-$OUTPUT.sha256}"
output_dir="$(dirname "$OUTPUT")"
hash_dir="$(dirname "$HASH_OUTPUT")"
[[ -d "$output_dir" ]] || { echo "output directory does not exist: $output_dir" >&2; exit 1; }
[[ -d "$hash_dir" ]] || { echo "hash output directory does not exist: $hash_dir" >&2; exit 1; }
output_target="$(cd "$output_dir" && pwd -P)/$(basename -- "$OUTPUT")"
hash_target="$(cd "$hash_dir" && pwd -P)/$(basename -- "$HASH_OUTPUT")"
[[ "$output_target" != "$hash_target" ]] || { echo "snapshot and hash output must differ" >&2; exit 2; }
[[ "$(basename -- "$OUTPUT")" != *[[:space:]]* ]] || {
  echo "snapshot filename must not contain whitespace" >&2
  exit 2
}

if [[ -z "$HEIGHT" ]]; then
  latest="$(curl --fail --silent --show-error --retry 3 \
    "$LCD/cosmos/base/tendermint/v1beta1/blocks/latest")"
  HEIGHT="$(jq -er '.block.header.height' <<<"$latest")"
fi
fixed_block="$(curl --fail --silent --show-error --retry 3 \
  "$LCD/cosmos/base/tendermint/v1beta1/blocks/$HEIGHT")"
CHAIN_ID="$(jq -er '.block.header.chain_id' <<<"$fixed_block")"
fixed_height="$(jq -er '.block.header.height' <<<"$fixed_block")"
[[ "$fixed_height" == "$HEIGHT" ]] || {
  echo "fixed block response height $fixed_height does not match requested height $HEIGHT" >&2
  exit 1
}
BLOCK_HASH="$(jq -er '.block_id.hash | ascii_downcase | select(test("^[0-9a-f]{64}$"))' <<<"$fixed_block")"

query() {
  local message="$1"
  local encoded contract_path response
  encoded="$(printf '%s' "$message" | base64 | tr -d '\n' | jq -sRr @uri)"
  contract_path="$(printf '%s' "$CONTRACT" | jq -sRr @uri)"
  response="$(curl --fail --silent --show-error --retry 3 \
    -H "x-cosmos-block-height: $HEIGHT" \
    "$LCD/cosmwasm/wasm/v1/contract/$contract_path/smart/$encoded")"
  if ! jq -ce '.data' <<<"$response"; then
    echo "LCD smart query response is invalid or missing data" >&2
    return 1
  fi
}

values() {
  local file="$1"
  [[ -n "$file" ]] || return 0
  awk 'NF && $1 !~ /^#/ { print $1 }' "$file" | LC_ALL=C sort -u
}

list_repos() {
  local owner="$1" start="" page count next all='[]' message
  while :; do
    message="$(jq -cn --arg owner "$owner" --arg start "$start" --argjson limit "$PAGE_LIMIT" \
      '{list_repos:({owner:$owner,limit:$limit} + (if $start == "" then {} else {start_after:$start} end))}')"
    page="$(query "$message")"
    count="$(jq -er '.repos | length' <<<"$page")"
    all="$(jq -cn --argjson all "$all" --argjson page "$page" '$all + $page.repos')"
    ((count < PAGE_LIMIT)) && break
    next="$(jq -er '.repos[-1].name' <<<"$page")"
    [[ "$next" != "$start" ]] || { echo "repo pagination did not advance for $owner" >&2; return 1; }
    start="$next"
  done
  printf '%s\n' "$all"
}

list_refs() {
  local owner="$1" repo="$2" start="" page count next all='[]' message
  while :; do
    message="$(jq -cn --arg owner "$owner" --arg repo "$repo" --arg start "$start" --argjson limit "$PAGE_LIMIT" \
      '{list_refs:({owner:$owner,repo:$repo,limit:$limit} + (if $start == "" then {} else {start_after:$start} end))}')"
    page="$(query "$message")"
    count="$(jq -er '.refs | length' <<<"$page")"
    all="$(jq -cn --argjson all "$all" --argjson page "$page" '$all + $page.refs')"
    ((count < PAGE_LIMIT)) && break
    next="$(jq -er '.refs[-1].ref_name' <<<"$page")"
    [[ "$next" != "$start" ]] || { echo "ref pagination did not advance for $owner/$repo" >&2; return 1; }
    start="$next"
  done
  printf '%s\n' "$all"
}

list_collaborators() {
  local owner="$1" repo="$2" start="" page count next all='[]' message
  while :; do
    message="$(jq -cn --arg owner "$owner" --arg repo "$repo" --arg start "$start" --argjson limit "$PAGE_LIMIT" \
      '{list_collaborators:({owner:$owner,repo:$repo,limit:$limit} + (if $start == "" then {} else {start_after:$start} end))}')"
    page="$(query "$message")"
    count="$(jq -er '.collaborators | length' <<<"$page")"
    all="$(jq -cn --argjson all "$all" --argjson page "$page" '$all + $page.collaborators')"
    ((count < PAGE_LIMIT)) && break
    next="$(jq -er '.collaborators[-1].address' <<<"$page")"
    [[ "$next" != "$start" ]] || { echo "collaborator pagination did not advance for $owner/$repo" >&2; return 1; }
    start="$next"
  done
  printf '%s\n' "$all"
}

list_badges() {
  local scope="$1" owner_or_recipient="$2" repo="${3:-}"
  local start="" page count next all='[]' message
  while :; do
    if [[ "$scope" == "repo" ]]; then
      message="$(jq -cn --arg owner "$owner_or_recipient" --arg repo "$repo" --arg start "$start" --argjson limit "$PAGE_LIMIT" \
        '{badges_by_repo:({owner:$owner,repo:$repo,limit:$limit} + (if $start == "" then {} else {start_after:($start|tonumber)} end))}')"
    else
      message="$(jq -cn --arg recipient "$owner_or_recipient" --arg start "$start" --argjson limit "$PAGE_LIMIT" \
        '{badges_by_recipient:({recipient:$recipient,limit:$limit} + (if $start == "" then {} else {start_after:($start|tonumber)} end))}')"
    fi
    page="$(query "$message")"
    count="$(jq -er '.badges | length' <<<"$page")"
    all="$(jq -cn --argjson all "$all" --argjson page "$page" '$all + $page.badges')"
    ((count < PAGE_LIMIT)) && break
    next="$(jq -er '.badges[-1].id' <<<"$page")"
    [[ "$next" != "$start" ]] || { echo "badge pagination did not advance" >&2; return 1; }
    start="$next"
  done
  printf '%s\n' "$all"
}

owners="$(values "$OWNERS_FILE" | jq -Rsc 'split("\n") | map(select(length > 0))')"
[[ "$(jq 'length' <<<"$owners")" -gt 0 ]] || { echo "owners file contains no addresses" >&2; exit 1; }

contract_config="$(query '{"config":{}}')"
upgrade_security="$(query '{"upgrade_security":{}}')"
repos='[]'
repo_refs='[]'
collaborators='[]'
repo_extensions='[]'

while IFS= read -r owner; do
  [[ -n "$owner" ]] || continue
  owner_repos="$(list_repos "$owner")"
  repos="$(jq -cn --argjson all "$repos" --argjson current "$owner_repos" '$all + $current')"
  while IFS= read -r repo; do
    [[ -n "$repo" ]] || continue
    refs="$(list_refs "$owner" "$repo")"
    collab="$(list_collaborators "$owner" "$repo")"
    splits="$(query "$(jq -cn --arg owner "$owner" --arg repo "$repo" '{revenue_splits:{owner:$owner,repo:$repo}}')")"
    sponsors="$(query "$(jq -cn --arg owner "$owner" --arg repo "$repo" '{sponsor_totals:{owner:$owner,repo:$repo}}')")"
    security="$(query "$(jq -cn --arg owner "$owner" --arg repo "$repo" '{ownership_security:{owner:$owner,repo:$repo}}')")"
    repo_badges="$(list_badges repo "$owner" "$repo")"
    repo_refs="$(jq -cn --argjson all "$repo_refs" --arg owner "$owner" --arg repo "$repo" --argjson refs "$refs" '$all + [{owner:$owner,repo:$repo,refs:$refs}]')"
    collaborators="$(jq -cn --argjson all "$collaborators" --arg owner "$owner" --arg repo "$repo" --argjson collab "$collab" '$all + [{owner:$owner,repo:$repo,collaborators:$collab}]')"
    repo_extensions="$(jq -cn --argjson all "$repo_extensions" --arg owner "$owner" --arg repo "$repo" \
      --argjson splits "$splits" --argjson sponsors "$sponsors" --argjson security "$security" --argjson badges "$repo_badges" \
      '$all + [{owner:$owner,repo:$repo,revenue_splits:$splits.splits,sponsor_totals:$sponsors.totals,ownership_security:$security,badges:$badges}]')"
  done < <(jq -r '.[].name' <<<"$owner_repos")
done < <(jq -r '.[]' <<<"$owners")

reports='[]'
while IFS= read -r id; do
  [[ -n "$id" ]] || continue
  [[ "$id" =~ ^[0-9]+$ ]] || { echo "invalid report ID: $id" >&2; exit 1; }
  item="$(query "$(jq -cn --argjson id "$id" '{moderation_report:{report_id:$id}}')")"
  reports="$(jq -cn --argjson all "$reports" --argjson item "$item" '$all + [$item]')"
done < <(values "$REPORTS_FILE")

usernames='[]'
while IFS= read -r name; do
  [[ -n "$name" ]] || continue
  item="$(query "$(jq -cn --arg name "$name" '{resolve_username:{name:$name}}')")"
  usernames="$(jq -cn --argjson all "$usernames" --argjson item "$item" '$all + [$item]')"
done < <(values "$USERNAMES_FILE")

recipient_badges='[]'
while IFS= read -r recipient; do
  [[ -n "$recipient" ]] || continue
  items="$(list_badges recipient "$recipient")"
  recipient_badges="$(jq -cn --argjson all "$recipient_badges" --arg recipient "$recipient" --argjson badges "$items" '$all + [{recipient:$recipient,badges:$badges}]')"
done < <(values "$BADGE_RECIPIENTS_FILE")

releases='[]'
while IFS= read -r version; do
  [[ -n "$version" ]] || continue
  item="$(query "$(jq -cn --arg version "$version" '{release_artifacts:{version:$version}}')")"
  releases="$(jq -cn --argjson all "$releases" --argjson item "$item" '$all + [$item]')"
done < <(values "$RELEASES_FILE")

if [[ -z "$EXPORTED_AT" ]]; then
  EXPORTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
fi
tmp="$(mktemp "$output_dir/.v1-snapshot.XXXXXX")"
hash_tmp="$(mktemp "$hash_dir/.v1-snapshot-hash.XXXXXX")"
trap 'rm -f "$tmp" "$hash_tmp"' EXIT
jq -nS \
  --arg schema "igit.cosmwasm-v1.snapshot.v1" \
  --arg exported_at "$EXPORTED_AT" \
  --arg lcd "$LCD" --arg contract "$CONTRACT" --arg chain_id "$CHAIN_ID" --arg height "$HEIGHT" --arg block_hash "$BLOCK_HASH" \
  --argjson owners "$owners" --argjson config "$contract_config" --argjson upgrade "$upgrade_security" \
  --argjson repos "$repos" --argjson refs "$repo_refs" --argjson collaborators "$collaborators" \
  --argjson extensions "$repo_extensions" --argjson reports "$reports" --argjson usernames "$usernames" \
  --argjson badges "$recipient_badges" --argjson releases "$releases" \
  '{schema:$schema,source:{lcd:$lcd,contract:$contract,chain_id:$chain_id,height:$height,block_hash:$block_hash,exported_at:$exported_at},contract_config:$config,upgrade_security:$upgrade,repositories:$repos,refs:$refs,collaborators:$collaborators,repo_extensions:$extensions,moderation_reports:$reports,usernames:$usernames,badges_by_recipient:$badges,releases:$releases}' \
  > "$tmp"

digest="$(sha256_digest "$tmp")"
printf '%s  %s\n' "$digest" "$(basename "$OUTPUT")" > "$hash_tmp"
validate_snapshot "$tmp" "$hash_tmp" "$(basename "$OUTPUT")" >/dev/null
mv -f "$tmp" "$OUTPUT"
mv -f "$hash_tmp" "$HASH_OUTPUT"
trap - EXIT
echo "snapshot: $OUTPUT"
echo "height:   $HEIGHT"
echo "sha256:   $digest"
echo "hash file: $HASH_OUTPUT"
