use cosmwasm_schema::{cw_serde, QueryResponses};

use crate::state::{ModerationStatus, Role};

#[cw_serde]
pub struct InstantiateMsg {
    /// Optional admin; defaults to the instantiator.
    pub admin: Option<String>,
    /// Optional moderation committee; falls back to admin when unset.
    pub moderation_committee: Option<String>,
}

/// Migration message (cw2-gated; admin authority is enforced by the chain's
/// wasm module via the contract-level admin).
#[cw_serde]
pub struct MigrateMsg {}

#[cw_serde]
pub enum ExecuteMsg {
    /// Register a new repository owned by the sender.
    CreateRepo {
        name: String,
        description: Option<String>,
        default_branch: Option<String>,
    },
    /// Update (or create) a ref. Sender must be owner or maintainer.
    /// `expected_sha` enables optimistic-concurrency / fast-forward checks:
    /// if set, the tx fails unless the current on-chain sha matches.
    /// `force` skips the check (force push).
    UpdateRef {
        owner: String,
        repo: String,
        ref_name: String,
        commit_sha: String,
        /// Storage URIs ("ipfs://<cid>", future "ar://...") of the packfiles.
        pack_uris: Vec<String>,
        expected_sha: Option<String>,
        force: bool,
    },
    /// Delete a ref. Sender must be owner or maintainer.
    DeleteRef {
        owner: String,
        repo: String,
        ref_name: String,
    },
    /// Add or update a collaborator. Owner only. `role = None` removes.
    SetCollaborator {
        repo: String,
        collaborator: String,
        role: Option<Role>,
    },
    /// Transfer repository ownership. Owner only.
    TransferOwnership {
        repo: String,
        new_owner: String,
    },
    /// Update repo metadata. Owner only.
    UpdateRepoInfo {
        repo: String,
        description: Option<String>,
        default_branch: Option<String>,
    },
    /// Set the moderation status of any repository. Committee only
    /// (admin when no committee is configured). Reasons live off-chain;
    /// `reason_hash` anchors the decision document (open-questions §5.3).
    SetModerationStatus {
        owner: String,
        repo: String,
        status: ModerationStatus,
        reason_hash: Option<String>,
    },
    /// Replace the moderation committee address. Admin only.
    SetModerationCommittee { committee: Option<String> },
}

#[cw_serde]
#[derive(QueryResponses)]
pub enum QueryMsg {
    /// Repository metadata.
    #[returns(RepoInfoResponse)]
    RepoInfo { owner: String, repo: String },
    /// All refs of a repository (paginated by ref name).
    #[returns(ListRefsResponse)]
    ListRefs {
        owner: String,
        repo: String,
        start_after: Option<String>,
        limit: Option<u32>,
    },
    /// Repositories of an owner (paginated by repo name).
    #[returns(ListReposResponse)]
    ListRepos {
        owner: String,
        start_after: Option<String>,
        limit: Option<u32>,
    },
    /// Resolve a single ref to its commit sha + pack URIs.
    #[returns(ResolveRefResponse)]
    ResolveRef {
        owner: String,
        repo: String,
        ref_name: String,
    },
    /// Collaborators of a repository.
    #[returns(ListCollaboratorsResponse)]
    ListCollaborators {
        owner: String,
        repo: String,
        start_after: Option<String>,
        limit: Option<u32>,
    },
    /// Contract-level configuration (admin / committee).
    #[returns(ConfigResponse)]
    Config {},
}

#[cw_serde]
pub struct RepoInfoResponse {
    pub owner: String,
    pub name: String,
    pub description: String,
    pub default_branch: String,
    pub created_at: u64,
    pub updated_at: u64,
    pub moderation_status: ModerationStatus,
}

#[cw_serde]
pub struct RefInfo {
    pub ref_name: String,
    pub commit_sha: String,
    pub pack_uris: Vec<String>,
    pub updated_at: u64,
    pub updated_by: String,
}

#[cw_serde]
pub struct ListRefsResponse {
    pub refs: Vec<RefInfo>,
}

#[cw_serde]
pub struct ListReposResponse {
    pub repos: Vec<RepoInfoResponse>,
}

#[cw_serde]
pub struct ResolveRefResponse {
    pub ref_name: String,
    pub commit_sha: String,
    pub pack_uris: Vec<String>,
}

#[cw_serde]
pub struct ConfigResponse {
    pub admin: String,
    pub moderation_committee: Option<String>,
}

#[cw_serde]
pub struct CollaboratorInfo {
    pub address: String,
    pub role: Role,
}

#[cw_serde]
pub struct ListCollaboratorsResponse {
    pub collaborators: Vec<CollaboratorInfo>,
}
