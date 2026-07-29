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
