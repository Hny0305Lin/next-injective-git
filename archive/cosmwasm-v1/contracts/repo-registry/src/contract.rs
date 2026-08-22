#[cfg(not(feature = "library"))]
use cosmwasm_std::entry_point;
use cosmwasm_std::{
    to_json_binary, Addr, BankMsg, Binary, Coin, Deps, DepsMut, Env, MessageInfo, Order, Response,
    StdResult, Uint128,
};
use cw_storage_plus::Bound;
use std::collections::BTreeMap;

use crate::error::ContractError;
use crate::msg::{
    AddressUsernameResponse, BadgesResponse, CollaboratorInfo, ConfigResponse, ExecuteMsg,
    InstantiateMsg, ListCollaboratorsResponse, ListRefsResponse, ListReposResponse, MigrateMsg,
    QueryMsg, RefInfo, ReleaseArtifactInfo, ReleaseArtifactInput, ReleaseArtifactsResponse,
    RepoInfoResponse, ResolveRefResponse, RevenueSplitsResponse, SplitRecipient, SponsorTotal,
    SponsorTotalsResponse, UsernameResponse, OwnershipSecurityResponse, OwnershipTransferInfo,
    RecoveryProposalInfo, ModerationReportResponse, UpgradeProposalInfo, UpgradeSecurityResponse,
};
use crate::state::{
    Badge, Config, ModerationStatus, Repo, RefEntry, Role, SplitEntry, UsernameRecord,
    ADDR_TO_NAME, BADGES, BADGES_BY_RECIPIENT, BADGES_BY_REPO, COLLABORATORS, CONFIG,
    DEFAULT_PLATFORM_FEE_BPS, GUARDIANS, GUARDIAN_CONFIGS, MAX_PLATFORM_FEE_BPS, NEXT_BADGE_ID,
    OWNERSHIP_TRANSFERS, RECOVERY_PROPOSALS, REFS, RELEASE_ARTIFACTS, REPOS, REVENUE_SPLITS,
    NEXT_REPORT_ID, REPORTS, SPONSOR_TOTALS, USERNAMES, UpgradeProposal, UPGRADE_PROPOSAL,
    UPGRADE_TIMELOCK_SECONDS,
};

const CONTRACT_NAME: &str = "crates.io:igit-repo-registry";
const CONTRACT_VERSION: &str = env!("CARGO_PKG_VERSION");

const DEFAULT_LIMIT: u32 = 30;
const MAX_LIMIT: u32 = 100;

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn instantiate(
    deps: DepsMut,
    _env: Env,
    info: MessageInfo,
    msg: InstantiateMsg,
) -> Result<Response, ContractError> {
    cw2::set_contract_version(deps.storage, CONTRACT_NAME, CONTRACT_VERSION)?;
    let admin = match msg.admin {
        Some(a) => deps.api.addr_validate(&a)?,
        None => info.sender,
    };
    let moderation_committee = msg
        .moderation_committee
        .map(|c| deps.api.addr_validate(&c))
        .transpose()?;
    let treasury = match msg.treasury {
        Some(t) => deps.api.addr_validate(&t)?,
        None => admin.clone(),
    };
    let platform_fee_bps = msg.platform_fee_bps.unwrap_or(DEFAULT_PLATFORM_FEE_BPS);
    if platform_fee_bps > MAX_PLATFORM_FEE_BPS {
        return Err(ContractError::FeeTooHigh {
            bps: platform_fee_bps,
            max: MAX_PLATFORM_FEE_BPS,
        });
    }
    // testnet default: 0.1 INJ (18 decimals)
    let username_deposit = msg.username_deposit.unwrap_or(Coin {
        denom: "inj".to_string(),
        amount: Uint128::new(100_000_000_000_000_000),
    });
    let username_fee = msg.username_fee.unwrap_or(Coin {
        denom: username_deposit.denom.clone(),
        amount: Uint128::zero(),
    });
    validate_username_fee(&username_deposit, &username_fee)?;
    let reserved_usernames = msg
        .reserved_usernames
        .unwrap_or_else(default_reserved_usernames);
    validate_reserved_usernames(&reserved_usernames)?;
    CONFIG.save(
        deps.storage,
        &Config {
            admin: admin.clone(),
            moderation_committee,
            treasury,
            platform_fee_bps,
            username_deposit,
            username_fee,
            reserved_usernames,
        },
    )?;
    Ok(Response::new()
        .add_attribute("action", "instantiate")
        .add_attribute("admin", admin))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn migrate(deps: DepsMut, env: Env, msg: MigrateMsg) -> Result<Response, ContractError> {
    // reject wasm blobs of a different contract; version-specific state
    // transforms hook in here as the schema evolves.
    let stored = cw2::get_contract_version(deps.storage)?;
    if stored.contract != CONTRACT_NAME {
        return Err(ContractError::Unauthorized {});
    }
    let proposal = UPGRADE_PROPOSAL
        .may_load(deps.storage)?
        .ok_or(ContractError::NoUpgradeScheduled {})?;
    if env.block.time.seconds() < proposal.execute_after {
        return Err(ContractError::UpgradeTooEarly {
            execute_after: proposal.execute_after,
        });
    }
    let provided_hash = msg
        .wasm_sha256
        .ok_or(ContractError::UpgradeHashRequired {})?;
    if provided_hash.to_ascii_lowercase() != proposal.wasm_sha256 {
        return Err(ContractError::UpgradeHashMismatch {});
    }
    UPGRADE_PROPOSAL.remove(deps.storage);
    cw2::set_contract_version(deps.storage, CONTRACT_NAME, CONTRACT_VERSION)?;
    Ok(Response::new()
        .add_attribute("action", "migrate")
        .add_attribute("from_version", stored.version)
        .add_attribute("to_version", CONTRACT_VERSION)
        .add_attribute("upgrade_timelock", UPGRADE_TIMELOCK_SECONDS.to_string()))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn execute(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    msg: ExecuteMsg,
) -> Result<Response, ContractError> {
    match msg {
        ExecuteMsg::CreateRepo {
            name,
            description,
            default_branch,
        } => exec_create_repo(deps, env, info, name, description, default_branch),
        ExecuteMsg::UpdateRef {
            owner,
            repo,
            ref_name,
            commit_sha,
            pack_uris,
            expected_sha,
            force,
        } => exec_update_ref(
            deps, env, info, owner, repo, ref_name, commit_sha, pack_uris, expected_sha, force,
        ),
        ExecuteMsg::DeleteRef {
            owner,
            repo,
            ref_name,
        } => exec_delete_ref(deps, env, info, owner, repo, ref_name),
        ExecuteMsg::SetCollaborator {
            repo,
            collaborator,
            role,
        } => exec_set_collaborator(deps, info, repo, collaborator, role),
        ExecuteMsg::TransferOwnership { repo, new_owner } => {
            exec_transfer_ownership(deps, env, info, repo, new_owner)
        }
        ExecuteMsg::CancelOwnershipTransfer { repo } => {
            exec_cancel_ownership_transfer(deps, info, repo)
        }
        ExecuteMsg::AcceptOwnership { owner, repo } => {
            exec_accept_ownership(deps, env, info, owner, repo)
        }
        ExecuteMsg::SetGuardians {
            repo,
            guardians,
            threshold,
        } => exec_set_guardians(deps, info, repo, guardians, threshold),
        ExecuteMsg::ProposeRecovery {
            owner,
            repo,
            new_owner,
        } => exec_propose_recovery(deps, env, info, owner, repo, new_owner),
        ExecuteMsg::ApproveRecovery { owner, repo } => {
            exec_approve_recovery(deps, info, owner, repo)
        }
        ExecuteMsg::CancelRecovery { repo } => exec_cancel_recovery(deps, info, repo),
        ExecuteMsg::AcceptRecovery { owner, repo } => {
            exec_accept_recovery(deps, env, info, owner, repo)
        }
        ExecuteMsg::UpdateRepoInfo {
            repo,
            description,
            default_branch,
        } => exec_update_repo_info(deps, env, info, repo, description, default_branch),
        ExecuteMsg::SetModerationStatus {
            owner,
            repo,
            status,
            reason_hash,
        } => exec_set_moderation_status(deps, env, info, owner, repo, status, reason_hash),
        ExecuteMsg::SetModerationCommittee { committee } => {
            exec_set_moderation_committee(deps, info, committee)
        }
        ExecuteMsg::SubmitModerationReport {
            owner,
            repo,
            reason_hash,
        } => exec_submit_moderation_report(deps, env, info, owner, repo, reason_hash),
        ExecuteMsg::ResolveModerationReport {
            report_id,
            status,
            reason_hash,
        } => exec_resolve_moderation_report(deps, env, info, report_id, status, reason_hash),
        ExecuteMsg::AppealModerationReport {
            report_id,
            reason_hash,
        } => exec_appeal_moderation_report(deps, env, info, report_id, reason_hash),
        ExecuteMsg::ResolveModerationAppeal {
            report_id,
            status,
            reason_hash,
        } => exec_resolve_moderation_appeal(deps, env, info, report_id, status, reason_hash),
        ExecuteMsg::Sponsor {
            owner,
            repo,
            message,
        } => exec_sponsor(deps, info, owner, repo, message),
        ExecuteMsg::SetRevenueSplits { repo, splits } => {
            exec_set_revenue_splits(deps, info, repo, splits)
        }
        ExecuteMsg::RegisterUsername { name } => exec_register_username(deps, env, info, name),
        ExecuteMsg::ReleaseUsername {} => exec_release_username(deps, info),
        ExecuteMsg::SetFeeConfig {
            treasury,
            platform_fee_bps,
        } => exec_set_fee_config(deps, info, treasury, platform_fee_bps),
        ExecuteMsg::SetUsernamePolicy {
            username_fee,
            reserved_usernames,
        } => exec_set_username_policy(deps, info, username_fee, reserved_usernames),
        ExecuteMsg::ScheduleUpgrade { wasm_sha256 } => {
            exec_schedule_upgrade(deps, env, info, wasm_sha256)
        }
        ExecuteMsg::CancelUpgrade {} => exec_cancel_upgrade(deps, info),
        ExecuteMsg::ForkRepo { owner, repo, name } => {
            exec_fork_repo(deps, env, info, owner, repo, name)
        }
        ExecuteMsg::AwardBadge {
            repo,
            recipient,
            reason,
        } => exec_award_badge(deps, env, info, repo, recipient, reason),
        ExecuteMsg::RegisterRelease { version, artifacts } => {
            exec_register_release(deps, env, info, version, artifacts)
        }
    }
}

fn validate_repo_name(name: &str) -> Result<(), ContractError> {
    let ok = !name.is_empty()
        && name.len() <= 64
        && name
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '_' | '-'));
    if ok {
        Ok(())
    } else {
        Err(ContractError::InvalidRepoName {
            name: name.to_string(),
        })
    }
}

fn validate_ref_name(ref_name: &str) -> Result<(), ContractError> {
    let ok = ref_name.starts_with("refs/")
        && ref_name.len() <= 256
        && !ref_name.contains("..")
        && ref_name
            .chars()
            .all(|c| c.is_ascii_graphic() && c != '~' && c != '^' && c != ':' && c != '\\');
    if ok {
        Ok(())
    } else {
        Err(ContractError::InvalidRefName {
            ref_name: ref_name.to_string(),
        })
    }
}

fn validate_commit_sha(sha: &str) -> Result<(), ContractError> {
    let ok = (sha.len() == 40 || sha.len() == 64) && sha.chars().all(|c| c.is_ascii_hexdigit());
    if ok {
        Ok(())
    } else {
        Err(ContractError::InvalidCommitSha {
            sha: sha.to_string(),
        })
    }
}

/// URIs must look like `<scheme>://<locator>` (e.g. "ipfs://bafy...").
fn validate_pack_uri(uri: &str) -> Result<(), ContractError> {
    let ok = uri.len() <= 512
        && matches!(uri.split_once("://"), Some((scheme, rest))
            if !scheme.is_empty() && !rest.is_empty()
                && scheme.chars().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit()));
    if ok {
        Ok(())
    } else {
        Err(ContractError::InvalidPackUri {
            uri: uri.to_string(),
        })
    }
}

fn validate_release_token(field: &str, value: &str, max_len: usize) -> Result<(), ContractError> {
    let ok = !value.is_empty()
        && value.len() <= max_len
        && value
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '_' | '-'));
    if ok {
        Ok(())
    } else {
        Err(ContractError::InvalidReleaseField {
            field: field.to_string(),
            value: value.to_string(),
        })
    }
}

fn validate_release_sha256(value: &str) -> Result<(), ContractError> {
    let ok = value.len() == 64 && value.chars().all(|c| c.is_ascii_hexdigit());
    if ok {
        Ok(())
    } else {
        Err(ContractError::InvalidReleaseField {
            field: "sha256".to_string(),
            value: value.to_string(),
        })
    }
}

/// Frozen repos accept no ref writes (open-questions §5.3, L2).
fn ensure_not_frozen(repo: &Repo) -> Result<(), ContractError> {
    if repo.moderation_status == ModerationStatus::Frozen {
        return Err(ContractError::RepoFrozen {
            owner: repo.owner.to_string(),
            name: repo.name.clone(),
        });
    }
    Ok(())
}

fn load_repo(deps: Deps, owner: &Addr, name: &str) -> Result<Repo, ContractError> {
    REPOS
        .may_load(deps.storage, (owner, name))?
        .ok_or_else(|| ContractError::RepoNotFound {
            owner: owner.to_string(),
            name: name.to_string(),
        })
}

/// Sender must be the repo owner or a maintainer to push.
fn ensure_can_push(
    deps: Deps,
    owner: &Addr,
    repo_name: &str,
    sender: &Addr,
) -> Result<(), ContractError> {
    if sender == owner {
        return Ok(());
    }
    match COLLABORATORS.may_load(deps.storage, (owner, repo_name, sender))? {
        Some(Role::Maintainer) => Ok(()),
        _ => Err(ContractError::Unauthorized {}),
    }
}

fn exec_create_repo(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    name: String,
    description: Option<String>,
    default_branch: Option<String>,
) -> Result<Response, ContractError> {
    validate_repo_name(&name)?;
    if REPOS.has(deps.storage, (&info.sender, &name)) {
        return Err(ContractError::RepoExists { name });
    }
    let now = env.block.time.seconds();
    let repo = Repo {
        owner: info.sender.clone(),
        name: name.clone(),
        description: description.unwrap_or_default(),
        default_branch: default_branch.unwrap_or_else(|| "main".to_string()),
        created_at: now,
        updated_at: now,
        moderation_status: ModerationStatus::Active,
        forked_from: None,
    };
    REPOS.save(deps.storage, (&info.sender, &name), &repo)?;
    Ok(Response::new()
        .add_attribute("action", "create_repo")
        .add_attribute("owner", info.sender)
        .add_attribute("repo", name))
}

#[allow(clippy::too_many_arguments)]
fn exec_update_ref(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    owner: String,
    repo: String,
    ref_name: String,
    commit_sha: String,
    pack_uris: Vec<String>,
    expected_sha: Option<String>,
    force: bool,
) -> Result<Response, ContractError> {
    let owner = deps.api.addr_validate(&owner)?;
    validate_ref_name(&ref_name)?;
    validate_commit_sha(&commit_sha)?;
    if pack_uris.is_empty() {
        return Err(ContractError::EmptyPackUris {});
    }
    for uri in &pack_uris {
        validate_pack_uri(uri)?;
    }
    let mut repo_meta = load_repo(deps.as_ref(), &owner, &repo)?;
    ensure_not_frozen(&repo_meta)?;
    ensure_can_push(deps.as_ref(), &owner, &repo, &info.sender)?;

    let existing = REFS.may_load(deps.storage, (&owner, &repo, &ref_name))?;
    if !force {
        if let Some(prev) = &existing {
            // optimistic concurrency: client states what it thinks the tip is
            let expected = expected_sha.unwrap_or_default();
            if expected != prev.commit_sha {
                return Err(ContractError::RefConflict {
                    ref_name,
                    expected,
                    actual: prev.commit_sha.clone(),
                });
            }
        }
    }

    let now = env.block.time.seconds();
    // non-force pushes append packfiles (incremental history);
    // force pushes replace the URI list entirely.
    let uris = match (&existing, force) {
        (Some(prev), false) => {
            let mut all = prev.pack_uris.clone();
            for uri in pack_uris {
                if !all.contains(&uri) {
                    all.push(uri);
                }
            }
            all
        }
        _ => pack_uris,
    };
    let entry = RefEntry {
        commit_sha: commit_sha.clone(),
        pack_uris: uris,
        updated_at: now,
        updated_by: info.sender.clone(),
    };
    REFS.save(deps.storage, (&owner, &repo, &ref_name), &entry)?;
    repo_meta.updated_at = now;
    REPOS.save(deps.storage, (&owner, &repo), &repo_meta)?;

    Ok(Response::new()
        .add_attribute("action", "update_ref")
        .add_attribute("owner", owner)
        .add_attribute("repo", repo)
        .add_attribute("ref", ref_name)
        .add_attribute("commit_sha", commit_sha)
        .add_attribute("force", force.to_string()))
}

fn exec_delete_ref(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    owner: String,
    repo: String,
    ref_name: String,
) -> Result<Response, ContractError> {
    let owner = deps.api.addr_validate(&owner)?;
    let mut repo_meta = load_repo(deps.as_ref(), &owner, &repo)?;
    ensure_not_frozen(&repo_meta)?;
    ensure_can_push(deps.as_ref(), &owner, &repo, &info.sender)?;
    if !REFS.has(deps.storage, (&owner, &repo, &ref_name)) {
        return Err(ContractError::RefNotFound { ref_name });
    }
    REFS.remove(deps.storage, (&owner, &repo, &ref_name));
    repo_meta.updated_at = env.block.time.seconds();
    REPOS.save(deps.storage, (&owner, &repo), &repo_meta)?;
    Ok(Response::new()
        .add_attribute("action", "delete_ref")
        .add_attribute("owner", owner)
        .add_attribute("repo", repo)
        .add_attribute("ref", ref_name))
}

fn exec_set_collaborator(
    deps: DepsMut,
    info: MessageInfo,
    repo: String,
    collaborator: String,
    role: Option<Role>,
) -> Result<Response, ContractError> {
    let collaborator = deps.api.addr_validate(&collaborator)?;
    // only the owner manages collaborators; repo is looked up under sender
    load_repo(deps.as_ref(), &info.sender, &repo)?;
    if collaborator == info.sender {
        return Err(ContractError::OwnerAsCollaborator {});
    }
    let action = match role {
        Some(r) => {
            COLLABORATORS.save(deps.storage, (&info.sender, &repo, &collaborator), &r)?;
            "set_collaborator"
        }
        None => {
            COLLABORATORS.remove(deps.storage, (&info.sender, &repo, &collaborator));
            "remove_collaborator"
        }
    };
    Ok(Response::new()
        .add_attribute("action", action)
        .add_attribute("owner", info.sender)
        .add_attribute("repo", repo)
        .add_attribute("collaborator", collaborator))
}

const OWNERSHIP_TIMELOCK: u64 = 7 * 24 * 60 * 60;

fn exec_transfer_ownership(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    repo: String,
    new_owner: String,
) -> Result<Response, ContractError> {
    let new_owner = deps.api.addr_validate(&new_owner)?;
    if new_owner == info.sender {
        return Err(ContractError::InvalidGuardians {
            reason: "new owner must differ from current owner".to_string(),
        });
    }
    load_repo(deps.as_ref(), &info.sender, &repo)?;
    if REPOS.has(deps.storage, (&new_owner, &repo)) {
        return Err(ContractError::RepoExists { name: repo });
    }
    if OWNERSHIP_TRANSFERS.has(deps.storage, (&info.sender, &repo)) {
        return Err(ContractError::OwnershipTransferPending {});
    }
    if RECOVERY_PROPOSALS.has(deps.storage, (&info.sender, &repo)) {
        return Err(ContractError::RecoveryPending {});
    }
    let proposed_at = env.block.time.seconds();
    OWNERSHIP_TRANSFERS.save(
        deps.storage,
        (&info.sender, &repo),
        &crate::state::OwnershipTransfer {
            new_owner: new_owner.clone(),
            proposed_at,
            execute_after: proposed_at + OWNERSHIP_TIMELOCK,
        },
    )?;
    Ok(Response::new()
        .add_attribute("action", "transfer_ownership_started")
        .add_attribute("repo", repo)
        .add_attribute("old_owner", info.sender)
        .add_attribute("new_owner", new_owner)
        .add_attribute("execute_after", (proposed_at + OWNERSHIP_TIMELOCK).to_string()))
}

fn exec_cancel_ownership_transfer(
    deps: DepsMut,
    info: MessageInfo,
    repo: String,
) -> Result<Response, ContractError> {
    load_repo(deps.as_ref(), &info.sender, &repo)?;
    if !OWNERSHIP_TRANSFERS.has(deps.storage, (&info.sender, &repo)) {
        return Err(ContractError::NoOwnershipTransfer {});
    }
    OWNERSHIP_TRANSFERS.remove(deps.storage, (&info.sender, &repo));
    Ok(Response::new()
        .add_attribute("action", "transfer_ownership_cancelled")
        .add_attribute("repo", repo)
        .add_attribute("owner", info.sender))
}

fn exec_accept_ownership(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    owner: String,
    repo: String,
) -> Result<Response, ContractError> {
    let owner = deps.api.addr_validate(&owner)?;
    let pending = OWNERSHIP_TRANSFERS
        .may_load(deps.storage, (&owner, &repo))?
        .ok_or(ContractError::NoOwnershipTransfer {})?;
    if info.sender != pending.new_owner {
        return Err(ContractError::Unauthorized {});
    }
    let now = env.block.time.seconds();
    if now < pending.execute_after {
        return Err(ContractError::OwnershipTransferTooEarly {
            execute_after: pending.execute_after,
        });
    }
    OWNERSHIP_TRANSFERS.remove(deps.storage, (&owner, &repo));
    move_repo_ownership(deps, owner, pending.new_owner, repo, info.sender)
}

fn move_repo_ownership(
    deps: DepsMut,
    old_owner: Addr,
    new_owner: Addr,
    repo: String,
    accepted_by: Addr,
) -> Result<Response, ContractError> {
    let mut repo_meta = load_repo(deps.as_ref(), &old_owner, &repo)?;
    if REPOS.has(deps.storage, (&new_owner, &repo)) {
        return Err(ContractError::RepoExists { name: repo });
    }
    repo_meta.owner = new_owner.clone();
    REPOS.remove(deps.storage, (&old_owner, &repo));
    REPOS.save(deps.storage, (&new_owner, &repo), &repo_meta)?;

    let refs: Vec<(String, RefEntry)> = REFS
        .prefix((&old_owner, &repo))
        .range(deps.storage, None, None, Order::Ascending)
        .collect::<StdResult<_>>()?;
    for (ref_name, entry) in refs {
        REFS.remove(deps.storage, (&old_owner, &repo, &ref_name));
        REFS.save(deps.storage, (&new_owner, &repo, &ref_name), &entry)?;
    }

    let collabs: Vec<(Addr, Role)> = COLLABORATORS
        .prefix((&old_owner, &repo))
        .range(deps.storage, None, None, Order::Ascending)
        .collect::<StdResult<_>>()?;
    for (addr, role) in collabs {
        COLLABORATORS.remove(deps.storage, (&old_owner, &repo, &addr));
        if addr != new_owner {
            COLLABORATORS.save(deps.storage, (&new_owner, &repo, &addr), &role)?;
        }
    }

    REVENUE_SPLITS.remove(deps.storage, (&old_owner, &repo));
    let totals: Vec<(String, Uint128)> = SPONSOR_TOTALS
        .prefix((&old_owner, &repo))
        .range(deps.storage, None, None, Order::Ascending)
        .collect::<StdResult<_>>()?;
    for (denom, amount) in totals {
        SPONSOR_TOTALS.remove(deps.storage, (&old_owner, &repo, &denom));
        SPONSOR_TOTALS.save(deps.storage, (&new_owner, &repo, &denom), &amount)?;
    }

    // Guardians are personal recovery delegates; reset them after ownership
    // changes so the new owner explicitly chooses its own trust set.
    let guardians: Vec<Addr> = GUARDIANS
        .prefix((&old_owner, &repo))
        .range(deps.storage, None, None, Order::Ascending)
        .map(|item| item.map(|(addr, _)| addr))
        .collect::<StdResult<_>>()?;
    for guardian in guardians {
        GUARDIANS.remove(deps.storage, (&old_owner, &repo, &guardian));
    }
    GUARDIAN_CONFIGS.remove(deps.storage, (&old_owner, &repo));
    RECOVERY_PROPOSALS.remove(deps.storage, (&old_owner, &repo));

    Ok(Response::new()
        .add_attribute("action", "transfer_ownership")
        .add_attribute("repo", repo)
        .add_attribute("old_owner", old_owner)
        .add_attribute("new_owner", new_owner)
        .add_attribute("accepted_by", accepted_by))
}

fn ensure_guardian(deps: Deps, owner: &Addr, repo: &str, sender: &Addr) -> Result<(), ContractError> {
    if GUARDIANS.has(deps.storage, (owner, repo, sender)) {
        Ok(())
    } else {
        Err(ContractError::NotGuardian {})
    }
}

fn exec_set_guardians(
    deps: DepsMut,
    info: MessageInfo,
    repo: String,
    guardians: Vec<String>,
    threshold: u8,
) -> Result<Response, ContractError> {
    load_repo(deps.as_ref(), &info.sender, &repo)?;
    if guardians.is_empty() || guardians.len() > 10 || threshold == 0 || usize::from(threshold) > guardians.len() {
        return Err(ContractError::InvalidGuardians {
            reason: "need 1..=10 guardians and threshold between 1 and count".to_string(),
        });
    }
    let mut validated = Vec::with_capacity(guardians.len());
    for raw in guardians {
        let guardian = deps.api.addr_validate(&raw)?;
        if guardian == info.sender || validated.iter().any(|a: &Addr| a == &guardian) {
            return Err(ContractError::InvalidGuardians {
                reason: "guardian list contains owner or duplicate address".to_string(),
            });
        }
        validated.push(guardian);
    }
    let old: Vec<Addr> = GUARDIANS
        .prefix((&info.sender, &repo))
        .range(deps.storage, None, None, Order::Ascending)
        .map(|item| item.map(|(addr, _)| addr))
        .collect::<StdResult<_>>()?;
    for guardian in old {
        GUARDIANS.remove(deps.storage, (&info.sender, &repo, &guardian));
    }
    for guardian in &validated {
        GUARDIANS.save(deps.storage, (&info.sender, &repo, guardian), &())?;
    }
    GUARDIAN_CONFIGS.save(
        deps.storage,
        (&info.sender, &repo),
        &crate::state::GuardianConfig { threshold },
    )?;
    Ok(Response::new()
        .add_attribute("action", "set_guardians")
        .add_attribute("owner", info.sender)
        .add_attribute("repo", repo)
        .add_attribute("threshold", threshold.to_string()))
}

fn exec_propose_recovery(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    owner: String,
    repo: String,
    new_owner: String,
) -> Result<Response, ContractError> {
    let owner = deps.api.addr_validate(&owner)?;
    let new_owner = deps.api.addr_validate(&new_owner)?;
    load_repo(deps.as_ref(), &owner, &repo)?;
    ensure_guardian(deps.as_ref(), &owner, &repo, &info.sender)?;
    if new_owner == owner || REPOS.has(deps.storage, (&new_owner, &repo)) {
        return Err(ContractError::InvalidGuardians {
            reason: "recovery target must be a different address without the same repo".to_string(),
        });
    }
    if RECOVERY_PROPOSALS.has(deps.storage, (&owner, &repo)) {
        return Err(ContractError::RecoveryPending {});
    }
    if OWNERSHIP_TRANSFERS.has(deps.storage, (&owner, &repo)) {
        return Err(ContractError::OwnershipTransferPending {});
    }
    let proposed_at = env.block.time.seconds();
    RECOVERY_PROPOSALS.save(
        deps.storage,
        (&owner, &repo),
        &crate::state::RecoveryProposal {
            new_owner: new_owner.clone(),
            proposed_at,
            execute_after: proposed_at + OWNERSHIP_TIMELOCK,
            approvals: vec![info.sender.clone()],
        },
    )?;
    Ok(Response::new()
        .add_attribute("action", "recovery_proposed")
        .add_attribute("owner", owner)
        .add_attribute("repo", repo)
        .add_attribute("new_owner", new_owner)
        .add_attribute("execute_after", (proposed_at + OWNERSHIP_TIMELOCK).to_string()))
}

fn exec_approve_recovery(
    deps: DepsMut,
    info: MessageInfo,
    owner: String,
    repo: String,
) -> Result<Response, ContractError> {
    let owner = deps.api.addr_validate(&owner)?;
    ensure_guardian(deps.as_ref(), &owner, &repo, &info.sender)?;
    let mut proposal = RECOVERY_PROPOSALS
        .may_load(deps.storage, (&owner, &repo))?
        .ok_or(ContractError::NoRecoveryProposal {})?;
    if !proposal.approvals.iter().any(|a| a == &info.sender) {
        proposal.approvals.push(info.sender.clone());
        RECOVERY_PROPOSALS.save(deps.storage, (&owner, &repo), &proposal)?;
    }
    Ok(Response::new()
        .add_attribute("action", "recovery_approved")
        .add_attribute("owner", owner)
        .add_attribute("repo", repo)
        .add_attribute("guardian", info.sender)
        .add_attribute("approvals", proposal.approvals.len().to_string()))
}

fn exec_cancel_recovery(
    deps: DepsMut,
    info: MessageInfo,
    repo: String,
) -> Result<Response, ContractError> {
    load_repo(deps.as_ref(), &info.sender, &repo)?;
    if !RECOVERY_PROPOSALS.has(deps.storage, (&info.sender, &repo)) {
        return Err(ContractError::NoRecoveryProposal {});
    }
    RECOVERY_PROPOSALS.remove(deps.storage, (&info.sender, &repo));
    Ok(Response::new()
        .add_attribute("action", "recovery_cancelled")
        .add_attribute("owner", info.sender)
        .add_attribute("repo", repo))
}

fn exec_accept_recovery(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    owner: String,
    repo: String,
) -> Result<Response, ContractError> {
    let owner = deps.api.addr_validate(&owner)?;
    let proposal = RECOVERY_PROPOSALS
        .may_load(deps.storage, (&owner, &repo))?
        .ok_or(ContractError::NoRecoveryProposal {})?;
    if info.sender != proposal.new_owner {
        return Err(ContractError::Unauthorized {});
    }
    if env.block.time.seconds() < proposal.execute_after {
        return Err(ContractError::OwnershipTransferTooEarly {
            execute_after: proposal.execute_after,
        });
    }
    let threshold = GUARDIAN_CONFIGS
        .may_load(deps.storage, (&owner, &repo))?
        .ok_or_else(|| ContractError::InvalidGuardians {
            reason: "guardian threshold is not configured".to_string(),
        })?
        .threshold;
    if proposal.approvals.len() < usize::from(threshold) {
        return Err(ContractError::RecoveryApprovalsInsufficient {
            required: threshold,
            actual: proposal.approvals.len() as u8,
        });
    }
    RECOVERY_PROPOSALS.remove(deps.storage, (&owner, &repo));
    move_repo_ownership(deps, owner, proposal.new_owner, repo, info.sender)
}

fn exec_update_repo_info(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    repo: String,
    description: Option<String>,
    default_branch: Option<String>,
) -> Result<Response, ContractError> {
    let mut repo_meta = load_repo(deps.as_ref(), &info.sender, &repo)?;
    if let Some(d) = description {
        repo_meta.description = d;
    }
    if let Some(b) = default_branch {
        repo_meta.default_branch = b;
    }
    repo_meta.updated_at = env.block.time.seconds();
    REPOS.save(deps.storage, (&info.sender, &repo), &repo_meta)?;
    Ok(Response::new()
        .add_attribute("action", "update_repo_info")
        .add_attribute("owner", info.sender)
        .add_attribute("repo", repo))
}

#[allow(clippy::too_many_arguments)]
fn exec_set_moderation_status(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    owner: String,
    repo: String,
    status: ModerationStatus,
    reason_hash: Option<String>,
) -> Result<Response, ContractError> {
    let cfg = CONFIG.load(deps.storage)?;
    let moderator = cfg.moderation_committee.as_ref().unwrap_or(&cfg.admin);
    if info.sender != *moderator {
        return Err(ContractError::Unauthorized {});
    }
    let owner = deps.api.addr_validate(&owner)?;
    let mut repo_meta = load_repo(deps.as_ref(), &owner, &repo)?;
    let status_str = format!("{status:?}").to_lowercase();
    repo_meta.moderation_status = status;
    repo_meta.updated_at = env.block.time.seconds();
    REPOS.save(deps.storage, (&owner, &repo), &repo_meta)?;
    Ok(Response::new()
        .add_attribute("action", "set_moderation_status")
        .add_attribute("owner", owner)
        .add_attribute("repo", repo)
        .add_attribute("status", status_str)
        .add_attribute("reason_hash", reason_hash.unwrap_or_default()))
}

fn exec_set_moderation_committee(
    deps: DepsMut,
    info: MessageInfo,
    committee: Option<String>,
) -> Result<Response, ContractError> {
    let mut cfg = CONFIG.load(deps.storage)?;
    if info.sender != cfg.admin {
        return Err(ContractError::Unauthorized {});
    }
    cfg.moderation_committee = committee
        .map(|c| deps.api.addr_validate(&c))
        .transpose()?;
    CONFIG.save(deps.storage, &cfg)?;
    Ok(Response::new()
        .add_attribute("action", "set_moderation_committee")
        .add_attribute(
            "committee",
            cfg.moderation_committee
                .map(|a| a.to_string())
                .unwrap_or_default(),
        ))
}

fn validate_report_reason(reason_hash: &str) -> Result<(), ContractError> {
    if reason_hash.is_empty() || reason_hash.len() > 128 {
        return Err(ContractError::InvalidReportReason {});
    }
    Ok(())
}

fn ensure_moderator(deps: Deps, sender: &Addr) -> Result<(), ContractError> {
    let cfg = CONFIG.load(deps.storage)?;
    let moderator = cfg.moderation_committee.as_ref().unwrap_or(&cfg.admin);
    if sender == moderator {
        Ok(())
    } else {
        Err(ContractError::Unauthorized {})
    }
}

fn exec_submit_moderation_report(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    owner: String,
    repo: String,
    reason_hash: String,
) -> Result<Response, ContractError> {
    validate_report_reason(&reason_hash)?;
    let owner_addr = deps.api.addr_validate(&owner)?;
    load_repo(deps.as_ref(), &owner_addr, &repo)?;
    let id = NEXT_REPORT_ID.may_load(deps.storage)?.unwrap_or(1);
    let now = env.block.time.seconds();
    REPORTS.save(
        deps.storage,
        id,
        &crate::state::ModerationReport {
            id,
            owner: owner_addr.clone(),
            repo: repo.clone(),
            reporter: info.sender.clone(),
            reason_hash: reason_hash.clone(),
            status: crate::state::ReportStatus::Open,
            resolution: None,
            resolution_hash: None,
            appeal_hash: None,
            created_at: now,
            updated_at: now,
        },
    )?;
    NEXT_REPORT_ID.save(deps.storage, &(id + 1))?;
    Ok(Response::new()
        .add_attribute("action", "submit_moderation_report")
        .add_attribute("report_id", id.to_string())
        .add_attribute("owner", owner_addr)
        .add_attribute("repo", repo)
        .add_attribute("reporter", info.sender)
        .add_attribute("reason_hash", reason_hash))
}

fn exec_resolve_moderation_report(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    report_id: u64,
    status: ModerationStatus,
    reason_hash: String,
) -> Result<Response, ContractError> {
    ensure_moderator(deps.as_ref(), &info.sender)?;
    validate_report_reason(&reason_hash)?;
    let mut report = REPORTS
        .may_load(deps.storage, report_id)?
        .ok_or(ContractError::ReportNotFound { id: report_id })?;
    if !matches!(report.status, crate::state::ReportStatus::Open) {
        return Err(ContractError::ReportNotAppealable {});
    }
    let mut repo = load_repo(deps.as_ref(), &report.owner, &report.repo)?;
    repo.moderation_status = status.clone();
    repo.updated_at = env.block.time.seconds();
    REPOS.save(deps.storage, (&report.owner, &report.repo), &repo)?;
    report.status = crate::state::ReportStatus::Resolved;
    report.resolution = Some(status.clone());
    report.resolution_hash = Some(reason_hash.clone());
    report.updated_at = env.block.time.seconds();
    REPORTS.save(deps.storage, report_id, &report)?;
    Ok(Response::new()
        .add_attribute("action", "resolve_moderation_report")
        .add_attribute("report_id", report_id.to_string())
        .add_attribute("status", format!("{status:?}").to_lowercase())
        .add_attribute("reason_hash", reason_hash))
}

fn exec_appeal_moderation_report(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    report_id: u64,
    reason_hash: String,
) -> Result<Response, ContractError> {
    validate_report_reason(&reason_hash)?;
    let mut report = REPORTS
        .may_load(deps.storage, report_id)?
        .ok_or(ContractError::ReportNotFound { id: report_id })?;
    if info.sender != report.owner {
        return Err(ContractError::ReportAppealUnauthorized {});
    }
    if !matches!(report.status, crate::state::ReportStatus::Resolved) {
        return Err(ContractError::ReportNotAppealable {});
    }
    report.status = crate::state::ReportStatus::Appealed;
    report.appeal_hash = Some(reason_hash.clone());
    report.updated_at = env.block.time.seconds();
    REPORTS.save(deps.storage, report_id, &report)?;
    Ok(Response::new()
        .add_attribute("action", "appeal_moderation_report")
        .add_attribute("report_id", report_id.to_string())
        .add_attribute("owner", info.sender)
        .add_attribute("reason_hash", reason_hash))
}

fn exec_resolve_moderation_appeal(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    report_id: u64,
    status: ModerationStatus,
    reason_hash: String,
) -> Result<Response, ContractError> {
    ensure_moderator(deps.as_ref(), &info.sender)?;
    validate_report_reason(&reason_hash)?;
    let mut report = REPORTS
        .may_load(deps.storage, report_id)?
        .ok_or(ContractError::ReportNotFound { id: report_id })?;
    if !matches!(report.status, crate::state::ReportStatus::Appealed) {
        return Err(ContractError::ReportNotAppealable {});
    }
    let mut repo = load_repo(deps.as_ref(), &report.owner, &report.repo)?;
    repo.moderation_status = status.clone();
    repo.updated_at = env.block.time.seconds();
    REPOS.save(deps.storage, (&report.owner, &report.repo), &repo)?;
    report.status = crate::state::ReportStatus::AppealResolved;
    report.resolution = Some(status.clone());
    report.resolution_hash = Some(reason_hash.clone());
    report.updated_at = env.block.time.seconds();
    REPORTS.save(deps.storage, report_id, &report)?;
    Ok(Response::new()
        .add_attribute("action", "resolve_moderation_appeal")
        .add_attribute("report_id", report_id.to_string())
        .add_attribute("status", format!("{status:?}").to_lowercase())
        .add_attribute("reason_hash", reason_hash))
}

/// Sponsor a repo: split attached funds instantly, custody nothing (§3).
fn exec_sponsor(
    deps: DepsMut,
    info: MessageInfo,
    owner: String,
    repo: String,
    message: Option<String>,
) -> Result<Response, ContractError> {
    if info.funds.is_empty() || info.funds.iter().all(|c| c.amount.is_zero()) {
        return Err(ContractError::NoFunds {});
    }
    let message = message.unwrap_or_default();
    if message.len() > 256 {
        return Err(ContractError::InvalidSplits {
            reason: "sponsor message too long (max 256)".to_string(),
        });
    }
    let owner = deps.api.addr_validate(&owner)?;
    let repo_meta = load_repo(deps.as_ref(), &owner, &repo)?;
    // Frozen repos must not keep earning (§5.3 L2: freeze the money flow).
    ensure_not_frozen(&repo_meta)?;

    let cfg = CONFIG.load(deps.storage)?;
    let splits = REVENUE_SPLITS
        .may_load(deps.storage, (&owner, &repo))?
        .unwrap_or_default();

    // recipient -> coins, deterministic order for stable msg output
    let mut payouts: BTreeMap<Addr, Vec<Coin>> = BTreeMap::new();
    let mut add_payout = |addr: &Addr, denom: &str, amount: Uint128| {
        if amount.is_zero() {
            return;
        }
        let coins = payouts.entry(addr.clone()).or_default();
        match coins.iter_mut().find(|c| c.denom == denom) {
            Some(c) => c.amount += amount,
            None => coins.push(Coin {
                denom: denom.to_string(),
                amount,
            }),
        }
    };

    for coin in &info.funds {
        if coin.amount.is_zero() {
            continue;
        }
        let fee = coin.amount.multiply_ratio(cfg.platform_fee_bps, 10_000u128);
        let distributable = coin.amount - fee;
        add_payout(&cfg.treasury, &coin.denom, fee);

        let mut assigned = Uint128::zero();
        for s in &splits {
            let share = distributable.multiply_ratio(s.bps, 10_000u128);
            assigned += share;
            add_payout(&s.address, &coin.denom, share);
        }
        // remainder (incl. rounding dust) always lands on the owner
        add_payout(&owner, &coin.denom, distributable - assigned);

        SPONSOR_TOTALS.update(
            deps.storage,
            (&owner, &repo, coin.denom.as_str()),
            |t| -> StdResult<_> { Ok(t.unwrap_or_default() + coin.amount) },
        )?;
    }

    let msgs: Vec<BankMsg> = payouts
        .into_iter()
        .map(|(to_address, amount)| BankMsg::Send {
            to_address: to_address.into_string(),
            amount,
        })
        .collect();

    let funds_str = info
        .funds
        .iter()
        .map(|c| format!("{}{}", c.amount, c.denom))
        .collect::<Vec<_>>()
        .join(",");
    Ok(Response::new()
        .add_messages(msgs)
        .add_attribute("action", "sponsor")
        .add_attribute("sponsor", info.sender)
        .add_attribute("owner", owner)
        .add_attribute("repo", repo)
        .add_attribute("funds", funds_str)
        .add_attribute("message", message))
}

/// Replace the revenue-split table (§3). Owner only.
fn exec_set_revenue_splits(
    deps: DepsMut,
    info: MessageInfo,
    repo: String,
    splits: Vec<SplitRecipient>,
) -> Result<Response, ContractError> {
    load_repo(deps.as_ref(), &info.sender, &repo)?;
    if splits.len() > 20 {
        return Err(ContractError::InvalidSplits {
            reason: "too many recipients (max 20)".to_string(),
        });
    }
    let mut total: u32 = 0;
    let mut validated: Vec<SplitEntry> = Vec::with_capacity(splits.len());
    for s in splits {
        let address = deps.api.addr_validate(&s.address)?;
        if s.bps == 0 {
            return Err(ContractError::InvalidSplits {
                reason: format!("zero share for {address}"),
            });
        }
        if address == info.sender {
            return Err(ContractError::InvalidSplits {
                reason: "owner receives the remainder implicitly".to_string(),
            });
        }
        if validated.iter().any(|v| v.address == address) {
            return Err(ContractError::InvalidSplits {
                reason: format!("duplicate recipient {address}"),
            });
        }
        total += u32::from(s.bps);
        validated.push(SplitEntry {
            address,
            bps: s.bps,
        });
    }
    if total > 10_000 {
        return Err(ContractError::InvalidSplits {
            reason: format!("shares sum to {total} bps (> 10000)"),
        });
    }
    if validated.is_empty() {
        REVENUE_SPLITS.remove(deps.storage, (&info.sender, &repo));
    } else {
        REVENUE_SPLITS.save(deps.storage, (&info.sender, &repo), &validated)?;
    }
    Ok(Response::new()
        .add_attribute("action", "set_revenue_splits")
        .add_attribute("owner", info.sender)
        .add_attribute("repo", repo)
        .add_attribute("total_bps", total.to_string()))
}

fn validate_username(name: &str) -> Result<(), ContractError> {
    let ok = (3..=32).contains(&name.len())
        && !name.starts_with('-')
        && !name.ends_with('-')
        && !name.starts_with("inj1") // never shadow bech32 addresses
        && name
            .chars()
            .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-');
    if ok {
        Ok(())
    } else {
        Err(ContractError::InvalidUsername {
            name: name.to_string(),
        })
    }
}

/// Register a deposit-backed username (§4).
fn exec_register_username(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    name: String,
) -> Result<Response, ContractError> {
    validate_username(&name)?;
    if USERNAMES.has(deps.storage, &name) {
        return Err(ContractError::UsernameTaken { name });
    }
    let cfg = CONFIG.load(deps.storage)?;
    if cfg.reserved_usernames.iter().any(|reserved| reserved == &name) {
        return Err(ContractError::UsernameReserved { name });
    }
    if let Some(existing) = ADDR_TO_NAME.may_load(deps.storage, &info.sender)? {
        return Err(ContractError::AlreadyHasUsername { name: existing });
    }
    let expected = &cfg.username_deposit;
    validate_username_fee(expected, &cfg.username_fee)?;
    let total = expected.amount + cfg.username_fee.amount;
    let paid_ok = info.funds.len() == 1
        && info.funds[0].denom == expected.denom
        && info.funds[0].amount == total;
    if !paid_ok {
        return Err(ContractError::DepositMismatch {
            expected: format!("{}{}", total, expected.denom),
            actual: info
                .funds
                .iter()
                .map(|c| format!("{}{}", c.amount, c.denom))
                .collect::<Vec<_>>()
                .join(","),
        });
    }
    USERNAMES.save(
        deps.storage,
        &name,
        &UsernameRecord {
            owner: info.sender.clone(),
            deposit: expected.clone(),
            registered_at: env.block.time.seconds(),
        },
    )?;
    ADDR_TO_NAME.save(deps.storage, &info.sender, &name)?;
    let mut response = Response::new()
        .add_attribute("action", "register_username")
        .add_attribute("name", name)
        .add_attribute("owner", info.sender);
    if !cfg.username_fee.amount.is_zero() {
        response = response.add_message(BankMsg::Send {
            to_address: cfg.treasury.to_string(),
            amount: vec![cfg.username_fee],
        });
    }
    Ok(response)
}

/// Release the sender's username and refund the escrowed deposit.
fn exec_release_username(deps: DepsMut, info: MessageInfo) -> Result<Response, ContractError> {
    let name = ADDR_TO_NAME
        .may_load(deps.storage, &info.sender)?
        .ok_or(ContractError::UsernameNotFound {
            name: "<none held by sender>".to_string(),
        })?;
    let record = USERNAMES.load(deps.storage, &name)?;
    USERNAMES.remove(deps.storage, &name);
    ADDR_TO_NAME.remove(deps.storage, &info.sender);
    Ok(Response::new()
        .add_message(BankMsg::Send {
            to_address: info.sender.to_string(),
            amount: vec![record.deposit],
        })
        .add_attribute("action", "release_username")
        .add_attribute("name", name)
        .add_attribute("owner", info.sender))
}

/// Update treasury / fee within the hard cap. Admin only.
fn exec_set_fee_config(
    deps: DepsMut,
    info: MessageInfo,
    treasury: Option<String>,
    platform_fee_bps: Option<u16>,
) -> Result<Response, ContractError> {
    let mut cfg = CONFIG.load(deps.storage)?;
    if info.sender != cfg.admin {
        return Err(ContractError::Unauthorized {});
    }
    if let Some(t) = treasury {
        cfg.treasury = deps.api.addr_validate(&t)?;
    }
    if let Some(bps) = platform_fee_bps {
        if bps > MAX_PLATFORM_FEE_BPS {
            return Err(ContractError::FeeTooHigh {
                bps,
                max: MAX_PLATFORM_FEE_BPS,
            });
        }
        cfg.platform_fee_bps = bps;
    }
    CONFIG.save(deps.storage, &cfg)?;
    Ok(Response::new()
        .add_attribute("action", "set_fee_config")
        .add_attribute("treasury", cfg.treasury)
        .add_attribute("platform_fee_bps", cfg.platform_fee_bps.to_string()))
}

fn exec_set_username_policy(
    deps: DepsMut,
    info: MessageInfo,
    username_fee: Option<Coin>,
    reserved_usernames: Option<Vec<String>>,
) -> Result<Response, ContractError> {
    let mut cfg = CONFIG.load(deps.storage)?;
    if info.sender != cfg.admin {
        return Err(ContractError::Unauthorized {});
    }
    if let Some(fee) = username_fee {
        validate_username_fee(&cfg.username_deposit, &fee)?;
        cfg.username_fee = fee;
    }
    if let Some(names) = reserved_usernames {
        validate_reserved_usernames(&names)?;
        cfg.reserved_usernames = names;
    }
    CONFIG.save(deps.storage, &cfg)?;
    Ok(Response::new()
        .add_attribute("action", "set_username_policy")
        .add_attribute("username_fee", format!("{}{}", cfg.username_fee.amount, cfg.username_fee.denom))
        .add_attribute("reserved_count", cfg.reserved_usernames.len().to_string()))
}

fn validate_upgrade_hash(hash: &str) -> Result<String, ContractError> {
    if hash.len() != 64 || !hash.chars().all(|c| c.is_ascii_hexdigit()) {
        return Err(ContractError::InvalidUpgradeHash {});
    }
    Ok(hash.to_ascii_lowercase())
}

fn exec_schedule_upgrade(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    wasm_sha256: String,
) -> Result<Response, ContractError> {
    let cfg = CONFIG.load(deps.storage)?;
    if info.sender != cfg.admin {
        return Err(ContractError::Unauthorized {});
    }
    if UPGRADE_PROPOSAL.may_load(deps.storage)?.is_some() {
        return Err(ContractError::UpgradeAlreadyScheduled {});
    }
    let wasm_sha256 = validate_upgrade_hash(&wasm_sha256)?;
    let proposed_at = env.block.time.seconds();
    let execute_after = proposed_at + UPGRADE_TIMELOCK_SECONDS;
    UPGRADE_PROPOSAL.save(
        deps.storage,
        &UpgradeProposal {
            wasm_sha256: wasm_sha256.clone(),
            proposed_at,
            execute_after,
        },
    )?;
    Ok(Response::new()
        .add_attribute("action", "schedule_upgrade")
        .add_attribute("wasm_sha256", wasm_sha256)
        .add_attribute("proposed_at", proposed_at.to_string())
        .add_attribute("execute_after", execute_after.to_string()))
}

fn exec_cancel_upgrade(deps: DepsMut, info: MessageInfo) -> Result<Response, ContractError> {
    let cfg = CONFIG.load(deps.storage)?;
    if info.sender != cfg.admin {
        return Err(ContractError::Unauthorized {});
    }
    if UPGRADE_PROPOSAL.may_load(deps.storage)?.is_none() {
        return Err(ContractError::NoUpgradeScheduled {});
    }
    UPGRADE_PROPOSAL.remove(deps.storage);
    Ok(Response::new().add_attribute("action", "cancel_upgrade"))
}

/// Fork owner/repo into the sender's namespace. Refs are copied by
/// reference (same pack URIs) — IPFS content addressing dedupes storage.
fn exec_fork_repo(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    owner: String,
    repo: String,
    name: Option<String>,
) -> Result<Response, ContractError> {
    let src_owner = deps.api.addr_validate(&owner)?;
    let src = load_repo(deps.as_ref(), &src_owner, &repo)?;
    // moderated content must not spread through forks
    if src.moderation_status != ModerationStatus::Active {
        return Err(ContractError::RepoFrozen {
            owner: src_owner.to_string(),
            name: repo,
        });
    }
    let new_name = name.unwrap_or_else(|| repo.clone());
    validate_repo_name(&new_name)?;
    if src_owner == info.sender && new_name == repo {
        return Err(ContractError::RepoExists { name: new_name });
    }
    if REPOS.has(deps.storage, (&info.sender, &new_name)) {
        return Err(ContractError::RepoExists { name: new_name });
    }

    let now = env.block.time.seconds();
    let fork = Repo {
        owner: info.sender.clone(),
        name: new_name.clone(),
        description: src.description.clone(),
        default_branch: src.default_branch.clone(),
        created_at: now,
        updated_at: now,
        moderation_status: ModerationStatus::Active,
        forked_from: Some(format!("{src_owner}/{repo}")),
    };
    REPOS.save(deps.storage, (&info.sender, &new_name), &fork)?;

    let refs: Vec<(String, RefEntry)> = REFS
        .prefix((&src_owner, &repo))
        .range(deps.storage, None, None, Order::Ascending)
        .collect::<StdResult<_>>()?;
    let ref_count = refs.len();
    for (ref_name, mut entry) in refs {
        entry.updated_at = now;
        entry.updated_by = info.sender.clone();
        REFS.save(deps.storage, (&info.sender, &new_name, &ref_name), &entry)?;
    }

    Ok(Response::new()
        .add_attribute("action", "fork_repo")
        .add_attribute("source_owner", src_owner)
        .add_attribute("source_repo", repo)
        .add_attribute("owner", info.sender)
        .add_attribute("repo", new_name)
        .add_attribute("refs", ref_count.to_string()))
}

/// Award a non-transferable contribution badge (§3 L1). Owner only.
fn exec_award_badge(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    repo: String,
    recipient: String,
    reason: String,
) -> Result<Response, ContractError> {
    let recipient = deps.api.addr_validate(&recipient)?;
    let repo_meta = load_repo(deps.as_ref(), &info.sender, &repo)?;
    // frozen repos stop issuing badges (§5.3 L2 sanctions)
    if repo_meta.moderation_status != ModerationStatus::Active {
        return Err(ContractError::RepoFrozen {
            owner: info.sender.to_string(),
            name: repo,
        });
    }
    if recipient == info.sender {
        return Err(ContractError::OwnerAsCollaborator {});
    }
    if reason.is_empty() || reason.len() > 256 {
        return Err(ContractError::InvalidSplits {
            reason: "badge reason must be 1..=256 chars".to_string(),
        });
    }

    let id = NEXT_BADGE_ID.may_load(deps.storage)?.unwrap_or(1);
    NEXT_BADGE_ID.save(deps.storage, &(id + 1))?;
    let badge = Badge {
        id,
        repo_owner: info.sender.clone(),
        repo_name: repo.clone(),
        recipient: recipient.clone(),
        reason,
        awarded_by: info.sender.clone(),
        awarded_at: env.block.time.seconds(),
    };
    BADGES.save(deps.storage, id, &badge)?;
    BADGES_BY_RECIPIENT.save(deps.storage, (&recipient, id), &())?;
    BADGES_BY_REPO.save(deps.storage, (&info.sender, &repo, id), &())?;

    Ok(Response::new()
        .add_attribute("action", "award_badge")
        .add_attribute("id", id.to_string())
        .add_attribute("owner", info.sender)
        .add_attribute("repo", repo)
        .add_attribute("recipient", recipient))
}

/// Register immutable release artifact checksums. Corrections require a new
/// version rather than mutating an already published artifact.
fn exec_register_release(
    deps: DepsMut,
    env: Env,
    info: MessageInfo,
    version: String,
    artifacts: Vec<ReleaseArtifactInput>,
) -> Result<Response, ContractError> {
    let cfg = CONFIG.load(deps.storage)?;
    if info.sender != cfg.admin {
        return Err(ContractError::Unauthorized {});
    }
    validate_release_token("version", &version, 64)?;
    if artifacts.is_empty() {
        return Err(ContractError::EmptyReleaseArtifacts {});
    }
    if artifacts.len() > 32 {
        return Err(ContractError::InvalidReleaseField {
            field: "artifacts".to_string(),
            value: "too many artifacts (max 32)".to_string(),
        });
    }
    let mut seen = std::collections::BTreeSet::new();
    for artifact in &artifacts {
        validate_release_token("platform", &artifact.platform, 64)?;
        validate_release_sha256(&artifact.sha256)?;
        if !seen.insert(artifact.platform.clone()) {
            return Err(ContractError::InvalidReleaseField {
                field: "platform".to_string(),
                value: format!("duplicate platform {}", artifact.platform),
            });
        }
        if RELEASE_ARTIFACTS.has(deps.storage, (&version, &artifact.platform)) {
            return Err(ContractError::ReleaseArtifactExists {
                version: version.clone(),
                platform: artifact.platform.clone(),
            });
        }
    }
    let now = env.block.time.seconds();
    for artifact in artifacts {
        let platform = artifact.platform.clone();
        RELEASE_ARTIFACTS.save(
            deps.storage,
            (&version, &platform),
            &crate::state::ReleaseArtifact {
                version: version.clone(),
                platform: platform.clone(),
                sha256: artifact.sha256.to_ascii_lowercase(),
                registered_by: info.sender.clone(),
                registered_at: now,
            },
        )?;
    }
    Ok(Response::new()
        .add_attribute("action", "register_release")
        .add_attribute("version", version)
        .add_attribute("artifacts", seen.len().to_string()))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn query(deps: Deps, _env: Env, msg: QueryMsg) -> StdResult<Binary> {
    match msg {
        QueryMsg::RepoInfo { owner, repo } => to_json_binary(&query_repo_info(deps, owner, repo)?),
        QueryMsg::ListRefs {
            owner,
            repo,
            start_after,
            limit,
        } => to_json_binary(&query_list_refs(deps, owner, repo, start_after, limit)?),
        QueryMsg::ListRepos {
            owner,
            start_after,
            limit,
        } => to_json_binary(&query_list_repos(deps, owner, start_after, limit)?),
        QueryMsg::ResolveRef {
            owner,
            repo,
            ref_name,
        } => to_json_binary(&query_resolve_ref(deps, owner, repo, ref_name)?),
        QueryMsg::ListCollaborators {
            owner,
            repo,
            start_after,
            limit,
        } => to_json_binary(&query_list_collaborators(
            deps,
            owner,
            repo,
            start_after,
            limit,
        )?),
        QueryMsg::Config {} => {
            let cfg = CONFIG.load(deps.storage)?;
            to_json_binary(&ConfigResponse {
                admin: cfg.admin.to_string(),
                moderation_committee: cfg.moderation_committee.map(|a| a.to_string()),
                treasury: cfg.treasury.to_string(),
                platform_fee_bps: cfg.platform_fee_bps,
                username_deposit: cfg.username_deposit,
                username_fee: cfg.username_fee,
                reserved_usernames: cfg.reserved_usernames,
            })
        }
        QueryMsg::UpgradeSecurity {} => {
            let proposal = UPGRADE_PROPOSAL
                .may_load(deps.storage)?
                .map(|proposal| UpgradeProposalInfo {
                    wasm_sha256: proposal.wasm_sha256,
                    proposed_at: proposal.proposed_at,
                    execute_after: proposal.execute_after,
                });
            to_json_binary(&UpgradeSecurityResponse {
                proposal,
                timelock_seconds: UPGRADE_TIMELOCK_SECONDS,
            })
        }
        QueryMsg::ResolveUsername { name } => {
            let rec = USERNAMES.load(deps.storage, &name)?;
            to_json_binary(&UsernameResponse {
                name,
                owner: rec.owner.to_string(),
                registered_at: rec.registered_at,
            })
        }
        QueryMsg::AddressUsername { address } => {
            let addr = deps.api.addr_validate(&address)?;
            let name = ADDR_TO_NAME.may_load(deps.storage, &addr)?;
            to_json_binary(&AddressUsernameResponse { address, name })
        }
        QueryMsg::RevenueSplits { owner, repo } => {
            let owner_addr = deps.api.addr_validate(&owner)?;
            let splits = REVENUE_SPLITS
                .may_load(deps.storage, (&owner_addr, &repo))?
                .unwrap_or_default();
            to_json_binary(&RevenueSplitsResponse {
                owner,
                repo,
                splits,
            })
        }
        QueryMsg::SponsorTotals { owner, repo } => {
            let owner_addr = deps.api.addr_validate(&owner)?;
            let totals = SPONSOR_TOTALS
                .prefix((&owner_addr, &repo))
                .range(deps.storage, None, None, Order::Ascending)
                .map(|item| {
                    let (denom, amount) = item?;
                    Ok(SponsorTotal { denom, amount })
                })
                .collect::<StdResult<_>>()?;
            to_json_binary(&SponsorTotalsResponse {
                owner,
                repo,
                totals,
            })
        }
        QueryMsg::BadgesByRecipient {
            recipient,
            start_after,
            limit,
        } => {
            let recipient = deps.api.addr_validate(&recipient)?;
            let limit = limit.unwrap_or(DEFAULT_LIMIT).min(MAX_LIMIT) as usize;
            let start = start_after.map(Bound::exclusive);
            let badges = BADGES_BY_RECIPIENT
                .prefix(&recipient)
                .range(deps.storage, start, None, Order::Ascending)
                .take(limit)
                .map(|item| {
                    let (id, ()) = item?;
                    BADGES.load(deps.storage, id)
                })
                .collect::<StdResult<_>>()?;
            to_json_binary(&BadgesResponse { badges })
        }
        QueryMsg::BadgesByRepo {
            owner,
            repo,
            start_after,
            limit,
        } => {
            let owner = deps.api.addr_validate(&owner)?;
            let limit = limit.unwrap_or(DEFAULT_LIMIT).min(MAX_LIMIT) as usize;
            let start = start_after.map(Bound::exclusive);
            let badges = BADGES_BY_REPO
                .prefix((&owner, &repo))
                .range(deps.storage, start, None, Order::Ascending)
                .take(limit)
                .map(|item| {
                    let (id, ()) = item?;
                    BADGES.load(deps.storage, id)
                })
                .collect::<StdResult<_>>()?;
            to_json_binary(&BadgesResponse { badges })
        }
        QueryMsg::ReleaseArtifacts { version } => {
            let artifacts = RELEASE_ARTIFACTS
                .prefix(&version)
                .range(deps.storage, None, None, Order::Ascending)
                .map(|item| {
                    let (_platform, artifact) = item?;
                    Ok(ReleaseArtifactInfo {
                        version: artifact.version,
                        platform: artifact.platform,
                        sha256: artifact.sha256,
                        registered_by: artifact.registered_by.to_string(),
                        registered_at: artifact.registered_at,
                    })
                })
                .collect::<StdResult<_>>()?;
            to_json_binary(&ReleaseArtifactsResponse { version, artifacts })
        }
        QueryMsg::OwnershipSecurity { owner, repo } => {
            let owner_addr = deps.api.addr_validate(&owner)?;
            let transfer = OWNERSHIP_TRANSFERS
                .may_load(deps.storage, (&owner_addr, &repo))?
                .map(|pending| OwnershipTransferInfo {
                    new_owner: pending.new_owner.to_string(),
                    proposed_at: pending.proposed_at,
                    execute_after: pending.execute_after,
                });
            let recovery = RECOVERY_PROPOSALS
                .may_load(deps.storage, (&owner_addr, &repo))?
                .map(|proposal| RecoveryProposalInfo {
                    new_owner: proposal.new_owner.to_string(),
                    proposed_at: proposal.proposed_at,
                    execute_after: proposal.execute_after,
                    approvals: proposal.approvals.into_iter().map(|a| a.to_string()).collect(),
                });
            let guardians = GUARDIANS
                .prefix((&owner_addr, &repo))
                .range(deps.storage, None, None, Order::Ascending)
                .map(|item| item.map(|(addr, _)| addr.to_string()))
                .collect::<StdResult<_>>()?;
            let guardian_threshold = GUARDIAN_CONFIGS
                .may_load(deps.storage, (&owner_addr, &repo))?
                .map(|cfg| cfg.threshold)
                .unwrap_or(0);
            to_json_binary(&OwnershipSecurityResponse {
                transfer,
                recovery,
                guardians,
                guardian_threshold,
            })
        }
        QueryMsg::ModerationReport { report_id } => {
            let report = REPORTS
                .may_load(deps.storage, report_id)?
                .ok_or_else(|| cosmwasm_std::StdError::not_found("moderation report"))?;
            to_json_binary(&ModerationReportResponse {
                id: report.id,
                owner: report.owner.to_string(),
                repo: report.repo,
                reporter: report.reporter.to_string(),
                reason_hash: report.reason_hash,
                status: report.status,
                resolution: report.resolution,
                resolution_hash: report.resolution_hash,
                appeal_hash: report.appeal_hash,
                created_at: report.created_at,
                updated_at: report.updated_at,
            })
        }
    }
}

fn default_reserved_usernames() -> Vec<String> {
    ["admin", "api", "git", "help", "injective", "owner", "root", "support", "www"]
        .into_iter()
        .map(str::to_string)
        .collect()
}

fn validate_reserved_usernames(names: &[String]) -> Result<(), ContractError> {
    if names.len() > 128 {
        return Err(ContractError::InvalidUsernamePolicy {
            reason: "too many reserved names (max 128)".to_string(),
        });
    }
    let mut seen = std::collections::BTreeSet::new();
    for name in names {
        validate_username(name)?;
        if !seen.insert(name) {
            return Err(ContractError::InvalidUsernamePolicy {
                reason: format!("duplicate reserved name {name}"),
            });
        }
    }
    Ok(())
}

fn validate_username_fee(deposit: &Coin, fee: &Coin) -> Result<(), ContractError> {
    if deposit.denom.is_empty() || fee.denom != deposit.denom {
        return Err(ContractError::InvalidUsernamePolicy {
            reason: "username deposit and fee must use the same non-empty denom".to_string(),
        });
    }
    Ok(())
}

fn repo_to_response(repo: Repo) -> RepoInfoResponse {
    RepoInfoResponse {
        owner: repo.owner.to_string(),
        name: repo.name,
        description: repo.description,
        default_branch: repo.default_branch,
        created_at: repo.created_at,
        updated_at: repo.updated_at,
        moderation_status: repo.moderation_status,
        forked_from: repo.forked_from,
    }
}

fn query_repo_info(deps: Deps, owner: String, repo: String) -> StdResult<RepoInfoResponse> {
    let owner = deps.api.addr_validate(&owner)?;
    let repo = REPOS.load(deps.storage, (&owner, &repo))?;
    Ok(repo_to_response(repo))
}

fn query_list_refs(
    deps: Deps,
    owner: String,
    repo: String,
    start_after: Option<String>,
    limit: Option<u32>,
) -> StdResult<ListRefsResponse> {
    let owner = deps.api.addr_validate(&owner)?;
    let limit = limit.unwrap_or(DEFAULT_LIMIT).min(MAX_LIMIT) as usize;
    let start = start_after.as_deref().map(Bound::exclusive);
    let refs = REFS
        .prefix((&owner, &repo))
        .range(deps.storage, start, None, Order::Ascending)
        .take(limit)
        .map(|item| {
            let (ref_name, e) = item?;
            Ok(RefInfo {
                ref_name,
                commit_sha: e.commit_sha,
                pack_uris: e.pack_uris,
                updated_at: e.updated_at,
                updated_by: e.updated_by.to_string(),
            })
        })
        .collect::<StdResult<_>>()?;
    Ok(ListRefsResponse { refs })
}

fn query_list_repos(
    deps: Deps,
    owner: String,
    start_after: Option<String>,
    limit: Option<u32>,
) -> StdResult<ListReposResponse> {
    let owner = deps.api.addr_validate(&owner)?;
    let limit = limit.unwrap_or(DEFAULT_LIMIT).min(MAX_LIMIT) as usize;
    let start = start_after.as_deref().map(Bound::exclusive);
    let repos = REPOS
        .prefix(&owner)
        .range(deps.storage, start, None, Order::Ascending)
        .take(limit)
        .map(|item| {
            let (_, repo) = item?;
            Ok(repo_to_response(repo))
        })
        .collect::<StdResult<_>>()?;
    Ok(ListReposResponse { repos })
}

fn query_resolve_ref(
    deps: Deps,
    owner: String,
    repo: String,
    ref_name: String,
) -> StdResult<ResolveRefResponse> {
    let owner = deps.api.addr_validate(&owner)?;
    let entry = REFS.load(deps.storage, (&owner, &repo, &ref_name))?;
    Ok(ResolveRefResponse {
        ref_name,
        commit_sha: entry.commit_sha,
        pack_uris: entry.pack_uris,
    })
}

fn query_list_collaborators(
    deps: Deps,
    owner: String,
    repo: String,
    start_after: Option<String>,
    limit: Option<u32>,
) -> StdResult<ListCollaboratorsResponse> {
    let owner = deps.api.addr_validate(&owner)?;
    let limit = limit.unwrap_or(DEFAULT_LIMIT).min(MAX_LIMIT) as usize;
    let start_addr = start_after.map(Addr::unchecked);
    let start = start_addr.as_ref().map(Bound::exclusive);
    let collaborators = COLLABORATORS
        .prefix((&owner, &repo))
        .range(deps.storage, start, None, Order::Ascending)
        .take(limit)
        .map(|item| {
            let (addr, role) = item?;
            Ok(CollaboratorInfo {
                address: addr.to_string(),
                role,
            })
        })
        .collect::<StdResult<_>>()?;
    Ok(ListCollaboratorsResponse { collaborators })
}

#[cfg(test)]
mod tests {
    use super::*;
    use cosmwasm_std::testing::{message_info, mock_dependencies, mock_env};

    fn admin_info() -> MessageInfo {
        message_info(&Addr::unchecked("admin"), &[])
    }

    fn instantiate_for_test(deps: DepsMut) {
        instantiate(
            deps,
            mock_env(),
            admin_info(),
            InstantiateMsg {
                admin: None,
                moderation_committee: None,
                treasury: None,
                platform_fee_bps: None,
                username_deposit: None,
                username_fee: None,
                reserved_usernames: None,
            },
        )
        .unwrap();
    }

    #[test]
    fn migrate_requires_scheduled_hash_and_delay() {
        let mut deps = mock_dependencies();
        instantiate_for_test(deps.as_mut());
        let hash = "a".repeat(64);

        // Simulate a previous deployed version. A future migration cannot
        // bypass the proposal state just because the chain-level admin signed.
        cw2::set_contract_version(deps.as_mut().storage, CONTRACT_NAME, "0.4.0").unwrap();
        let env = mock_env();
        let err = migrate(
            deps.as_mut(),
            env.clone(),
            MigrateMsg {
                wasm_sha256: Some(hash.clone()),
            },
        )
        .unwrap_err();
        assert!(matches!(err, ContractError::NoUpgradeScheduled {}));

        execute(
            deps.as_mut(),
            env.clone(),
            admin_info(),
            ExecuteMsg::ScheduleUpgrade {
                wasm_sha256: hash.clone(),
            },
        )
        .unwrap();
        let err = migrate(
            deps.as_mut(),
            env.clone(),
            MigrateMsg {
                wasm_sha256: Some(hash.clone()),
            },
        )
        .unwrap_err();
        assert!(matches!(err, ContractError::UpgradeTooEarly { .. }));

        let mut late = env;
        late.block.time = late.block.time.plus_seconds(UPGRADE_TIMELOCK_SECONDS);
        let err = migrate(
            deps.as_mut(),
            late.clone(),
            MigrateMsg {
                wasm_sha256: Some("b".repeat(64)),
            },
        )
        .unwrap_err();
        assert!(matches!(err, ContractError::UpgradeHashMismatch {}));

        migrate(
            deps.as_mut(),
            late,
            MigrateMsg {
                wasm_sha256: Some(hash),
            },
        )
        .unwrap();
        assert!(UPGRADE_PROPOSAL
            .may_load(deps.as_ref().storage)
            .unwrap()
            .is_none());
    }

    #[test]
    fn upgrade_hash_validation_is_strict() {
        let mut deps = mock_dependencies();
        instantiate_for_test(deps.as_mut());
        let err = execute(
            deps.as_mut(),
            mock_env(),
            admin_info(),
            ExecuteMsg::ScheduleUpgrade {
                wasm_sha256: "not-a-hash".to_string(),
            },
        )
        .unwrap_err();
        assert!(matches!(err, ContractError::InvalidUpgradeHash {}));
    }
}
