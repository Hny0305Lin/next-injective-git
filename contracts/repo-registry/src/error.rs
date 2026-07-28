use cosmwasm_std::StdError;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ContractError {
    #[error("{0}")]
    Std(#[from] StdError),

    #[error("unauthorized")]
    Unauthorized {},

    #[error("repo already exists: {name}")]
    RepoExists { name: String },

    #[error("repo not found: {owner}/{name}")]
    RepoNotFound { owner: String, name: String },

    #[error("ref not found: {ref_name}")]
    RefNotFound { ref_name: String },

    #[error(
        "ref conflict on {ref_name}: expected {expected}, actual {actual} (fetch first or force push)"
    )]
    RefConflict {
        ref_name: String,
        expected: String,
        actual: String,
    },

    #[error("invalid repo name: {name} (allowed: [a-zA-Z0-9._-], 1..=64 chars)")]
    InvalidRepoName { name: String },

    #[error("invalid ref name: {ref_name} (must start with 'refs/')")]
    InvalidRefName { ref_name: String },

    #[error("invalid commit sha: {sha} (expected 40 or 64 hex chars)")]
    InvalidCommitSha { sha: String },

    #[error("packfile_cids must not be empty")]
    EmptyPackfileCids {},

    #[error("owner cannot be a collaborator of their own repo")]
    OwnerAsCollaborator {},
}
