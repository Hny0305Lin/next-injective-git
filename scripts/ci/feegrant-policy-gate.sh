#!/usr/bin/env bash
# Local policy gate for the platform feegrant issuer.
#
# This script never broadcasts a transaction. The operator first runs `check`,
# submits a Cosmos feegrant transaction with the approved signer, then records
# the resulting grant. Push records must only be written after the corresponding
# update_ref transaction has been confirmed with code 0.
set -euo pipefail

STATE="${IGIT_FEEGRANT_STATE:-/var/lib/igit-feegrant/state.tsv}"
MAX_PUSHES="${IGIT_FEEGRANT_MAX_PUSHES:-3}"
TTL_SECONDS="${IGIT_FEEGRANT_TTL_SECONDS:-604800}"
COOLDOWN_SECONDS="${IGIT_FEEGRANT_COOLDOWN_SECONDS:-2592000}"
MAX_GRANTS_PER_DAY="${IGIT_FEEGRANT_MAX_GRANTS_PER_DAY:-100}"

die() {
  echo "feegrant policy gate: $*" >&2
  exit 1
}

usage() {
  cat >&2 <<'EOF'
usage:
  feegrant-policy-gate.sh check <inj-address> <identity-hash> [now]
  feegrant-policy-gate.sh record-grant <inj-address> <identity-hash> <tx-hash> <expires-at> [now]
  feegrant-policy-gate.sh record-push <inj-address> <tx-hash> [now]
  feegrant-policy-gate.sh record-revoke <inj-address> <tx-hash> [now]
  feegrant-policy-gate.sh status <inj-address> [now]
EOF
  exit 2
}

[[ "$MAX_PUSHES" =~ ^[1-9][0-9]*$ ]] || die "MAX_PUSHES must be a positive integer"
[[ "$TTL_SECONDS" =~ ^[1-9][0-9]*$ ]] || die "TTL_SECONDS must be a positive integer"
[[ "$COOLDOWN_SECONDS" =~ ^[1-9][0-9]*$ ]] || die "COOLDOWN_SECONDS must be a positive integer"
[[ "$MAX_GRANTS_PER_DAY" =~ ^[1-9][0-9]*$ ]] || die "MAX_GRANTS_PER_DAY must be a positive integer"

command -v flock >/dev/null || die "flock is required"
mkdir -p "$(dirname "$STATE")"
chmod 700 "$(dirname "$STATE")"
touch "$STATE"
chmod 600 "$STATE"
exec 9>"$STATE.lock"
flock 9

address_re='^inj1[0-9a-z]{38}$'
hash_re='^[[:xdigit:]]{64}$'
tx_re='^[[:xdigit:]]{64}$'
[[ "${1:-}" =~ ^(check|record-grant|record-push|record-revoke|status)$ ]] || usage
action="$1"

validate_address() {
  [[ "$1" =~ $address_re ]] || die "address must be a lowercase 42-character inj1... bech32 address"
}

validate_hash() {
  [[ "$1" =~ $hash_re ]] || die "identity hash must be 64 hexadecimal characters"
}

validate_tx() {
  [[ "$1" =~ $tx_re ]] || die "transaction hash must be 64 hexadecimal characters"
}

now_value() {
  local value="${1:-}"
  if [ -n "$value" ]; then
    [[ "$value" =~ ^[0-9]+$ ]] || die "timestamp must be an integer"
    printf '%s\n' "$value"
  else
    date +%s
  fi
}

validate_state() {
  awk -F '\t' '
    NF == 0 { next }
    $1 == "grant" && NF == 6 && $2 ~ /^inj1[0-9a-z]{38}$/ && $3 ~ /^[[:xdigit:]]{64}$/ && $4 ~ /^[0-9]+$/ && $5 ~ /^[0-9]+$/ && $6 ~ /^[[:xdigit:]]{64}$/ { next }
    $1 == "push" && NF == 4 && $2 ~ /^inj1[0-9a-z]{38}$/ && $3 ~ /^[[:xdigit:]]{64}$/ && $4 ~ /^[0-9]+$/ { next }
    $1 == "revoke" && NF == 4 && $2 ~ /^inj1[0-9a-z]{38}$/ && $3 ~ /^[[:xdigit:]]{64}$/ && $4 ~ /^[0-9]+$/ { next }
    { bad = 1 }
    END { exit bad }
  ' "$STATE" || die "malformed state file; refusing to issue or count grants"
}

grant_summary() {
  local address="$1" identity="${2,,}" now="$3"
  awk -F '\t' -v address="$address" -v identity="$identity" -v now="$now" \
    -v cooldown="$COOLDOWN_SECONDS" -v max_pushes="$MAX_PUSHES" '
    $1 == "grant" {
      if ($2 == address && $4 >= latest_grant) {
        latest_grant = $4; latest_expires = $5; latest_tx = $6; latest_identity = $3; pushes = 0; revoked = 0
      }
      if ($3 == identity && $2 != address && $4 + cooldown > now) other_identity = 1
    }
    $1 == "push" && $2 == address && $4 >= latest_grant { pushes++ }
    $1 == "revoke" && $2 == address && $4 >= latest_grant { revoked = 1 }
    END {
      if (other_identity) { print "identity_in_use"; exit }
      if (revoked && latest_grant + cooldown > now) { print "revoked"; exit }
      if (latest_grant > 0 && latest_expires > now) { print "active_grant"; exit }
      if (latest_grant > 0 && latest_grant + cooldown > now) { print "cooldown"; exit }
      # A completed grant is a cooldown record, not a permanent ban. Once the
      # address/identity cooldown has elapsed, a fresh grant starts a new push
      # counter. Active, revoked, and cooling-down grants are handled above.
      print "eligible"
    }
  ' "$STATE"
}

check_eligibility() {
  local address="$1" identity="${2,,}" now="$3"
  validate_address "$address"
  validate_hash "$identity"
  local grants_today
  grants_today=$(awk -F '\t' -v cutoff="$((now - 86400))" '$1 == "grant" && $4 >= cutoff { n++ } END { print n + 0 }' "$STATE")
  [ "$grants_today" -lt "$MAX_GRANTS_PER_DAY" ] || die "daily treasury grant limit reached"
  local reason
  reason=$(grant_summary "$address" "$identity" "$now")
  [ "$reason" = eligible ] || die "not eligible: $reason"
  echo "eligible"
}

validate_state
case "$action" in
  check)
    [ "$#" -ge 3 ] && [ "$#" -le 4 ] || usage
    now=$(now_value "${4:-}")
    check_eligibility "$2" "$3" "$now"
    ;;
  record-grant)
    [ "$#" -ge 5 ] && [ "$#" -le 6 ] || usage
    address="$2"; identity="$3"; tx="$4"; expires="$5"; now=$(now_value "${6:-}")
    validate_tx "$tx"
    [[ "$expires" =~ ^[0-9]+$ ]] || die "expires-at must be an integer"
    [ "$expires" -gt "$now" ] || die "grant expiration must be in the future"
    [ "$expires" -le "$((now + TTL_SECONDS))" ] || die "grant expiration exceeds policy TTL"
    check_eligibility "$address" "$identity" "$now" >/dev/null
    printf 'grant\t%s\t%s\t%s\t%s\t%s\n' "$address" "${identity,,}" "$now" "$expires" "${tx,,}" >> "$STATE"
    echo "grant recorded for $address; pushes=0; max_pushes=$MAX_PUSHES"
    ;;
  record-push)
    [ "$#" -ge 3 ] && [ "$#" -le 4 ] || usage
    address="$2"; tx="$3"; now=$(now_value "${4:-}")
    validate_address "$address"
    validate_tx "$tx"
    latest=$(awk -F '\t' -v address="$address" '$1 == "grant" && $2 == address && $4 >= latest { latest=$4; expires=$5; identity=$3 } END { if (latest) printf "%s\t%s\t%s\n", latest, expires, identity }' "$STATE")
    [ -n "$latest" ] || die "no recorded grant for address"
    IFS=$'\t' read -r granted expires identity <<< "$latest"
    [ "$expires" -gt "$now" ] || die "grant has expired"
    revoked=$(awk -F '\t' -v address="$address" -v granted="$granted" '$1 == "revoke" && $2 == address && $4 >= granted { print 1; exit }' "$STATE")
    [ -z "$revoked" ] || die "grant has been revoked"
    pushes=$(awk -F '\t' -v address="$address" -v granted="$granted" '$1 == "push" && $2 == address && $4 >= granted { n++ } END { print n + 0 }' "$STATE")
    [ "$pushes" -lt "$MAX_PUSHES" ] || die "push allowance exhausted"
    printf 'push\t%s\t%s\t%s\n' "$address" "${tx,,}" "$now" >> "$STATE"
    echo "push recorded for $address; used=$((pushes + 1)); remaining=$((MAX_PUSHES - pushes - 1))"
    ;;
  record-revoke)
    [ "$#" -ge 3 ] && [ "$#" -le 4 ] || usage
    address="$2"; tx="$3"; now=$(now_value "${4:-}")
    validate_address "$address"
    validate_tx "$tx"
    latest=$(awk -F '\t' -v address="$address" '$1 == "grant" && $2 == address && $4 >= latest { latest=$4 } END { print latest + 0 }' "$STATE")
    [ "$latest" -gt 0 ] || die "no recorded grant for address"
    printf 'revoke\t%s\t%s\t%s\n' "$address" "${tx,,}" "$now" >> "$STATE"
    echo "revoke recorded for $address"
    ;;
  status)
    [ "$#" -ge 2 ] && [ "$#" -le 3 ] || usage
    address="$2"; now=$(now_value "${3:-}")
    validate_address "$address"
    awk -F '\t' -v address="$address" -v now="$now" -v max_pushes="$MAX_PUSHES" '
      $1 == "grant" && $2 == address && $4 >= latest { latest=$4; identity=$3; expires=$5; tx=$6; pushes=0; revoked=0 }
      $1 == "push" && $2 == address && $4 >= latest { pushes++ }
      $1 == "revoke" && $2 == address && $4 >= latest { revoked=1 }
      END {
        if (!latest) { print "status=none"; exit }
        state = (revoked ? "revoked" : (expires > now ? "active" : "expired"))
        printf "status=%s\nidentity_hash=%s\nexpires_at=%s\npushes=%d\nremaining=%d\ngrant_tx=%s\n", state, identity, expires, pushes, (max_pushes - pushes > 0 ? max_pushes - pushes : 0), tx
      }
    ' "$STATE"
    ;;
esac
