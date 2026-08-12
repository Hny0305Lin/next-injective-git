use cosmwasm_std::{coins, Addr};
use cw_multi_test::{App, ContractWrapper, Executor};

use repo_registry::msg::{
    BadgesResponse, ExecuteMsg, InstantiateMsg, ListCollaboratorsResponse, ListRefsResponse,
    ListReposResponse, MigrateMsg, ModerationReportResponse, OwnershipSecurityResponse, QueryMsg,
    ReleaseArtifactInput, ReleaseArtifactsResponse, RepoInfoResponse, ResolveRefResponse,
    RevenueSplitsResponse, SplitRecipient, SponsorTotalsResponse,
};
use repo_registry::state::{ModerationStatus, Role};
use repo_registry::ContractError;

const SHA_A: &str = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const SHA_B: &str = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
const SHA_C: &str = "cccccccccccccccccccccccccccccccccccccccc";

struct TestEnv {
    app: App,
    contract: Addr,
    alice: Addr,
    bob: Addr,
    carol: Addr,
}

fn setup() -> TestEnv {
    let mut app = App::default();
    let code = ContractWrapper::new(
        repo_registry::contract::execute,
        repo_registry::contract::instantiate,
        repo_registry::contract::query,
    )
    .with_migrate(repo_registry::contract::migrate);
    let code_id = app.store_code(Box::new(code));
    let alice = app.api().addr_make("alice");
    let bob = app.api().addr_make("bob");
    let carol = app.api().addr_make("carol");
    let contract = app
        .instantiate_contract(
            code_id,
            alice.clone(),
            &InstantiateMsg {
                admin: None,
                moderation_committee: None,
                treasury: None,
                platform_fee_bps: None,
                username_deposit: None,
                username_fee: None,
                reserved_usernames: None,
            },
            &[],
            "repo-registry",
            Some(alice.to_string()),
        )
        .unwrap();
    TestEnv {
        app,
        contract,
        alice,
        bob,
        carol,
    }
}

fn create_repo(env: &mut TestEnv, sender: &Addr, name: &str) {
    env.app
        .execute_contract(
            sender.clone(),
            env.contract.clone(),
            &ExecuteMsg::CreateRepo {
                name: name.to_string(),
                description: Some("test repo".to_string()),
                default_branch: None,
            },
            &[],
        )
        .unwrap();
}

fn update_ref_msg(
    owner: &Addr,
    repo: &str,
    ref_name: &str,
    sha: &str,
    cids: Vec<&str>,
    expected: Option<&str>,
    force: bool,
) -> ExecuteMsg {
    ExecuteMsg::UpdateRef {
        owner: owner.to_string(),
        repo: repo.to_string(),
        ref_name: ref_name.to_string(),
        commit_sha: sha.to_string(),
        // tests pass bare CIDs; wrap them in the canonical ipfs:// scheme
        pack_uris: cids.into_iter().map(|c| format!("ipfs://{c}")).collect(),
        expected_sha: expected.map(String::from),
        force,
    }
}

fn repo_info(env: &TestEnv, owner: &Addr, repo: &str) -> RepoInfoResponse {
    env.app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::RepoInfo {
                owner: owner.to_string(),
                repo: repo.to_string(),
            },
        )
        .unwrap()
}

#[test]
fn create_repo_and_query_info() {
    let mut env = setup();
    let alice = env.alice.clone();
    create_repo(&mut env, &alice, "hello");

    let info: RepoInfoResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::RepoInfo {
                owner: env.alice.to_string(),
                repo: "hello".to_string(),
            },
        )
        .unwrap();
    assert_eq!(info.name, "hello");
    assert_eq!(info.owner, env.alice.to_string());
    assert_eq!(info.default_branch, "main");

    // duplicate name rejected
    let err = env
        .app
        .execute_contract(
            env.alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::CreateRepo {
                name: "hello".to_string(),
                description: None,
                default_branch: None,
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RepoExists { .. }
    ));
}

#[test]
fn update_repo_info_patches_fields_clears_values_and_preserves_repo_state() {
    let mut env = setup();
    let (alice, bob) = (env.alice.clone(), env.bob.clone());
    create_repo(&mut env, &alice, "hello");
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(
                &alice,
                "hello",
                "refs/heads/main",
                SHA_A,
                vec!["cid1"],
                None,
                false,
            ),
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetCollaborator {
                repo: "hello".to_string(),
                collaborator: bob.to_string(),
                role: Some(Role::Maintainer),
            },
            &[],
        )
        .unwrap();

    let before = repo_info(&env, &alice, "hello");
    let refs_before: ListRefsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRefs {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    let collaborators_before: ListCollaboratorsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListCollaborators {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();

    env.app
        .update_block(|block| block.time = block.time.plus_seconds(10));
    let response = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::UpdateRepoInfo {
                repo: "hello".to_string(),
                description: Some("updated description".to_string()),
                default_branch: None,
            },
            &[],
        )
        .unwrap();

    let wasm_event = response
        .events
        .iter()
        .find(|event| event.ty == "wasm")
        .expect("metadata update must emit wasm attributes");
    for (key, value) in [
        ("action", "update_repo_info"),
        ("owner", alice.as_str()),
        ("repo", "hello"),
    ] {
        assert!(
            wasm_event
                .attributes
                .iter()
                .any(|attribute| attribute.key == key && attribute.value == value),
            "missing {key}={value} in {wasm_event:?}"
        );
    }

    let after_description = repo_info(&env, &alice, "hello");
    assert_eq!(after_description.description, "updated description");
    assert_eq!(after_description.default_branch, before.default_branch);
    assert_eq!(after_description.owner, before.owner);
    assert_eq!(after_description.name, before.name);
    assert_eq!(after_description.created_at, before.created_at);
    assert_eq!(
        after_description.moderation_status,
        before.moderation_status
    );
    assert_eq!(after_description.forked_from, before.forked_from);

    env.app
        .update_block(|block| block.time = block.time.plus_seconds(10));
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::UpdateRepoInfo {
                repo: "hello".to_string(),
                description: None,
                default_branch: Some("develop".to_string()),
            },
            &[],
        )
        .unwrap();
    let after_branch = repo_info(&env, &alice, "hello");
    assert_eq!(after_branch.description, "updated description");
    assert_eq!(after_branch.default_branch, "develop");

    env.app
        .update_block(|block| block.time = block.time.plus_seconds(10));
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::UpdateRepoInfo {
                repo: "hello".to_string(),
                description: Some(String::new()),
                default_branch: Some(String::new()),
            },
            &[],
        )
        .unwrap();
    let after_clear = repo_info(&env, &alice, "hello");
    assert_eq!(after_clear.description, "");
    assert_eq!(after_clear.default_branch, "");

    let refs_after: ListRefsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRefs {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    let collaborators_after: ListCollaboratorsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListCollaborators {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(refs_after, refs_before);
    assert_eq!(collaborators_after, collaborators_before);
}

#[test]
fn update_repo_info_none_fields_still_advances_updated_at() {
    let mut env = setup();
    let alice = env.alice.clone();
    create_repo(&mut env, &alice, "hello");
    let before = repo_info(&env, &alice, "hello");

    env.app
        .update_block(|block| block.time = block.time.plus_seconds(17));
    let expected_updated_at = env.app.block_info().time.seconds();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::UpdateRepoInfo {
                repo: "hello".to_string(),
                description: None,
                default_branch: None,
            },
            &[],
        )
        .unwrap();

    let mut expected = before.clone();
    expected.updated_at = expected_updated_at;
    assert_eq!(repo_info(&env, &alice, "hello"), expected);
    assert!(expected_updated_at > before.updated_at);
}

#[test]
fn update_repo_info_is_owner_only() {
    let mut env = setup();
    let (alice, bob, carol) = (env.alice.clone(), env.bob.clone(), env.carol.clone());
    let stranger = env.app.api().addr_make("stranger");
    create_repo(&mut env, &alice, "hello");
    for (collaborator, role) in [(&bob, Role::Maintainer), (&carol, Role::Reader)] {
        env.app
            .execute_contract(
                alice.clone(),
                env.contract.clone(),
                &ExecuteMsg::SetCollaborator {
                    repo: "hello".to_string(),
                    collaborator: collaborator.to_string(),
                    role: Some(role),
                },
                &[],
            )
            .unwrap();
    }

    for sender in [&bob, &carol, &stranger] {
        let err = env
            .app
            .execute_contract(
                sender.clone(),
                env.contract.clone(),
                &ExecuteMsg::UpdateRepoInfo {
                    repo: "hello".to_string(),
                    description: Some("tampered".to_string()),
                    default_branch: None,
                },
                &[],
            )
            .unwrap_err();
        match err.downcast::<ContractError>().unwrap() {
            ContractError::RepoNotFound { owner, name } => {
                assert_eq!(owner, sender.to_string());
                assert_eq!(name, "hello");
            }
            other => panic!("unexpected metadata authorization error: {other:?}"),
        }
    }
    assert_eq!(repo_info(&env, &alice, "hello").description, "test repo");

    // UpdateRepoInfo always targets the sender's namespace. A maintainer with
    // an independently owned same-name repo updates that repo, not the target.
    create_repo(&mut env, &bob, "hello");
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::UpdateRepoInfo {
                repo: "hello".to_string(),
                description: Some("bob's repo".to_string()),
                default_branch: None,
            },
            &[],
        )
        .unwrap();
    assert_eq!(repo_info(&env, &alice, "hello").description, "test repo");
    assert_eq!(repo_info(&env, &bob, "hello").description, "bob's repo");
}

#[test]
fn update_repo_info_is_allowed_while_delisted_or_frozen() {
    let mut env = setup();
    let (moderator, owner) = (env.alice.clone(), env.bob.clone());
    create_repo(&mut env, &owner, "moderated");

    env.app
        .execute_contract(
            moderator.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: owner.to_string(),
                repo: "moderated".to_string(),
                status: ModerationStatus::Delisted,
                reason_hash: None,
            },
            &[],
        )
        .unwrap();
    env.app
        .update_block(|block| block.time = block.time.plus_seconds(5));
    env.app
        .execute_contract(
            owner.clone(),
            env.contract.clone(),
            &ExecuteMsg::UpdateRepoInfo {
                repo: "moderated".to_string(),
                description: Some("delisted metadata".to_string()),
                default_branch: None,
            },
            &[],
        )
        .unwrap();
    let delisted = repo_info(&env, &owner, "moderated");
    assert_eq!(delisted.description, "delisted metadata");
    assert_eq!(delisted.moderation_status, ModerationStatus::Delisted);

    env.app
        .execute_contract(
            moderator,
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: owner.to_string(),
                repo: "moderated".to_string(),
                status: ModerationStatus::Frozen,
                reason_hash: None,
            },
            &[],
        )
        .unwrap();
    env.app
        .update_block(|block| block.time = block.time.plus_seconds(5));
    env.app
        .execute_contract(
            owner.clone(),
            env.contract.clone(),
            &ExecuteMsg::UpdateRepoInfo {
                repo: "moderated".to_string(),
                description: None,
                default_branch: Some("frozen-branch".to_string()),
            },
            &[],
        )
        .unwrap();
    let frozen = repo_info(&env, &owner, "moderated");
    assert_eq!(frozen.description, "delisted metadata");
    assert_eq!(frozen.default_branch, "frozen-branch");
    assert_eq!(frozen.moderation_status, ModerationStatus::Frozen);
}

#[test]
fn invalid_repo_name_rejected() {
    let mut env = setup();
    let err = env
        .app
        .execute_contract(
            env.alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::CreateRepo {
                name: "bad/name".to_string(),
                description: None,
                default_branch: None,
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::InvalidRepoName { .. }
    ));
}

#[test]
fn push_and_resolve_ref() {
    let mut env = setup();
    let alice = env.alice.clone();
    create_repo(&mut env, &alice, "hello");

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(&alice, "hello", "refs/heads/main", SHA_A, vec!["cid1"], None, false),
            &[],
        )
        .unwrap();

    let resolved: ResolveRefResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ResolveRef {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                ref_name: "refs/heads/main".to_string(),
            },
        )
        .unwrap();
    assert_eq!(resolved.commit_sha, SHA_A);
    assert_eq!(resolved.pack_uris, vec!["ipfs://cid1"]);

    // incremental push appends CID
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(
                &alice,
                "hello",
                "refs/heads/main",
                SHA_B,
                vec!["cid2"],
                Some(SHA_A),
                false,
            ),
            &[],
        )
        .unwrap();
    let resolved: ResolveRefResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ResolveRef {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                ref_name: "refs/heads/main".to_string(),
            },
        )
        .unwrap();
    assert_eq!(resolved.commit_sha, SHA_B);
    assert_eq!(resolved.pack_uris, vec!["ipfs://cid1", "ipfs://cid2"]);
}

#[test]
fn stale_push_rejected_force_push_allowed() {
    let mut env = setup();
    let alice = env.alice.clone();
    create_repo(&mut env, &alice, "hello");

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(&alice, "hello", "refs/heads/main", SHA_A, vec!["cid1"], None, false),
            &[],
        )
        .unwrap();

    // stale expected_sha (client hasn't fetched SHA_A yet) -> conflict
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(
                &alice,
                "hello",
                "refs/heads/main",
                SHA_B,
                vec!["cid2"],
                Some(SHA_C),
                false,
            ),
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RefConflict { .. }
    ));

    // force push replaces CID list entirely
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(&alice, "hello", "refs/heads/main", SHA_C, vec!["cid9"], None, true),
            &[],
        )
        .unwrap();
    let resolved: ResolveRefResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ResolveRef {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                ref_name: "refs/heads/main".to_string(),
            },
        )
        .unwrap();
    assert_eq!(resolved.commit_sha, SHA_C);
    assert_eq!(resolved.pack_uris, vec!["ipfs://cid9"]);
}

#[test]
fn collaborator_permissions() {
    let mut env = setup();
    let (alice, bob, carol) = (env.alice.clone(), env.bob.clone(), env.carol.clone());
    create_repo(&mut env, &alice, "hello");

    // bob can't push before being added
    let err = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &update_ref_msg(&alice, "hello", "refs/heads/main", SHA_A, vec!["cid1"], None, false),
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::Unauthorized {}
    ));

    // owner adds bob as maintainer, carol as reader
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetCollaborator {
                repo: "hello".to_string(),
                collaborator: bob.to_string(),
                role: Some(Role::Maintainer),
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetCollaborator {
                repo: "hello".to_string(),
                collaborator: carol.to_string(),
                role: Some(Role::Reader),
            },
            &[],
        )
        .unwrap();

    // maintainer can push
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &update_ref_msg(&alice, "hello", "refs/heads/main", SHA_A, vec!["cid1"], None, false),
            &[],
        )
        .unwrap();

    // reader cannot push
    let err = env
        .app
        .execute_contract(
            carol.clone(),
            env.contract.clone(),
            &update_ref_msg(
                &alice,
                "hello",
                "refs/heads/main",
                SHA_B,
                vec!["cid2"],
                Some(SHA_A),
                false,
            ),
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::Unauthorized {}
    ));

    // remove bob -> can no longer push
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetCollaborator {
                repo: "hello".to_string(),
                collaborator: bob.to_string(),
                role: None,
            },
            &[],
        )
        .unwrap();
    let err = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &update_ref_msg(
                &alice,
                "hello",
                "refs/heads/main",
                SHA_B,
                vec!["cid2"],
                Some(SHA_A),
                false,
            ),
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::Unauthorized {}
    ));
}

#[test]
fn delete_ref_and_list() {
    let mut env = setup();
    let alice = env.alice.clone();
    create_repo(&mut env, &alice, "hello");

    for (ref_name, sha) in [
        ("refs/heads/main", SHA_A),
        ("refs/heads/dev", SHA_B),
        ("refs/tags/v1.0", SHA_C),
    ] {
        env.app
            .execute_contract(
                alice.clone(),
                env.contract.clone(),
                &update_ref_msg(&alice, "hello", ref_name, sha, vec!["cid1"], None, false),
                &[],
            )
            .unwrap();
    }

    let listed: ListRefsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRefs {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(listed.refs.len(), 3);

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::DeleteRef {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                ref_name: "refs/heads/dev".to_string(),
            },
            &[],
        )
        .unwrap();

    let listed: ListRefsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRefs {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(listed.refs.len(), 2);

    // deleting a missing ref fails
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::DeleteRef {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                ref_name: "refs/heads/dev".to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RefNotFound { .. }
    ));
}

#[test]
fn transfer_ownership_moves_refs() {
    let mut env = setup();
    let (alice, bob) = (env.alice.clone(), env.bob.clone());
    create_repo(&mut env, &alice, "hello");
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(&alice, "hello", "refs/heads/main", SHA_A, vec!["cid1"], None, false),
            &[],
        )
        .unwrap();

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::TransferOwnership {
                repo: "hello".to_string(),
                new_owner: bob.to_string(),
            },
            &[],
        )
        .unwrap();

    // transfer is timelocked; the repo remains under alice until bob accepts
    let repos: ListReposResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRepos {
                owner: bob.to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(repos.repos.len(), 0);

    let early = env.app.execute_contract(
        bob.clone(),
        env.contract.clone(),
        &ExecuteMsg::AcceptOwnership {
            owner: alice.to_string(),
            repo: "hello".to_string(),
        },
        &[],
    );
    assert!(matches!(
        early.unwrap_err().downcast::<ContractError>().unwrap(),
        ContractError::OwnershipTransferTooEarly { .. }
    ));

    env.app.update_block(|block| {
        block.time = block.time.plus_seconds(7 * 24 * 60 * 60);
    });
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::AcceptOwnership {
                owner: alice.to_string(),
                repo: "hello".to_string(),
            },
            &[],
        )
        .unwrap();

    // repo now listed under bob, refs preserved
    let repos: ListReposResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRepos {
                owner: bob.to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(repos.repos.len(), 1);
    assert_eq!(repos.repos[0].owner, bob.to_string());

    let resolved: ResolveRefResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ResolveRef {
                owner: bob.to_string(),
                repo: "hello".to_string(),
                ref_name: "refs/heads/main".to_string(),
            },
        )
        .unwrap();
    assert_eq!(resolved.commit_sha, SHA_A);

    // alice no longer owns it: her push must fail
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(
                &bob,
                "hello",
                "refs/heads/main",
                SHA_B,
                vec!["cid2"],
                Some(SHA_A),
                false,
            ),
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::Unauthorized {}
    ));
}

#[test]
fn transfer_ownership_cancel_collisions_and_recovery_are_mutually_exclusive() {
    let mut env = setup();
    let (alice, bob, carol) = (env.alice.clone(), env.bob.clone(), env.carol.clone());
    create_repo(&mut env, &alice, "hello");

    let same_owner = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::TransferOwnership {
                repo: "hello".to_string(),
                new_owner: alice.to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        same_owner.downcast::<ContractError>().unwrap(),
        ContractError::InvalidGuardians { .. }
    ));

    create_repo(&mut env, &bob, "hello");
    let collision = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::TransferOwnership {
                repo: "hello".to_string(),
                new_owner: bob.to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        collision.downcast::<ContractError>().unwrap(),
        ContractError::RepoExists { .. }
    ));

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetGuardians {
                repo: "hello".to_string(),
                guardians: vec![bob.to_string()],
                threshold: 1,
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::TransferOwnership {
                repo: "hello".to_string(),
                new_owner: carol.to_string(),
            },
            &[],
        )
        .unwrap();

    let security: OwnershipSecurityResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::OwnershipSecurity {
                owner: alice.to_string(),
                repo: "hello".to_string(),
            },
        )
        .unwrap();
    let pending = security.transfer.expect("pending transfer");
    assert_eq!(pending.new_owner, carol.to_string());
    assert_eq!(
        pending.execute_after - pending.proposed_at,
        7 * 24 * 60 * 60
    );

    let duplicate = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::TransferOwnership {
                repo: "hello".to_string(),
                new_owner: carol.to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        duplicate.downcast::<ContractError>().unwrap(),
        ContractError::OwnershipTransferPending {}
    ));

    let unauthorized_accept = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::AcceptOwnership {
                owner: alice.to_string(),
                repo: "hello".to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        unauthorized_accept.downcast::<ContractError>().unwrap(),
        ContractError::Unauthorized {}
    ));

    let recovery_during_transfer = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::ProposeRecovery {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                new_owner: carol.to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        recovery_during_transfer
            .downcast::<ContractError>()
            .unwrap(),
        ContractError::OwnershipTransferPending {}
    ));

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::CancelOwnershipTransfer {
                repo: "hello".to_string(),
            },
            &[],
        )
        .unwrap();
    let after_cancel: OwnershipSecurityResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::OwnershipSecurity {
                owner: alice.to_string(),
                repo: "hello".to_string(),
            },
        )
        .unwrap();
    assert!(after_cancel.transfer.is_none());

    let accept_cancelled = env
        .app
        .execute_contract(
            carol.clone(),
            env.contract.clone(),
            &ExecuteMsg::AcceptOwnership {
                owner: alice.to_string(),
                repo: "hello".to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        accept_cancelled.downcast::<ContractError>().unwrap(),
        ContractError::NoOwnershipTransfer {}
    ));

    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::ProposeRecovery {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                new_owner: carol.to_string(),
            },
            &[],
        )
        .unwrap();
    let transfer_during_recovery = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::TransferOwnership {
                repo: "hello".to_string(),
                new_owner: carol.to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        transfer_during_recovery
            .downcast::<ContractError>()
            .unwrap(),
        ContractError::RecoveryPending {}
    ));
}

#[test]
fn transfer_ownership_preserves_and_resets_v1_extension_state() {
    let (mut env, _treasury) = setup_funded();
    let (alice, bob, carol) = (env.alice.clone(), env.bob.clone(), env.carol.clone());
    create_repo(&mut env, &alice, "stateful");
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::UpdateRepoInfo {
                repo: "stateful".to_string(),
                description: Some("state survives transfer".to_string()),
                default_branch: Some("release".to_string()),
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(
                &alice,
                "stateful",
                "refs/heads/release",
                SHA_A,
                vec!["cid1", "cid2"],
                None,
                false,
            ),
            &[],
        )
        .unwrap();
    for (collaborator, role) in [(&carol, Role::Maintainer), (&bob, Role::Reader)] {
        env.app
            .execute_contract(
                alice.clone(),
                env.contract.clone(),
                &ExecuteMsg::SetCollaborator {
                    repo: "stateful".to_string(),
                    collaborator: collaborator.to_string(),
                    role: Some(role),
                },
                &[],
            )
            .unwrap();
    }
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetRevenueSplits {
                repo: "stateful".to_string(),
                splits: vec![SplitRecipient {
                    address: carol.to_string(),
                    bps: 1000,
                }],
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::Sponsor {
                owner: alice.to_string(),
                repo: "stateful".to_string(),
                message: Some("before transfer".to_string()),
            },
            &coins(INJ, "inj"),
        )
        .unwrap();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetGuardians {
                repo: "stateful".to_string(),
                guardians: vec![carol.to_string()],
                threshold: 1,
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::AwardBadge {
                repo: "stateful".to_string(),
                recipient: carol.to_string(),
                reason: "before transfer".to_string(),
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            carol.clone(),
            env.contract.clone(),
            &ExecuteMsg::SubmitModerationReport {
                owner: alice.to_string(),
                repo: "stateful".to_string(),
                reason_hash: "report-before-transfer".to_string(),
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: alice.to_string(),
                repo: "stateful".to_string(),
                status: ModerationStatus::Delisted,
                reason_hash: Some("delisted-before-transfer".to_string()),
            },
            &[],
        )
        .unwrap();

    let before_repo = repo_info(&env, &alice, "stateful");
    let before_refs: ListRefsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRefs {
                owner: alice.to_string(),
                repo: "stateful".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::TransferOwnership {
                repo: "stateful".to_string(),
                new_owner: bob.to_string(),
            },
            &[],
        )
        .unwrap();
    env.app
        .update_block(|block| block.time = block.time.plus_seconds(7 * 24 * 60 * 60));
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::AcceptOwnership {
                owner: alice.to_string(),
                repo: "stateful".to_string(),
            },
            &[],
        )
        .unwrap();

    let moved = repo_info(&env, &bob, "stateful");
    assert_eq!(moved.owner, bob.to_string());
    assert_eq!(moved.description, before_repo.description);
    assert_eq!(moved.default_branch, before_repo.default_branch);
    assert_eq!(moved.created_at, before_repo.created_at);
    assert_eq!(moved.updated_at, before_repo.updated_at);
    assert_eq!(moved.moderation_status, ModerationStatus::Delisted);
    assert_eq!(moved.forked_from, before_repo.forked_from);
    assert!(env
        .app
        .wrap()
        .query_wasm_smart::<RepoInfoResponse>(
            &env.contract,
            &QueryMsg::RepoInfo {
                owner: alice.to_string(),
                repo: "stateful".to_string(),
            },
        )
        .is_err());

    let moved_refs: ListRefsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRefs {
                owner: bob.to_string(),
                repo: "stateful".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(moved_refs.refs, before_refs.refs);

    let collaborators: ListCollaboratorsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListCollaborators {
                owner: bob.to_string(),
                repo: "stateful".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(collaborators.collaborators.len(), 1);
    assert_eq!(collaborators.collaborators[0].address, carol.to_string());
    assert_eq!(collaborators.collaborators[0].role, Role::Maintainer);

    let splits: RevenueSplitsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::RevenueSplits {
                owner: bob.to_string(),
                repo: "stateful".to_string(),
            },
        )
        .unwrap();
    assert!(splits.splits.is_empty());

    let totals: SponsorTotalsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::SponsorTotals {
                owner: bob.to_string(),
                repo: "stateful".to_string(),
            },
        )
        .unwrap();
    assert_eq!(totals.totals.len(), 1);
    assert_eq!(totals.totals[0].amount.u128(), INJ);

    let security: OwnershipSecurityResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::OwnershipSecurity {
                owner: bob.to_string(),
                repo: "stateful".to_string(),
            },
        )
        .unwrap();
    assert!(security.guardians.is_empty());
    assert_eq!(security.guardian_threshold, 0);
    assert!(security.transfer.is_none());
    assert!(security.recovery.is_none());

    let old_badges: BadgesResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::BadgesByRepo {
                owner: alice.to_string(),
                repo: "stateful".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    let moved_badges: BadgesResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::BadgesByRepo {
                owner: bob.to_string(),
                repo: "stateful".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(old_badges.badges.len(), 1);
    assert_eq!(old_badges.badges[0].repo_owner, alice);
    assert!(moved_badges.badges.is_empty());

    let report: ModerationReportResponse = env
        .app
        .wrap()
        .query_wasm_smart(&env.contract, &QueryMsg::ModerationReport { report_id: 1 })
        .unwrap();
    assert_eq!(report.owner, alice.to_string());
    assert_eq!(report.repo, "stateful");
}

#[test]
fn transfer_ownership_is_allowed_while_frozen_or_delisted() {
    let mut env = setup();
    let (alice, bob) = (env.alice.clone(), env.bob.clone());
    for (repo, status) in [
        ("frozen-transfer", ModerationStatus::Frozen),
        ("delisted-transfer", ModerationStatus::Delisted),
    ] {
        create_repo(&mut env, &alice, repo);
        env.app
            .execute_contract(
                alice.clone(),
                env.contract.clone(),
                &ExecuteMsg::SetModerationStatus {
                    owner: alice.to_string(),
                    repo: repo.to_string(),
                    status,
                    reason_hash: None,
                },
                &[],
            )
            .unwrap();
        env.app
            .execute_contract(
                alice.clone(),
                env.contract.clone(),
                &ExecuteMsg::TransferOwnership {
                    repo: repo.to_string(),
                    new_owner: bob.to_string(),
                },
                &[],
            )
            .unwrap();
    }
    env.app
        .update_block(|block| block.time = block.time.plus_seconds(7 * 24 * 60 * 60));
    for (repo, status) in [
        ("frozen-transfer", ModerationStatus::Frozen),
        ("delisted-transfer", ModerationStatus::Delisted),
    ] {
        env.app
            .execute_contract(
                bob.clone(),
                env.contract.clone(),
                &ExecuteMsg::AcceptOwnership {
                    owner: alice.to_string(),
                    repo: repo.to_string(),
                },
                &[],
            )
            .unwrap();
        assert_eq!(repo_info(&env, &bob, repo).moderation_status, status);
    }
}

#[test]
fn guardian_recovery_requires_threshold_and_timelock() {
    let mut env = setup();
    let (alice, bob, carol) = (env.alice.clone(), env.bob.clone(), env.carol.clone());
    let dave = env.app.api().addr_make("dave");
    create_repo(&mut env, &alice, "recover");

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetGuardians {
                repo: "recover".to_string(),
                guardians: vec![bob.to_string(), carol.to_string()],
                threshold: 2,
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::ProposeRecovery {
                owner: alice.to_string(),
                repo: "recover".to_string(),
                new_owner: dave.to_string(),
            },
            &[],
        )
        .unwrap();

    let insufficient = env.app.execute_contract(
        dave.clone(),
        env.contract.clone(),
        &ExecuteMsg::AcceptRecovery {
            owner: alice.to_string(),
            repo: "recover".to_string(),
        },
        &[],
    );
    assert!(matches!(
        insufficient.unwrap_err().downcast::<ContractError>().unwrap(),
        ContractError::OwnershipTransferTooEarly { .. }
    ));

    env.app
        .execute_contract(
            carol.clone(),
            env.contract.clone(),
            &ExecuteMsg::ApproveRecovery {
                owner: alice.to_string(),
                repo: "recover".to_string(),
            },
            &[],
        )
        .unwrap();
    env.app.update_block(|block| {
        block.time = block.time.plus_seconds(7 * 24 * 60 * 60);
    });
    env.app
        .execute_contract(
            dave.clone(),
            env.contract.clone(),
            &ExecuteMsg::AcceptRecovery {
                owner: alice.to_string(),
                repo: "recover".to_string(),
            },
            &[],
        )
        .unwrap();

    let repos: ListReposResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRepos {
                owner: dave.to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(repos.repos.len(), 1);
}

#[test]
fn invalid_inputs_rejected() {
    let mut env = setup();
    let alice = env.alice.clone();
    create_repo(&mut env, &alice, "hello");

    // bad ref name
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(&alice, "hello", "main", SHA_A, vec!["cid1"], None, false),
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::InvalidRefName { .. }
    ));

    // bad sha
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(&alice, "hello", "refs/heads/main", "xyz", vec!["cid1"], None, false),
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::InvalidCommitSha { .. }
    ));

    // empty cids
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(&alice, "hello", "refs/heads/main", SHA_A, vec![], None, false),
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::EmptyPackUris {}
    ));

    // bare CID without scheme rejected
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::UpdateRef {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                ref_name: "refs/heads/main".to_string(),
                commit_sha: SHA_A.to_string(),
                pack_uris: vec!["bafyrawcid".to_string()],
                expected_sha: None,
                force: false,
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::InvalidPackUri { .. }
    ));

    // push to nonexistent repo
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &update_ref_msg(&alice, "nope", "refs/heads/main", SHA_A, vec!["cid1"], None, false),
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RepoNotFound { .. }
    ));
}

#[test]
fn moderation_freeze_blocks_push() {
    let mut env = setup();
    // alice instantiated with admin: None -> admin (and fallback moderator) is alice
    let (alice, bob) = (env.alice.clone(), env.bob.clone());
    create_repo(&mut env, &bob, "bobrepo");
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &update_ref_msg(&bob, "bobrepo", "refs/heads/main", SHA_A, vec!["cid1"], None, false),
            &[],
        )
        .unwrap();

    // non-moderator cannot change moderation status
    let err = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: bob.to_string(),
                repo: "bobrepo".to_string(),
                status: ModerationStatus::Frozen,
                reason_hash: None,
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::Unauthorized {}
    ));

    // admin freezes the repo
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: bob.to_string(),
                repo: "bobrepo".to_string(),
                status: ModerationStatus::Frozen,
                reason_hash: Some("deadbeef".to_string()),
            },
            &[],
        )
        .unwrap();

    // even the owner cannot push or delete refs while frozen
    let err = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &update_ref_msg(
                &bob,
                "bobrepo",
                "refs/heads/main",
                SHA_B,
                vec!["cid2"],
                Some(SHA_A),
                false,
            ),
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RepoFrozen { .. }
    ));

    // status is visible in repo info
    let info: RepoInfoResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::RepoInfo {
                owner: bob.to_string(),
                repo: "bobrepo".to_string(),
            },
        )
        .unwrap();
    assert_eq!(info.moderation_status, ModerationStatus::Frozen);

    // unfreeze -> push works again
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: bob.to_string(),
                repo: "bobrepo".to_string(),
                status: ModerationStatus::Active,
                reason_hash: None,
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &update_ref_msg(
                &bob,
                "bobrepo",
                "refs/heads/main",
                SHA_B,
                vec!["cid2"],
                Some(SHA_A),
                false,
            ),
            &[],
        )
        .unwrap();
}

#[test]
fn moderation_report_and_appeal_are_auditable() {
    let mut env = setup();
    let (alice, bob, carol) = (env.alice.clone(), env.bob.clone(), env.carol.clone());
    create_repo(&mut env, &bob, "reported");

    env.app
        .execute_contract(
            carol.clone(),
            env.contract.clone(),
            &ExecuteMsg::SubmitModerationReport {
                owner: bob.to_string(),
                repo: "reported".to_string(),
                reason_hash: "report-hash".to_string(),
            },
            &[],
        )
        .unwrap();
    let open: ModerationReportResponse = env
        .app
        .wrap()
        .query_wasm_smart(&env.contract, &QueryMsg::ModerationReport { report_id: 1 })
        .unwrap();
    assert_eq!(open.reporter, carol.to_string());

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::ResolveModerationReport {
                report_id: 1,
                status: ModerationStatus::Frozen,
                reason_hash: "decision-hash".to_string(),
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::AppealModerationReport {
                report_id: 1,
                reason_hash: "appeal-hash".to_string(),
            },
            &[],
        )
        .unwrap();
    let unauthorized = env.app.execute_contract(
        carol,
        env.contract.clone(),
        &ExecuteMsg::ResolveModerationAppeal {
            report_id: 1,
            status: ModerationStatus::Active,
            reason_hash: "bad-resolution".to_string(),
        },
        &[],
    );
    assert!(matches!(
        unauthorized.unwrap_err().downcast::<ContractError>().unwrap(),
        ContractError::Unauthorized {}
    ));
    env.app
        .execute_contract(
            alice,
            env.contract.clone(),
            &ExecuteMsg::ResolveModerationAppeal {
                report_id: 1,
                status: ModerationStatus::Active,
                reason_hash: "appeal-decision".to_string(),
            },
            &[],
        )
        .unwrap();
    let resolved: ModerationReportResponse = env
        .app
        .wrap()
        .query_wasm_smart(&env.contract, &QueryMsg::ModerationReport { report_id: 1 })
        .unwrap();
    assert!(matches!(resolved.status, repo_registry::state::ReportStatus::AppealResolved));
}

#[test]
fn migrate_same_contract_ok() {
    let mut env = setup();
    let alice = env.alice.clone();
    // re-store the same code and migrate the instance to it (alice is the
    // wasm-level admin set in setup)
    let code = ContractWrapper::new(
        repo_registry::contract::execute,
        repo_registry::contract::instantiate,
        repo_registry::contract::query,
    )
    .with_migrate(repo_registry::contract::migrate);
    let new_code_id = env.app.store_code(Box::new(code));
    let hash = "a".repeat(64);
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::ScheduleUpgrade {
                wasm_sha256: hash.clone(),
            },
            &[],
        )
        .unwrap();
    env.app.update_block(|block| {
        block.time = block.time.plus_seconds(14 * 24 * 60 * 60);
    });
    env.app
        .migrate_contract(
            alice,
            env.contract.clone(),
            &MigrateMsg {
                wasm_sha256: Some(hash),
            },
            new_code_id,
        )
        .unwrap();
}

#[test]
fn upgrade_schedule_is_admin_only_and_timelocked() {
    let mut env = setup();
    let hash = "a".repeat(64);

    let unauthorized = env.app.execute_contract(
        env.bob.clone(),
        env.contract.clone(),
        &ExecuteMsg::ScheduleUpgrade {
            wasm_sha256: hash.clone(),
        },
        &[],
    );
    assert!(matches!(
        unauthorized.unwrap_err().downcast::<ContractError>().unwrap(),
        ContractError::Unauthorized {}
    ));

    env.app
        .execute_contract(
            env.alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::ScheduleUpgrade {
                wasm_sha256: hash.clone(),
            },
            &[],
        )
        .unwrap();

    let security: repo_registry::msg::UpgradeSecurityResponse = env
        .app
        .wrap()
        .query_wasm_smart(&env.contract, &QueryMsg::UpgradeSecurity {})
        .unwrap();
    let proposal = security.proposal.expect("upgrade proposal");
    assert_eq!(proposal.wasm_sha256, hash);
    assert_eq!(
        proposal.execute_after - proposal.proposed_at,
        14 * 24 * 60 * 60
    );

    let duplicate = env.app.execute_contract(
        env.alice.clone(),
        env.contract.clone(),
        &ExecuteMsg::ScheduleUpgrade {
            wasm_sha256: "b".repeat(64),
        },
        &[],
    );
    assert!(matches!(
        duplicate.unwrap_err().downcast::<ContractError>().unwrap(),
        ContractError::UpgradeAlreadyScheduled {}
    ));

    env.app
        .execute_contract(
            env.alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::CancelUpgrade {},
            &[],
        )
        .unwrap();
    let cleared: repo_registry::msg::UpgradeSecurityResponse = env
        .app
        .wrap()
        .query_wasm_smart(&env.contract, &QueryMsg::UpgradeSecurity {})
        .unwrap();
    assert!(cleared.proposal.is_none());
}

#[test]
fn fork_copies_refs_and_records_source() {
    let mut env = setup();
    let (alice, bob) = (env.alice.clone(), env.bob.clone());
    create_repo(&mut env, &alice, "hello");
    for (r, sha) in [("refs/heads/main", SHA_A), ("refs/tags/v1", SHA_B)] {
        env.app
            .execute_contract(
                alice.clone(),
                env.contract.clone(),
                &update_ref_msg(&alice, "hello", r, sha, vec!["cid1"], None, false),
                &[],
            )
            .unwrap();
    }

    // bob forks alice/hello
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::ForkRepo {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                name: None,
            },
            &[],
        )
        .unwrap();

    // fork metadata records the source
    let info: RepoInfoResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::RepoInfo {
                owner: bob.to_string(),
                repo: "hello".to_string(),
            },
        )
        .unwrap();
    assert_eq!(info.forked_from, Some(format!("{alice}/hello")));

    // refs (incl. pack uris) are copied
    let resolved: ResolveRefResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ResolveRef {
                owner: bob.to_string(),
                repo: "hello".to_string(),
                ref_name: "refs/heads/main".to_string(),
            },
        )
        .unwrap();
    assert_eq!(resolved.commit_sha, SHA_A);
    assert_eq!(resolved.pack_uris, vec!["ipfs://cid1"]);

    // bob pushing to his fork does not touch the source
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &update_ref_msg(&bob, "hello", "refs/heads/main", SHA_C, vec!["cid9"], Some(SHA_A), false),
            &[],
        )
        .unwrap();
    let src: ResolveRefResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ResolveRef {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                ref_name: "refs/heads/main".to_string(),
            },
        )
        .unwrap();
    assert_eq!(src.commit_sha, SHA_A);

    // forking again into the same name collides
    let err = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::ForkRepo {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                name: None,
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RepoExists { .. }
    ));

    // frozen repos cannot be forked
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                status: ModerationStatus::Frozen,
                reason_hash: None,
            },
            &[],
        )
        .unwrap();
    let err = env
        .app
        .execute_contract(
            env.carol.clone(),
            env.contract.clone(),
            &ExecuteMsg::ForkRepo {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                name: Some("hello2".to_string()),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RepoFrozen { .. }
    ));
}

#[test]
fn fork_copies_only_v1_metadata_and_ref_snapshot() {
    let (mut env, _treasury) = setup_funded();
    let (alice, bob, carol) = (env.alice.clone(), env.bob.clone(), env.carol.clone());
    create_repo(&mut env, &alice, "source");
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::UpdateRepoInfo {
                repo: "source".to_string(),
                description: Some("fork snapshot".to_string()),
                default_branch: Some("release".to_string()),
            },
            &[],
        )
        .unwrap();
    for (ref_name, sha, cid) in [
        ("refs/heads/release", SHA_A, "release-cid"),
        ("refs/tags/v1", SHA_B, "tag-cid"),
    ] {
        env.app
            .execute_contract(
                alice.clone(),
                env.contract.clone(),
                &update_ref_msg(&alice, "source", ref_name, sha, vec![cid], None, false),
                &[],
            )
            .unwrap();
    }
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetCollaborator {
                repo: "source".to_string(),
                collaborator: carol.to_string(),
                role: Some(Role::Maintainer),
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetRevenueSplits {
                repo: "source".to_string(),
                splits: vec![SplitRecipient {
                    address: carol.to_string(),
                    bps: 1000,
                }],
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::Sponsor {
                owner: alice.to_string(),
                repo: "source".to_string(),
                message: None,
            },
            &coins(INJ, "inj"),
        )
        .unwrap();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetGuardians {
                repo: "source".to_string(),
                guardians: vec![carol.to_string()],
                threshold: 1,
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::AwardBadge {
                repo: "source".to_string(),
                recipient: carol.to_string(),
                reason: "source-only badge".to_string(),
            },
            &[],
        )
        .unwrap();
    env.app
        .execute_contract(
            carol.clone(),
            env.contract.clone(),
            &ExecuteMsg::SubmitModerationReport {
                owner: alice.to_string(),
                repo: "source".to_string(),
                reason_hash: "source-only-report".to_string(),
            },
            &[],
        )
        .unwrap();

    env.app
        .update_block(|block| block.time = block.time.plus_seconds(60));
    let forked_at = env.app.block_info().time.seconds();
    let response = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::ForkRepo {
                owner: alice.to_string(),
                repo: "source".to_string(),
                name: Some("copy".to_string()),
            },
            &[],
        )
        .unwrap();
    let wasm_event = response
        .events
        .iter()
        .find(|event| event.ty == "wasm")
        .expect("fork must emit wasm attributes");
    for (key, value) in [
        ("action", "fork_repo"),
        ("source_owner", alice.as_str()),
        ("source_repo", "source"),
        ("owner", bob.as_str()),
        ("repo", "copy"),
        ("refs", "2"),
    ] {
        assert!(
            wasm_event
                .attributes
                .iter()
                .any(|attribute| attribute.key == key && attribute.value == value),
            "missing {key}={value} in {wasm_event:?}"
        );
    }

    let fork = repo_info(&env, &bob, "copy");
    assert_eq!(fork.description, "fork snapshot");
    assert_eq!(fork.default_branch, "release");
    assert_eq!(fork.created_at, forked_at);
    assert_eq!(fork.updated_at, forked_at);
    assert_eq!(fork.moderation_status, ModerationStatus::Active);
    assert_eq!(fork.forked_from, Some(format!("{alice}/source")));

    let fork_refs: ListRefsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRefs {
                owner: bob.to_string(),
                repo: "copy".to_string(),
                start_after: None,
                limit: Some(100),
            },
        )
        .unwrap();
    assert_eq!(fork_refs.refs.len(), 2);
    for entry in &fork_refs.refs {
        assert_eq!(entry.updated_at, forked_at);
        assert_eq!(entry.updated_by, bob.to_string());
    }
    let release = fork_refs
        .refs
        .iter()
        .find(|entry| entry.ref_name == "refs/heads/release")
        .expect("release ref");
    assert_eq!(release.commit_sha, SHA_A);
    assert_eq!(release.pack_uris, vec!["ipfs://release-cid"]);

    let collaborators: ListCollaboratorsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListCollaborators {
                owner: bob.to_string(),
                repo: "copy".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert!(collaborators.collaborators.is_empty());
    let splits: RevenueSplitsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::RevenueSplits {
                owner: bob.to_string(),
                repo: "copy".to_string(),
            },
        )
        .unwrap();
    assert!(splits.splits.is_empty());
    let totals: SponsorTotalsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::SponsorTotals {
                owner: bob.to_string(),
                repo: "copy".to_string(),
            },
        )
        .unwrap();
    assert!(totals.totals.is_empty());
    let security: OwnershipSecurityResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::OwnershipSecurity {
                owner: bob.to_string(),
                repo: "copy".to_string(),
            },
        )
        .unwrap();
    assert!(security.guardians.is_empty());
    assert_eq!(security.guardian_threshold, 0);
    let badges: BadgesResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::BadgesByRepo {
                owner: bob.to_string(),
                repo: "copy".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert!(badges.badges.is_empty());

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: alice.to_string(),
                repo: "source".to_string(),
                status: ModerationStatus::Delisted,
                reason_hash: None,
            },
            &[],
        )
        .unwrap();
    let delisted = env
        .app
        .execute_contract(
            carol,
            env.contract.clone(),
            &ExecuteMsg::ForkRepo {
                owner: alice.to_string(),
                repo: "source".to_string(),
                name: Some("delisted-copy".to_string()),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        delisted.downcast::<ContractError>().unwrap(),
        ContractError::RepoFrozen { .. }
    ));
}

#[test]
fn fork_copies_refs_beyond_the_default_query_page() {
    let mut env = setup();
    let (alice, bob) = (env.alice.clone(), env.bob.clone());
    create_repo(&mut env, &alice, "many-refs");
    for index in 0..35 {
        env.app
            .execute_contract(
                alice.clone(),
                env.contract.clone(),
                &ExecuteMsg::UpdateRef {
                    owner: alice.to_string(),
                    repo: "many-refs".to_string(),
                    ref_name: format!("refs/heads/branch-{index:03}"),
                    commit_sha: SHA_A.to_string(),
                    pack_uris: vec![format!("ipfs://cid-{index:03}")],
                    expected_sha: None,
                    force: false,
                },
                &[],
            )
            .unwrap();
    }

    let response = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::ForkRepo {
                owner: alice.to_string(),
                repo: "many-refs".to_string(),
                name: None,
            },
            &[],
        )
        .unwrap();
    let wasm_event = response
        .events
        .iter()
        .find(|event| event.ty == "wasm")
        .expect("fork must emit wasm attributes");
    assert!(wasm_event
        .attributes
        .iter()
        .any(|attribute| attribute.key == "refs" && attribute.value == "35"));

    let first_page: ListRefsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRefs {
                owner: bob.to_string(),
                repo: "many-refs".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(first_page.refs.len(), 30);
    let second_page: ListRefsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ListRefs {
                owner: bob.to_string(),
                repo: "many-refs".to_string(),
                start_after: Some(
                    first_page
                        .refs
                        .last()
                        .expect("first page ref")
                        .ref_name
                        .clone(),
                ),
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(second_page.refs.len(), 5);
    let last: ResolveRefResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ResolveRef {
                owner: bob.to_string(),
                repo: "many-refs".to_string(),
                ref_name: "refs/heads/branch-034".to_string(),
            },
        )
        .unwrap();
    assert_eq!(last.pack_uris, vec!["ipfs://cid-034"]);
}

// ---- v3: sponsorship, revenue splits, usernames ----

#[test]
fn award_badge_and_trophy_wall() {
    let mut env = setup();
    let (alice, bob, carol) = (env.alice.clone(), env.bob.clone(), env.carol.clone());
    create_repo(&mut env, &alice, "hello");

    // owner awards bob twice, carol once
    for (rcpt, reason) in [
        (&bob, "fixed the flaky CI"),
        (&carol, "great bug report"),
        (&bob, "reviewed the v2 refactor"),
    ] {
        env.app
            .execute_contract(
                alice.clone(),
                env.contract.clone(),
                &ExecuteMsg::AwardBadge {
                    repo: "hello".to_string(),
                    recipient: rcpt.to_string(),
                    reason: reason.to_string(),
                },
                &[],
            )
            .unwrap();
    }

    // bob's trophy wall has 2 badges with reasons
    let wall: BadgesResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::BadgesByRecipient {
                recipient: bob.to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(wall.badges.len(), 2);
    assert_eq!(wall.badges[0].reason, "fixed the flaky CI");
    assert_eq!(wall.badges[0].repo_name, "hello");

    // repo index sees all 3
    let by_repo: BadgesResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::BadgesByRepo {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                start_after: None,
                limit: None,
            },
        )
        .unwrap();
    assert_eq!(by_repo.badges.len(), 3);

    // self-award rejected
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::AwardBadge {
                repo: "hello".to_string(),
                recipient: alice.to_string(),
                reason: "me".to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::OwnerAsCollaborator {}
    ));

    // non-owner cannot award
    let err = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::AwardBadge {
                repo: "hello".to_string(),
                recipient: carol.to_string(),
                reason: "nope".to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RepoNotFound { .. }
    ));

    // frozen repo stops issuing badges (alice is fallback moderator)
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                status: ModerationStatus::Frozen,
                reason_hash: None,
            },
            &[],
        )
        .unwrap();
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::AwardBadge {
                repo: "hello".to_string(),
                recipient: bob.to_string(),
                reason: "frozen".to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RepoFrozen { .. }
    ));
}

#[test]
fn award_badge_rejects_delisted_repo_like_frozen_repo() {
    let mut env = setup();
    let (alice, bob) = (env.alice.clone(), env.bob.clone());
    create_repo(&mut env, &alice, "delisted-badge");

    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: alice.to_string(),
                repo: "delisted-badge".to_string(),
                status: ModerationStatus::Delisted,
                reason_hash: Some("characterize-delisted-badge".to_string()),
            },
            &[],
        )
        .unwrap();

    let err = env
        .app
        .execute_contract(
            alice,
            env.contract.clone(),
            &ExecuteMsg::AwardBadge {
                repo: "delisted-badge".to_string(),
                recipient: bob.to_string(),
                reason: "delisted parity".to_string(),
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RepoFrozen { .. }
    ));
}

const INJ: u128 = 1_000_000_000_000_000_000; // 1 INJ in base units

/// Funded environment: dave is the treasury, carol holds spendable INJ.
fn setup_funded() -> (TestEnv, Addr) {
    let mut app = App::new(|router, api, storage| {
        for (name, amount) in [("alice", 10 * INJ), ("bob", 10 * INJ), ("carol", 10 * INJ)] {
            router
                .bank
                .init_balance(storage, &api.addr_make(name), coins(amount, "inj"))
                .unwrap();
        }
    });
    let code = ContractWrapper::new(
        repo_registry::contract::execute,
        repo_registry::contract::instantiate,
        repo_registry::contract::query,
    )
    .with_migrate(repo_registry::contract::migrate);
    let code_id = app.store_code(Box::new(code));
    let alice = app.api().addr_make("alice");
    let bob = app.api().addr_make("bob");
    let carol = app.api().addr_make("carol");
    let dave = app.api().addr_make("dave");
    let contract = app
        .instantiate_contract(
            code_id,
            alice.clone(),
            &InstantiateMsg {
                admin: None,
                moderation_committee: None,
                treasury: Some(dave.to_string()),
                platform_fee_bps: None, // default 300 = 3%
                username_deposit: None, // default 0.1 INJ
                username_fee: Some(cosmwasm_std::coin(INJ / 100, "inj")),
                reserved_usernames: None,
            },
            &[],
            "repo-registry",
            Some(alice.to_string()),
        )
        .unwrap();
    (
        TestEnv {
            app,
            contract,
            alice,
            bob,
            carol,
        },
        dave,
    )
}

fn balance(env: &TestEnv, addr: &Addr) -> u128 {
    env.app
        .wrap()
        .query_balance(addr, "inj")
        .unwrap()
        .amount
        .u128()
}

#[test]
fn sponsor_splits_fee_and_shares() {
    let (mut env, dave) = setup_funded();
    let (alice, bob, carol) = (env.alice.clone(), env.bob.clone(), env.carol.clone());
    create_repo(&mut env, &alice, "hello");

    // alice grants bob a 20% share
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetRevenueSplits {
                repo: "hello".to_string(),
                splits: vec![SplitRecipient {
                    address: bob.to_string(),
                    bps: 2000,
                }],
            },
            &[],
        )
        .unwrap();

    let (a0, b0, d0) = (balance(&env, &alice), balance(&env, &bob), balance(&env, &dave));

    // carol sponsors 1 INJ
    env.app
        .execute_contract(
            carol.clone(),
            env.contract.clone(),
            &ExecuteMsg::Sponsor {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                message: Some("great work".to_string()),
            },
            &coins(INJ, "inj"),
        )
        .unwrap();

    // fee 3% -> dave; of the rest, 20% -> bob, remainder -> alice
    let fee = INJ * 300 / 10_000;
    let distributable = INJ - fee;
    let bob_share = distributable * 2000 / 10_000;
    let alice_share = distributable - bob_share;
    assert_eq!(balance(&env, &dave) - d0, fee);
    assert_eq!(balance(&env, &bob) - b0, bob_share);
    assert_eq!(balance(&env, &alice) - a0, alice_share);

    // lifetime totals recorded
    let totals: SponsorTotalsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::SponsorTotals {
                owner: alice.to_string(),
                repo: "hello".to_string(),
            },
        )
        .unwrap();
    assert_eq!(totals.totals.len(), 1);
    assert_eq!(totals.totals[0].amount.u128(), INJ);

    // sponsoring with no funds is rejected
    let err = env
        .app
        .execute_contract(
            carol.clone(),
            env.contract.clone(),
            &ExecuteMsg::Sponsor {
                owner: alice.to_string(),
                repo: "hello".to_string(),
                message: None,
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::NoFunds {}
    ));
}

#[test]
fn sponsor_frozen_repo_rejected() {
    let (mut env, _dave) = setup_funded();
    let (alice, bob, carol) = (env.alice.clone(), env.bob.clone(), env.carol.clone());
    create_repo(&mut env, &bob, "bobrepo");
    // alice is admin -> fallback moderator
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetModerationStatus {
                owner: bob.to_string(),
                repo: "bobrepo".to_string(),
                status: ModerationStatus::Frozen,
                reason_hash: None,
            },
            &[],
        )
        .unwrap();
    let err = env
        .app
        .execute_contract(
            carol.clone(),
            env.contract.clone(),
            &ExecuteMsg::Sponsor {
                owner: bob.to_string(),
                repo: "bobrepo".to_string(),
                message: None,
            },
            &coins(INJ, "inj"),
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RepoFrozen { .. }
    ));
}

#[test]
fn revenue_splits_validation() {
    let (mut env, _dave) = setup_funded();
    let (alice, bob) = (env.alice.clone(), env.bob.clone());
    create_repo(&mut env, &alice, "hello");

    // sum > 10000 rejected
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetRevenueSplits {
                repo: "hello".to_string(),
                splits: vec![
                    SplitRecipient {
                        address: bob.to_string(),
                        bps: 8000,
                    },
                    SplitRecipient {
                        address: env.carol.to_string(),
                        bps: 3000,
                    },
                ],
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::InvalidSplits { .. }
    ));

    // owner in the table rejected (owner gets the remainder implicitly)
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetRevenueSplits {
                repo: "hello".to_string(),
                splits: vec![SplitRecipient {
                    address: alice.to_string(),
                    bps: 1000,
                }],
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::InvalidSplits { .. }
    ));

    // only the owner may set splits
    let err = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::SetRevenueSplits {
                repo: "hello".to_string(),
                splits: vec![],
            },
            &[],
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::RepoNotFound { .. }
    ));
}

#[test]
fn username_register_and_release() {
    let (mut env, dave) = setup_funded();
    let (alice, bob) = (env.alice.clone(), env.bob.clone());
    let deposit = INJ / 10; // default 0.1 INJ
    let fee = INJ / 100; // configured non-refundable registration fee
    let registration_cost = deposit + fee;

    // wrong deposit rejected
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::RegisterUsername {
                name: "alice-dev".to_string(),
            },
            &coins(deposit / 2, "inj"),
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::DepositMismatch { .. }
    ));

    // exact deposit registers
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::RegisterUsername {
                name: "alice-dev".to_string(),
            },
            &coins(registration_cost, "inj"),
        )
        .unwrap();

    // duplicate name rejected
    let err = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::RegisterUsername {
                name: "alice-dev".to_string(),
            },
            &coins(deposit, "inj"),
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::UsernameTaken { .. }
    ));

    // reserved names are rejected even when the registration fee is paid
    let err = env
        .app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::RegisterUsername {
                name: "admin".to_string(),
            },
            &coins(registration_cost, "inj"),
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::UsernameReserved { .. }
    ));

    // one name per address
    let err = env
        .app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::RegisterUsername {
                name: "alice-two".to_string(),
            },
            &coins(registration_cost, "inj"),
        )
        .unwrap_err();
    assert!(matches!(
        err.downcast::<ContractError>().unwrap(),
        ContractError::AlreadyHasUsername { .. }
    ));

    // invalid names rejected
    for bad in ["ab", "-abc", "abc-", "inj1abcdef", "Has-Upper"] {
        let err = env
            .app
            .execute_contract(
                bob.clone(),
                env.contract.clone(),
                &ExecuteMsg::RegisterUsername {
                    name: bad.to_string(),
                },
                &coins(registration_cost, "inj"),
            )
            .unwrap_err();
        assert!(matches!(
            err.downcast::<ContractError>().unwrap(),
            ContractError::InvalidUsername { .. }
        ));
    }

    // release refunds the deposit and frees the name
    let a0 = balance(&env, &alice);
    env.app
        .execute_contract(
            alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::ReleaseUsername {},
            &[],
        )
        .unwrap();
    assert_eq!(balance(&env, &alice) - a0, deposit);
    assert_eq!(balance(&env, &dave), fee);
    env.app
        .execute_contract(
            bob.clone(),
            env.contract.clone(),
            &ExecuteMsg::RegisterUsername {
                name: "alice-dev".to_string(),
            },
            &coins(registration_cost, "inj"),
        )
        .unwrap();
}

#[test]
fn admin_registers_immutable_release_checksums() {
    let mut env = setup();
    let version = "v0.5.0".to_string();
    let digest = "ABCDEFabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123";
    env.app
        .execute_contract(
            env.alice.clone(),
            env.contract.clone(),
            &ExecuteMsg::RegisterRelease {
                version: version.clone(),
                artifacts: vec![
                    ReleaseArtifactInput {
                        platform: "linux-amd64".to_string(),
                        sha256: digest.to_string(),
                    },
                    ReleaseArtifactInput {
                        platform: "repo-registry.wasm".to_string(),
                        sha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef".to_string(),
                    },
                ],
            },
            &[],
        )
        .unwrap();

    let out: ReleaseArtifactsResponse = env
        .app
        .wrap()
        .query_wasm_smart(
            &env.contract,
            &QueryMsg::ReleaseArtifacts { version: version.clone() },
        )
        .unwrap();
    assert_eq!(out.artifacts.len(), 2);
    assert_eq!(out.artifacts[0].sha256, digest.to_ascii_lowercase());

    let duplicate = env.app.execute_contract(
        env.alice.clone(),
        env.contract.clone(),
        &ExecuteMsg::RegisterRelease {
            version,
            artifacts: vec![ReleaseArtifactInput {
                platform: "linux-amd64".to_string(),
                sha256: digest.to_string(),
            }],
        },
        &[],
    );
    assert!(matches!(
        duplicate.unwrap_err().downcast::<ContractError>().unwrap(),
        ContractError::ReleaseArtifactExists { .. }
    ));

    let unauthorized = env.app.execute_contract(
        env.bob.clone(),
        env.contract.clone(),
        &ExecuteMsg::RegisterRelease {
            version: "v0.5.1".to_string(),
            artifacts: vec![ReleaseArtifactInput {
                platform: "linux-amd64".to_string(),
                sha256: digest.to_string(),
            }],
        },
        &[],
    );
    assert!(matches!(
        unauthorized.unwrap_err().downcast::<ContractError>().unwrap(),
        ContractError::Unauthorized {}
    ));
}
