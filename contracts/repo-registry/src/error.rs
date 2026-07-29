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

    #[error("pack_uris must not be empty")]
    EmptyPackUris {},

    #[error("invalid pack uri: {uri} (expected <scheme>://<locator>, e.g. ipfs://<cid>)")]
    InvalidPackUri { uri: String },

    #[error("repo is frozen by moderation: {owner}/{name}")]
    RepoFrozen { owner: String, name: String },

    #[error("owner cannot be a collaborator of their own repo")]
    OwnerAsCollaborator {},

    #[error("platform fee {bps} bps exceeds hard cap {max} bps")]
    FeeTooHigh { bps: u16, max: u16 },

    #[error("sponsorship requires attached funds")]
    NoFunds {},

    #[error("revenue split table invalid: {reason}")]
    InvalidSplits { reason: String },

    #[error("invalid username: {name} (3-32 chars, [a-z0-9-], no leading/trailing '-', not an address)")]
    InvalidUsername { name: String },

    #[error("username already taken: {name}")]
    UsernameTaken { name: String },

    #[error("address already holds username: {name}")]
    AlreadyHasUsername { name: String },

    #[error("username not found: {name}")]
    UsernameNotFound { name: String },

    #[error("deposit mismatch: expected {expected}, got {actual}")]
    DepositMismatch { expected: String, actual: String },
}
