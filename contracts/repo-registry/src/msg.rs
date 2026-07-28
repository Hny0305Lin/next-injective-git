use cosmwasm_schema::{cw_serde, QueryResponses};

use crate::state::Role;

#[cw_serde]
pub struct InstantiateMsg {
    /// Optional admin; defaults to the instantiator.
    pub admin: Option<String>,
}

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
        packfile_cids: Vec<String>,
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
    /// Resolve a single ref to its commit sha + packfile CIDs.
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
}

#[cw_serde]
pub struct RepoInfoResponse {
    pub owner: String,
    pub name: String,
    pub description: String,
    pub default_branch: String,
    pub created_at: u64,
    pub updated_at: u64,
}

#[cw_serde]
pub struct RefInfo {
    pub ref_name: String,
    pub commit_sha: String,
    pub packfile_cids: Vec<String>,
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
    pub packfile_cids: Vec<String>,
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
