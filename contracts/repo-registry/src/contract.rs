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
    AddressUsernameResponse, CollaboratorInfo, ConfigResponse, ExecuteMsg, InstantiateMsg,
    ListCollaboratorsResponse, ListRefsResponse, ListReposResponse, MigrateMsg, QueryMsg, RefInfo,
    RepoInfoResponse, ResolveRefResponse, RevenueSplitsResponse, SplitRecipient, SponsorTotal,
    SponsorTotalsResponse, UsernameResponse,
};
use crate::state::{
    Config, ModerationStatus, Repo, RefEntry, Role, SplitEntry, UsernameRecord, ADDR_TO_NAME,
    COLLABORATORS, CONFIG, DEFAULT_PLATFORM_FEE_BPS, MAX_PLATFORM_FEE_BPS, REFS, REPOS,
    REVENUE_SPLITS, SPONSOR_TOTALS, USERNAMES,
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
    CONFIG.save(
        deps.storage,
        &Config {
            admin: admin.clone(),
            moderation_committee,
            treasury,
            platform_fee_bps,
            username_deposit,
        },
    )?;
    Ok(Response::new()
        .add_attribute("action", "instantiate")
        .add_attribute("admin", admin))
}

#[cfg_attr(not(feature = "library"), entry_point)]
pub fn migrate(deps: DepsMut, _env: Env, _msg: MigrateMsg) -> Result<Response, ContractError> {
    // reject wasm blobs of a different contract; version-specific state
    // transforms hook in here as the schema evolves.
    let stored = cw2::get_contract_version(deps.storage)?;
    if stored.contract != CONTRACT_NAME {
        return Err(ContractError::Unauthorized {});
    }
    cw2::set_contract_version(deps.storage, CONTRACT_NAME, CONTRACT_VERSION)?;
    Ok(Response::new()
        .add_attribute("action", "migrate")
        .add_attribute("from_version", stored.version)
        .add_attribute("to_version", CONTRACT_VERSION))
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
            exec_transfer_ownership(deps, info, repo, new_owner)
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

fn exec_transfer_ownership(
    deps: DepsMut,
    info: MessageInfo,
    repo: String,
    new_owner: String,
) -> Result<Response, ContractError> {
    let new_owner = deps.api.addr_validate(&new_owner)?;
    let mut repo_meta = load_repo(deps.as_ref(), &info.sender, &repo)?;
    if REPOS.has(deps.storage, (&new_owner, &repo)) {
        return Err(ContractError::RepoExists { name: repo });
    }

    // move repo metadata
    repo_meta.owner = new_owner.clone();
    REPOS.remove(deps.storage, (&info.sender, &repo));
    REPOS.save(deps.storage, (&new_owner, &repo), &repo_meta)?;

    // move refs under the new owner key
    let refs: Vec<(String, RefEntry)> = REFS
        .prefix((&info.sender, &repo))
        .range(deps.storage, None, None, Order::Ascending)
        .collect::<StdResult<_>>()?;
    for (ref_name, entry) in refs {
        REFS.remove(deps.storage, (&info.sender, &repo, &ref_name));
        REFS.save(deps.storage, (&new_owner, &repo, &ref_name), &entry)?;
    }

    // move collaborators; drop new owner if they were a collaborator
    let collabs: Vec<(Addr, Role)> = COLLABORATORS
        .prefix((&info.sender, &repo))
        .range(deps.storage, None, None, Order::Ascending)
        .collect::<StdResult<_>>()?;
    for (addr, role) in collabs {
        COLLABORATORS.remove(deps.storage, (&info.sender, &repo, &addr));
        if addr != new_owner {
            COLLABORATORS.save(deps.storage, (&new_owner, &repo, &addr), &role)?;
        }
    }

    // revenue splits do NOT follow the repo — the new owner decides anew (§3)
    REVENUE_SPLITS.remove(deps.storage, (&info.sender, &repo));
    // sponsorship history follows the repo (sponsor wall / §14 metrics)
    let totals: Vec<(String, Uint128)> = SPONSOR_TOTALS
        .prefix((&info.sender, &repo))
        .range(deps.storage, None, None, Order::Ascending)
        .collect::<StdResult<_>>()?;
    for (denom, amount) in totals {
        SPONSOR_TOTALS.remove(deps.storage, (&info.sender, &repo, &denom));
        SPONSOR_TOTALS.save(deps.storage, (&new_owner, &repo, &denom), &amount)?;
    }

    Ok(Response::new()
        .add_attribute("action", "transfer_ownership")
        .add_attribute("repo", repo)
        .add_attribute("old_owner", info.sender)
        .add_attribute("new_owner", new_owner))
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
    if let Some(existing) = ADDR_TO_NAME.may_load(deps.storage, &info.sender)? {
        return Err(ContractError::AlreadyHasUsername { name: existing });
    }
    let cfg = CONFIG.load(deps.storage)?;
    let expected = &cfg.username_deposit;
    let paid_ok = info.funds.len() == 1
        && info.funds[0].denom == expected.denom
        && info.funds[0].amount == expected.amount;
    if !paid_ok {
        return Err(ContractError::DepositMismatch {
            expected: format!("{}{}", expected.amount, expected.denom),
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
    Ok(Response::new()
        .add_attribute("action", "register_username")
        .add_attribute("name", name)
        .add_attribute("owner", info.sender))
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
    }
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
