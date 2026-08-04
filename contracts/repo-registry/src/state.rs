use cosmwasm_schema::cw_serde;
use cosmwasm_std::{Addr, Coin, Uint128};
use cw_storage_plus::{Item, Map};

/// Global contract config.
#[cw_serde]
pub struct Config {
    /// Contract admin (can migrate / future governance hooks).
    pub admin: Addr,
    /// Content moderation committee (multisig); falls back to admin when
    /// unset. Kept separate from `admin` so upgrade power and content
    /// power can live in different multisigs (open-questions §5.3/§7).
    pub moderation_committee: Option<Addr>,
    /// Treasury receiving the platform fee + username fees (§3).
    pub treasury: Addr,
    /// Platform cut of every sponsorship, in basis points.
    /// Hard-capped by `MAX_PLATFORM_FEE_BPS`.
    pub platform_fee_bps: u16,
    /// Refundable deposit locked while a username is held (§4).
    pub username_deposit: Coin,
    /// Non-refundable registration fee sent to the treasury (§4).
    #[serde(default = "default_username_fee")]
    pub username_fee: Coin,
    /// Names that cannot be claimed by users.
    #[serde(default = "default_reserved_usernames")]
    pub reserved_usernames: Vec<String>,
}

fn default_username_fee() -> Coin {
    Coin {
        denom: "inj".to_string(),
        amount: Uint128::zero(),
    }
}

fn default_reserved_usernames() -> Vec<String> {
    ["admin", "api", "git", "help", "injective", "owner", "root", "support", "www"]
        .into_iter()
        .map(str::to_string)
        .collect()
}

/// Hard cap for the platform fee: 5% (§3 已定案).
pub const MAX_PLATFORM_FEE_BPS: u16 = 500;
/// Default platform fee: 3% (§3 已定案).
pub const DEFAULT_PLATFORM_FEE_BPS: u16 = 300;

/// One revenue-split recipient (§3). Shares are NOT transferable — the
/// table can only be rewritten by the owner (L3/L4 forbidden by design).
#[cw_serde]
pub struct SplitEntry {
    pub address: Addr,
    /// Share of the post-fee amount in basis points; the table sums to
    /// at most 10_000 and the remainder always goes to the repo owner.
    pub bps: u16,
}

/// A registered username (§4): first-come-first-served, deposit-backed.
#[cw_serde]
pub struct UsernameRecord {
    pub owner: Addr,
    /// Deposit escrowed at registration, refunded on release.
    pub deposit: Coin,
    /// Block timestamp (seconds) of registration.
    pub registered_at: u64,
}

/// Content moderation state of a repository (open-questions §5.3).
#[cw_serde]
pub enum ModerationStatus {
    /// Normal operation.
    Active,
    /// Hidden from frontends/indexers; chain writes still allowed.
    Delisted,
    /// Contract rejects new pushes / ref changes.
    Frozen,
}

/// Repository metadata stored on-chain.
#[cw_serde]
pub struct Repo {
    pub owner: Addr,
    pub name: String,
    pub description: String,
    pub default_branch: String,
    /// Block timestamp (seconds) at creation.
    pub created_at: u64,
    /// Block timestamp (seconds) of last ref update.
    pub updated_at: u64,
    /// Moderation state; only the committee can change it.
    pub moderation_status: ModerationStatus,
    /// "owner/repo" this repository was forked from, if any.
    /// serde(default) keeps pre-v0.3.1 stored entries readable after migrate.
    #[serde(default)]
    pub forked_from: Option<String>,
}

/// A single git ref (branch/tag) entry.
#[cw_serde]
pub struct RefEntry {
    /// Commit SHA the ref points to (hex, sha1 40 chars or sha256 64 chars).
    pub commit_sha: String,
    /// Ordered list of storage URIs (e.g. "ipfs://<cid>"); fetching all
    /// packfiles and applying them in order reconstructs the full history
    /// reachable from `commit_sha`.
    pub pack_uris: Vec<String>,
    /// Block timestamp (seconds) of this update.
    pub updated_at: u64,
    /// Address that performed the update.
    pub updated_by: Addr,
}

/// Collaborator role on a repository.
#[cw_serde]
pub enum Role {
    /// Can push (update refs) but not manage collaborators.
    Maintainer,
    /// Read-only marker (useful for future private repos / indexing).
    Reader,
}

/// A contribution badge (§3 L1): a non-transferable, purely honorary token
/// awarded by a repo owner to a contributor. No monetary rights attached
/// (L3/L4 forbidden by design), hence no transfer mechanics exist.
#[cw_serde]
pub struct Badge {
    pub id: u64,
    pub repo_owner: Addr,
    pub repo_name: String,
    pub recipient: Addr,
    /// Free-text reason (≤256 chars); v3 will reference on-chain issue/PR ids.
    pub reason: String,
    pub awarded_by: Addr,
    /// Block timestamp (seconds).
    pub awarded_at: u64,
}

#[cw_serde]
pub enum ReportStatus {
    Open,
    Resolved,
    Appealed,
    AppealResolved,
}

#[cw_serde]
pub struct ModerationReport {
    pub id: u64,
    pub owner: Addr,
    pub repo: String,
    pub reporter: Addr,
    pub reason_hash: String,
    pub status: ReportStatus,
    pub resolution: Option<ModerationStatus>,
    pub resolution_hash: Option<String>,
    pub appeal_hash: Option<String>,
    pub created_at: u64,
    pub updated_at: u64,
}

/// Immutable checksum record for a published CLI/helper/Wasm artifact.
/// Registration is keyed by (semantic version, platform).
#[cw_serde]
pub struct ReleaseArtifact {
    pub version: String,
    pub platform: String,
    pub sha256: String,
    pub registered_by: Addr,
    pub registered_at: u64,
}

#[cw_serde]
pub struct OwnershipTransfer {
    pub new_owner: Addr,
    pub proposed_at: u64,
    pub execute_after: u64,
}

#[cw_serde]
pub struct RecoveryProposal {
    pub new_owner: Addr,
    pub proposed_at: u64,
    pub execute_after: u64,
    pub approvals: Vec<Addr>,
}

#[cw_serde]
pub struct GuardianConfig {
    pub threshold: u8,
}

/// A scheduled contract upgrade. The chain-level admin (configured as a
/// multisig in production) must announce the exact Wasm hash and wait before
/// invoking the migration entry point.
#[cw_serde]
pub struct UpgradeProposal {
    pub wasm_sha256: String,
    pub proposed_at: u64,
    pub execute_after: u64,
}

/// Production upgrade delay required by the governance policy.
pub const UPGRADE_TIMELOCK_SECONDS: u64 = 14 * 24 * 60 * 60;

pub const CONFIG: Item<Config> = Item::new("config");

/// (owner, repo_name) => Repo
pub const REPOS: Map<(&Addr, &str), Repo> = Map::new("repos");

/// (owner, repo_name, ref_name) => RefEntry
pub const REFS: Map<(&Addr, &str, &str), RefEntry> = Map::new("refs");

/// (owner, repo_name, collaborator) => Role
pub const COLLABORATORS: Map<(&Addr, &str, &Addr), Role> = Map::new("collaborators");

/// (owner, repo_name) => revenue split table (absent = 100% to owner)
pub const REVENUE_SPLITS: Map<(&Addr, &str), Vec<SplitEntry>> = Map::new("revenue_splits");

/// username => record
pub const USERNAMES: Map<&str, UsernameRecord> = Map::new("usernames");

/// address => username (enforces one name per address)
pub const ADDR_TO_NAME: Map<&Addr, String> = Map::new("addr_to_name");

/// Lifetime sponsorship volume per repo, per denom: (owner, repo, denom) => total.
/// Powers the sponsor wall / §14 copyright-subsidy metrics without an indexer.
pub const SPONSOR_TOTALS: Map<(&Addr, &str, &str), Uint128> = Map::new("sponsor_totals");

/// Monotonic badge id.
pub const NEXT_BADGE_ID: Item<u64> = Item::new("next_badge_id");

/// Monotonic moderation report id.
pub const NEXT_REPORT_ID: Item<u64> = Item::new("next_report_id");

/// id => Badge
pub const BADGES: Map<u64, Badge> = Map::new("badges");

/// (recipient, id) => () — contributor trophy wall index
pub const BADGES_BY_RECIPIENT: Map<(&Addr, u64), ()> = Map::new("badges_by_recipient");

/// (repo_owner, repo_name, id) => () — per-repo awarded index
pub const BADGES_BY_REPO: Map<(&Addr, &str, u64), ()> = Map::new("badges_by_repo");

pub const REPORTS: Map<u64, ModerationReport> = Map::new("moderation_reports");

/// (release version, platform) => immutable SHA-256 registration.
pub const RELEASE_ARTIFACTS: Map<(&str, &str), ReleaseArtifact> = Map::new("release_artifacts");

/// Pending owner-initiated transfer keyed by the current owner/repo.
pub const OWNERSHIP_TRANSFERS: Map<(&Addr, &str), OwnershipTransfer> =
    Map::new("ownership_transfers");

/// Pending guardian recovery keyed by the current owner/repo.
pub const RECOVERY_PROPOSALS: Map<(&Addr, &str), RecoveryProposal> =
    Map::new("recovery_proposals");

/// Guardian threshold keyed by the current owner/repo.
pub const GUARDIAN_CONFIGS: Map<(&Addr, &str), GuardianConfig> = Map::new("guardian_configs");

/// (current owner, repo, guardian) => membership marker.
pub const GUARDIANS: Map<(&Addr, &str, &Addr), ()> = Map::new("guardians");

/// At most one pending global contract upgrade.
pub const UPGRADE_PROPOSAL: Item<UpgradeProposal> = Item::new("upgrade_proposal");
