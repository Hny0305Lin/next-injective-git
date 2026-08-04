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

    #[error("username is reserved: {name}")]
    UsernameReserved { name: String },

    #[error("address already holds username: {name}")]
    AlreadyHasUsername { name: String },

    #[error("username not found: {name}")]
    UsernameNotFound { name: String },

    #[error("deposit mismatch: expected {expected}, got {actual}")]
    DepositMismatch { expected: String, actual: String },

    #[error("invalid username policy: {reason}")]
    InvalidUsernamePolicy { reason: String },

    #[error("release must contain at least one artifact")]
    EmptyReleaseArtifacts {},

    #[error("invalid release {field}: {value}")]
    InvalidReleaseField { field: String, value: String },

    #[error("release artifact already registered: {version}/{platform}")]
    ReleaseArtifactExists { version: String, platform: String },

    #[error("ownership transfer already pending")]
    OwnershipTransferPending {},

    #[error("no ownership transfer pending")]
    NoOwnershipTransfer {},

    #[error("ownership transfer cannot execute before {execute_after}")]
    OwnershipTransferTooEarly { execute_after: u64 },

    #[error("recovery proposal already pending")]
    RecoveryPending {},

    #[error("no recovery proposal pending")]
    NoRecoveryProposal {},

    #[error("guardian configuration invalid: {reason}")]
    InvalidGuardians { reason: String },

    #[error("recovery needs {required} approvals, got {actual}")]
    RecoveryApprovalsInsufficient { required: u8, actual: u8 },

    #[error("sender is not a configured guardian")]
    NotGuardian {},

    #[error("moderation report not found: {id}")]
    ReportNotFound { id: u64 },

    #[error("moderation report is not appealable")]
    ReportNotAppealable {},

    #[error("only the repository owner may appeal this report")]
    ReportAppealUnauthorized {},

    #[error("report reason hash must be non-empty and at most 128 characters")]
    InvalidReportReason {},

    #[error("upgrade hash must be a 64-character hexadecimal SHA-256")]
    InvalidUpgradeHash {},

    #[error("an upgrade is already scheduled")]
    UpgradeAlreadyScheduled {},

    #[error("no upgrade is scheduled")]
    NoUpgradeScheduled {},

    #[error("upgrade cannot execute before {execute_after}")]
    UpgradeTooEarly { execute_after: u64 },

    #[error("migration must provide the scheduled Wasm SHA-256")]
    UpgradeHashRequired {},

    #[error("migration Wasm SHA-256 does not match the scheduled hash")]
    UpgradeHashMismatch {},
}
