# CosmWasm V1 Protocol Baseline

This document freezes the behavior of the current `repo-registry` contract.
It is the compatibility contract for the EVM V2 implementation: an EVM
feature is not considered migrated until its observable behavior, authorization
rules, and failure cases are covered by an equivalent test.

The source of truth for executable behavior is
`archive/cosmwasm-v1/contracts/repo-registry/tests/integration.rs`. The test names below are kept
stable so a V2 test suite can use the same scenario names.

## Core repository state

| Capability | V1 behavior | Regression scenario |
| --- | --- | --- |
| Create | Sender becomes owner. Names are normalized and validated; duplicate names under one owner are rejected. | `create_repo_and_query_info`, `invalid_repo_name_rejected` |
| Metadata | Description and default branch use patch semantics: an omitted field is preserved, an explicit empty string clears it, and even an empty patch advances `updated_at`. Only the owner may update metadata. | `update_repo_info_patches_fields_clears_values_and_preserves_repo_state`, `update_repo_info_none_fields_still_advances_updated_at`, `update_repo_info_is_owner_only` |
| Refs | A ref stores its commit SHA, ordered pack URI list, update time, and updater. | `push_and_resolve_ref` |
| Optimistic update | `expected_sha`, when present, must equal the current tip. `force=true` bypasses the check. | `stale_push_rejected_force_push_allowed` |
| Delete | Owner or maintainer may delete a ref. A reader may not write. | `delete_ref_and_list`, `collaborator_permissions` |
| Queries | `repo_info`, `list_repos`, `list_refs`, and `resolve_ref` return deterministic domain records and support pagination. | `create_repo_and_query_info`, `push_and_resolve_ref` |

Commit SHAs are 40- or 64-character hexadecimal Git object identities; V1
accepts either letter case. Pack URIs use a non-empty `<scheme>://<locator>`
shape and are stored verbatim. The contract rejects malformed or empty values
rather than silently changing them.

## Authorization and lifecycle

- The owner can update metadata, update/delete refs, manage collaborators,
  transfer ownership, configure guardians, and set revenue splits.
- A `maintainer` can update/delete refs but cannot change repository metadata or
  collaborators.
- A `reader` has read-only access.
- Ownership transfer and guardian recovery are delayed. The proposed owner
  must explicitly accept after the timelock; guardian recovery also requires
  the configured threshold.
- Ownership transfer uses a seven-day timelock, rejects a target that already
  owns the same repo name, can be cancelled by the current owner, and is
  mutually exclusive with guardian recovery. Frozen and Delisted repositories
  may still be transferred.
- A completed V1 transfer moves refs, collaborators (except the new owner),
  and sponsor totals while preserving repo/ref timestamps and moderation. It
  deletes revenue splits and guardian/recovery state. Badges and moderation
  reports remain indexed by the old owner/repo key; these are characterization
  facts, not recommended V2 data-loss semantics.
- Fork is Active-only. It copies description, default branch, and every ref/CID
  snapshot, rewrites each copied ref's timestamp/updater to the fork time and
  forker, and records the source string. It does not copy collaborators,
  splits, sponsor totals, guardians, badges, reports, or pending ownership
  state. The V1 implementation iterates all refs, including refs beyond one
  query page, without a hard fork gas bound.
- A frozen repository rejects ref writes, forks, badges, and sponsorships, but
  its owner may still update repository metadata. A delisted repository also
  permits metadata updates, remains queryable, and is omitted from the default
  active repository listing.
- Ownership transfer, recovery, and moderation actions emit audit attributes;
  no action may silently mutate the owner.

Scenarios: `collaborator_permissions`, `transfer_ownership_moves_refs`,
`transfer_ownership_cancel_collisions_and_recovery_are_mutually_exclusive`,
`transfer_ownership_preserves_and_resets_v1_extension_state`,
`transfer_ownership_is_allowed_while_frozen_or_delisted`,
`guardian_recovery_requires_threshold_and_timelock`,
`moderation_freeze_blocks_push`,
`update_repo_info_is_allowed_while_delisted_or_frozen`,
`fork_copies_refs_and_records_source`,
`fork_copies_only_v1_metadata_and_ref_snapshot`, and
`fork_copies_refs_beyond_the_default_query_page`.

## Economic and community extensions

- Sponsorship takes the configured native denomination, charges the platform
  fee, then distributes the remainder according to the repository split table.
  An empty table sends the remainder to the owner. No funds are custodied.
- Split entries must be unique, cannot contain the owner, and must total at
  most 10,000 basis points.
- Usernames are first-come-first-served. A valid name is reserved for one
  address, requires the exact configured deposit plus fee, and releasing it
  refunds only the deposit.
- Badges are non-transferable contribution records. The owner cannot award a
  badge to themself, and frozen repositories cannot issue badges.
- Release artifacts are immutable `(version, platform, sha256)` records. A
  duplicate tuple is rejected and a non-admin cannot register one.

Scenarios: `sponsor_splits_fee_and_shares`, `sponsor_frozen_repo_rejected`,
`revenue_splits_validation`, `username_register_and_release`,
`award_badge_and_trophy_wall`, and `admin_registers_immutable_release_checksums`.

## Moderation and upgrade controls

Reports contain an immutable reason hash, reporter, repository, status, and an
append-only resolution/appeal trail. The moderation committee (or the admin
when no committee is configured) controls status changes. Upgrade scheduling
records the exact Wasm SHA-256 and enforces the configured delay before
migration. A migration with a different hash or before the delay is rejected.

Scenarios: `moderation_report_and_appeal_are_auditable`,
`upgrade_schedule_is_admin_only_and_timelocked`, and `migrate_same_contract_ok`.

## V2 compatibility requirements

1. Preserve the domain-level fields above, including `forked_from`, ref pack
   ordering, moderation state, and update timestamps.
2. Preserve authorization outcomes and conflict behavior. Error text may be
   adapted to EVM decoding, but the CLI must classify the same operation as
   accepted or rejected.
3. Emit an EVM event for every state transition that currently has an
   observable V1 response attribute.
4. Import snapshots produced by `archive/cosmwasm-v1/scripts/v1-export-snapshot.sh` and verify the
   imported count and critical fields before enabling V2 writes.
5. During migration, create operations always target V2. Reads use V2 first
   and fall back to V1 only when the V2 repository does not exist. A repository
   must never be written to both contracts after its migration marker is set.

Intentional V2 corrections to V1 side effects, such as preserving revenue
splits and attaching badges/reports to a stable repo identity, must be declared
and tested rather than hidden as parity. The approved identity and importer
rules are in [evm-v2-repo-identity.md](./evm-v2-repo-identity.md).

## Snapshot and regression commands

The read-only exporter takes an audited owner/key inventory, pins LCD smart
queries to one block height, paginates repository/ref/collaborator/badge state,
and writes a companion SHA-256 record:

```bash
bash archive/cosmwasm-v1/scripts/v1-export-snapshot.sh \
  --contract inj1... --owners-file owners.txt \
  --output v1-snapshot.json
bash archive/cosmwasm-v1/scripts/v1-export-snapshot.sh \
  --validate v1-snapshot.json --hash-file v1-snapshot.json.sha256
bash archive/cosmwasm-v1/scripts/v1-export-snapshot-test.sh
```

The fixture uses a fake LCD and must stay offline. It intentionally exercises
pagination, height headers, extension state, URI-safe query encoding, schema
and field completeness, duplicate repository/ref rejection, deterministic
timestamp/hash verification, malformed-input rejection, and fail-closed
temporary output handling; it must not be replaced by a live testnet script.

The --validate mode is an offline, read-only check. It parses the snapshot,
enforces the frozen schema and cross-record keys, requires canonical jq -S
JSON, and recomputes the companion SHA-256 record before a migration importer
may use the file. Use --exported-at <UTC-RFC3339> in controlled fixtures when
a byte-for-byte reproducible export is required.
