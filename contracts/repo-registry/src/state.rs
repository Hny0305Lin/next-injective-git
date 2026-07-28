use cosmwasm_schema::cw_serde;
use cosmwasm_std::Addr;
use cw_storage_plus::{Item, Map};

/// Global contract config.
#[cw_serde]
pub struct Config {
    /// Contract admin (can migrate / future governance hooks).
    pub admin: Addr,
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
}

/// A single git ref (branch/tag) entry.
#[cw_serde]
pub struct RefEntry {
    /// Commit SHA the ref points to (hex, sha1 40 chars or sha256 64 chars).
    pub commit_sha: String,
    /// Ordered list of IPFS CIDs; fetching all packfiles and applying them in
    /// order reconstructs the full history reachable from `commit_sha`.
    pub packfile_cids: Vec<String>,
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
