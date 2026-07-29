use cosmwasm_schema::cw_serde;
use cosmwasm_std::Addr;
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
