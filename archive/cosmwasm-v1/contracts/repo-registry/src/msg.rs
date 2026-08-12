use cosmwasm_schema::{cw_serde, QueryResponses};
use cosmwasm_std::{Coin, Uint128};

use crate::state::{Badge, ModerationStatus, ReportStatus, Role, SplitEntry};

#[cw_serde]
pub struct InstantiateMsg {
    /// Optional admin; defaults to the instantiator.
    pub admin: Option<String>,
    /// Optional moderation committee; falls back to admin when unset.
    pub moderation_committee: Option<String>,
    /// Treasury for platform fees + username fees; defaults to admin (§3).
    pub treasury: Option<String>,
    /// Platform fee in bps; defaults to 300, hard cap 500 (§3).
    pub platform_fee_bps: Option<u16>,
    /// Username deposit; defaults to 0.1 INJ (§4, testnet default).
    pub username_deposit: Option<Coin>,
    /// Non-refundable username registration fee; defaults to zero for
    /// backwards-compatible testnet deployments.
    pub username_fee: Option<Coin>,
    /// Reserved usernames; defaults to the platform's built-in list.
    pub reserved_usernames: Option<Vec<String>>,
}

/// Migration message (cw2-gated; the scheduled Wasm hash and 14-day delay are
/// enforced by the contract in addition to the chain-level admin).
#[cw_serde]
pub struct MigrateMsg {
    #[serde(default)]
    pub wasm_sha256: Option<String>,
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
    /// Cancel a pending owner-initiated transfer.
    CancelOwnershipTransfer { repo: String },
    /// Accept a pending transfer after its timelock.
    AcceptOwnership { owner: String, repo: String },
    /// Configure guardian addresses and the approval threshold. Owner only.
    SetGuardians {
        repo: String,
        guardians: Vec<String>,
        threshold: u8,
    },
    /// Guardian recovery flow: start a replacement-owner proposal.
    ProposeRecovery {
        owner: String,
        repo: String,
        new_owner: String,
    },
    /// Add the sender's guardian approval to a recovery proposal.
    ApproveRecovery { owner: String, repo: String },
    /// Current owner vetoes a pending recovery proposal.
    CancelRecovery { repo: String },
    /// Proposed owner accepts a guardian recovery after its timelock.
    AcceptRecovery { owner: String, repo: String },
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
    /// Submit an auditable content report. Anyone may submit one.
    SubmitModerationReport {
        owner: String,
        repo: String,
        reason_hash: String,
    },
    /// Committee resolves an open report and applies the moderation status.
    ResolveModerationReport {
        report_id: u64,
        status: ModerationStatus,
        reason_hash: String,
    },
    /// Repository owner appeals a resolved report.
    AppealModerationReport { report_id: u64, reason_hash: String },
    /// Committee resolves an appeal and applies the final status.
    ResolveModerationAppeal {
        report_id: u64,
        status: ModerationStatus,
        reason_hash: String,
    },
    /// Sponsor a repository: attached funds are split instantly — platform
    /// fee to the treasury, the rest per the repo's revenue-split table
    /// (remainder to the owner). No funds are custodied (§3).
    Sponsor {
        owner: String,
        repo: String,
        /// Free-text note for the sponsor wall (≤256 chars, event-logged).
        message: Option<String>,
    },
    /// Replace the repo's revenue-split table. Owner only; shares are not
    /// transferable by recipients (§3, L3/L4 forbidden).
    SetRevenueSplits {
        repo: String,
        splits: Vec<SplitRecipient>,
    },
    /// Register a username for the sender. First-come-first-served; the
    /// exact configured deposit must be attached and is refunded on
    /// release (§4).
    RegisterUsername { name: String },
    /// Release the sender's username and refund the deposit.
    ReleaseUsername {},
    /// Update treasury and/or fee. Admin only; fee hard-capped at 500 bps.
    SetFeeConfig {
        treasury: Option<String>,
        platform_fee_bps: Option<u16>,
    },
    /// Update username fee and/or reserved names. Admin only.
    SetUsernamePolicy {
        username_fee: Option<Coin>,
        reserved_usernames: Option<Vec<String>>,
    },
    /// Announce the exact next Wasm SHA-256. Admin/multisig only; migration is
    /// blocked until the 14-day timelock expires.
    ScheduleUpgrade { wasm_sha256: String },
    /// Cancel a pending upgrade before migration. Admin/multisig only.
    CancelUpgrade {},
    /// Fork a repository into the sender's namespace: repo metadata and all
    /// refs (pack URI lists) are copied — the underlying IPFS content is
    /// shared, so forking costs no storage re-upload.
    ForkRepo {
        owner: String,
        repo: String,
        /// Name under the sender; defaults to the source repo name.
        name: Option<String>,
    },
    /// Award a non-transferable contribution badge (§3 L1). Repo owner only;
    /// honorary, carries no monetary rights.
    AwardBadge {
        repo: String,
        recipient: String,
        /// Why this contributor earned it (≤256 chars).
        reason: String,
    },
    /// Register immutable SHA-256 checksums for a release. Admin/governance
    /// only; users can verify the record without trusting GitHub metadata.
    RegisterRelease {
        version: String,
        artifacts: Vec<ReleaseArtifactInput>,
    },
}

/// String-typed split entry used in messages (validated into `SplitEntry`).
#[cw_serde]
pub struct SplitRecipient {
    pub address: String,
    pub bps: u16,
}

/// One immutable release artifact checksum to register.
#[cw_serde]
pub struct ReleaseArtifactInput {
    pub platform: String,
    pub sha256: String,
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
    /// Pending contract upgrade announcement and execution time.
    #[returns(UpgradeSecurityResponse)]
    UpgradeSecurity {},
    /// Resolve a username to its owning address.
    #[returns(UsernameResponse)]
    ResolveUsername { name: String },
    /// Reverse lookup: the username held by an address (if any).
    #[returns(AddressUsernameResponse)]
    AddressUsername { address: String },
    /// Revenue-split table of a repository.
    #[returns(RevenueSplitsResponse)]
    RevenueSplits { owner: String, repo: String },
    /// Lifetime sponsorship totals of a repository.
    #[returns(SponsorTotalsResponse)]
    SponsorTotals { owner: String, repo: String },
    /// Badges held by a contributor (trophy wall), newest-id pagination.
    #[returns(BadgesResponse)]
    BadgesByRecipient {
        recipient: String,
        start_after: Option<u64>,
        limit: Option<u32>,
    },
    /// Badges awarded by a repository.
    #[returns(BadgesResponse)]
    BadgesByRepo {
        owner: String,
        repo: String,
        start_after: Option<u64>,
        limit: Option<u32>,
    },
    /// Checksums registered for one release version.
    #[returns(ReleaseArtifactsResponse)]
    ReleaseArtifacts { version: String },
    /// Pending owner transfer and guardian recovery status.
    #[returns(OwnershipSecurityResponse)]
    OwnershipSecurity { owner: String, repo: String },
    /// Fetch one moderation report and its audit trail.
    #[returns(ModerationReportResponse)]
    ModerationReport { report_id: u64 },
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
    pub forked_from: Option<String>,
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
    pub treasury: String,
    pub platform_fee_bps: u16,
    pub username_deposit: Coin,
    pub username_fee: Coin,
    pub reserved_usernames: Vec<String>,
}

#[cw_serde]
pub struct UsernameResponse {
    pub name: String,
    pub owner: String,
    pub registered_at: u64,
}

#[cw_serde]
pub struct AddressUsernameResponse {
    pub address: String,
    pub name: Option<String>,
}

#[cw_serde]
pub struct RevenueSplitsResponse {
    pub owner: String,
    pub repo: String,
    /// Empty table means 100% to the owner (after platform fee).
    pub splits: Vec<SplitEntry>,
}

#[cw_serde]
pub struct SponsorTotal {
    pub denom: String,
    pub amount: Uint128,
}

#[cw_serde]
pub struct SponsorTotalsResponse {
    pub owner: String,
    pub repo: String,
    pub totals: Vec<SponsorTotal>,
}

#[cw_serde]
pub struct BadgesResponse {
    pub badges: Vec<Badge>,
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

#[cw_serde]
pub struct ReleaseArtifactInfo {
    pub version: String,
    pub platform: String,
    pub sha256: String,
    pub registered_by: String,
    pub registered_at: u64,
}

#[cw_serde]
pub struct ReleaseArtifactsResponse {
    pub version: String,
    pub artifacts: Vec<ReleaseArtifactInfo>,
}

#[cw_serde]
pub struct OwnershipTransferInfo {
    pub new_owner: String,
    pub proposed_at: u64,
    pub execute_after: u64,
}

#[cw_serde]
pub struct RecoveryProposalInfo {
    pub new_owner: String,
    pub proposed_at: u64,
    pub execute_after: u64,
    pub approvals: Vec<String>,
}

#[cw_serde]
pub struct OwnershipSecurityResponse {
    pub transfer: Option<OwnershipTransferInfo>,
    pub recovery: Option<RecoveryProposalInfo>,
    pub guardians: Vec<String>,
    pub guardian_threshold: u8,
}

#[cw_serde]
pub struct UpgradeProposalInfo {
    pub wasm_sha256: String,
    pub proposed_at: u64,
    pub execute_after: u64,
}

#[cw_serde]
pub struct UpgradeSecurityResponse {
    pub proposal: Option<UpgradeProposalInfo>,
    pub timelock_seconds: u64,
}

#[cw_serde]
pub struct ModerationReportResponse {
    pub id: u64,
    pub owner: String,
    pub repo: String,
    pub reporter: String,
    pub reason_hash: String,
    pub status: ReportStatus,
    pub resolution: Option<ModerationStatus>,
    pub resolution_hash: Option<String>,
    pub appeal_hash: Option<String>,
    pub created_at: u64,
    pub updated_at: u64,
}
