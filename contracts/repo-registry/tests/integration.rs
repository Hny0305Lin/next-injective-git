use cosmwasm_std::Addr;
use cw_multi_test::{App, ContractWrapper, Executor};

use repo_registry::msg::{
    ExecuteMsg, InstantiateMsg, ListRefsResponse, ListReposResponse, QueryMsg, RepoInfoResponse,
    ResolveRefResponse,
};
use repo_registry::state::Role;
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
    );
    let code_id = app.store_code(Box::new(code));
    let alice = app.api().addr_make("alice");
    let bob = app.api().addr_make("bob");
    let carol = app.api().addr_make("carol");
    let contract = app
        .instantiate_contract(
            code_id,
            alice.clone(),
            &InstantiateMsg { admin: None },
            &[],
            "repo-registry",
            None,
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
        packfile_cids: cids.into_iter().map(String::from).collect(),
        expected_sha: expected.map(String::from),
        force,
    }
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
    assert_eq!(resolved.packfile_cids, vec!["cid1"]);

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
    assert_eq!(resolved.packfile_cids, vec!["cid1", "cid2"]);
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
    assert_eq!(resolved.packfile_cids, vec!["cid9"]);
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
        ContractError::EmptyPackfileCids {}
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
