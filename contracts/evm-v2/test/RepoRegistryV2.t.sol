// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

import {RepoRegistryV2} from "../src/RepoRegistryV2.sol";

interface Vm {
    function prank(address sender) external;
    function startPrank(address sender) external;
    function stopPrank() external;
    function expectRevert(bytes calldata revertData) external;
    function expectEmit(bool checkTopic1, bool checkTopic2, bool checkTopic3, bool checkData) external;
    function warp(uint256 newTimestamp) external;
}

abstract contract RepoRegistryV2TestBase {
    Vm internal constant vm = Vm(address(uint160(uint256(keccak256("hevm cheat code")))));
    RepoRegistryV2 internal registry;
    address internal alice = address(0xA11CE);
    address internal bob = address(0xB0B);
    address internal carol = address(0xCA701);
    string internal constant SHA1 = "0123456789abcdef0123456789abcdef01234567";
    string internal constant SHA2 = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789";

    event RepoInfoUpdated(
        bytes32 indexed repoId,
        address indexed owner,
        string repo,
        uint8 fieldMask,
        string description,
        string defaultBranch
    );
    event OwnershipTransferStarted(
        bytes32 indexed repoId,
        address indexed oldOwner,
        address indexed newOwner,
        string name,
        uint64 proposedAt,
        uint64 executeAfter,
        uint64 expiresAt
    );
    event OwnershipTransferCancelled(bytes32 indexed repoId, address indexed owner, address target);
    event OwnershipTransferred(
        bytes32 indexed repoId,
        address indexed oldOwner,
        address indexed newOwner,
        address acceptedBy,
        string name,
        uint64 executeAfter
    );

    function oneUri() internal pure returns (string[] memory uris) {
        uris = new string[](1);
        uris[0] = "ipfs://cid";
    }

    function sizedString(uint256 length) internal pure returns (string memory value) {
        bytes memory data = new bytes(length);
        for (uint256 i; i < length; ++i) data[i] = bytes1("a");
        value = string(data);
    }

    function sameString(string memory left, string memory right) internal pure returns (bool) {
        return keccak256(bytes(left)) == keccak256(bytes(right));
    }

    function setUp() public virtual {
        registry = new RepoRegistryV2(address(this));
        vm.prank(alice);
        registry.createRepo("demo", "A demo repository", "main");
    }

}

abstract contract RepoRegistryV2NativeTestBase is RepoRegistryV2TestBase {
    function testCreateAndGetRepo() public view {
        RepoRegistryV2.Repo memory repo = registry.getRepo(alice, "demo");
        require(repo.owner == alice, "owner");
        require(keccak256(bytes(repo.name)) == keccak256(bytes("demo")), "name");
        require(repo.exists, "exists");
    }
}

contract RepoRegistryV2IdentityTest is RepoRegistryV2NativeTestBase {

    function testStableRepoIdUsesDomainSeparatedCreationIdentity() public view {
        bytes32 expected = keccak256(
            abi.encode(
                "igit:v2:repo",
                registry.identityChainId(),
                address(registry),
                alice,
                "demo"
            )
        );
        bytes32 derived = registry.computeRepoId(alice, "demo");
        require(derived == expected, "repo id formula");
        require(registry.computeRepoId(alice, "demo") == expected, "compat id helper");

        (bytes32 resolved, bool isCanonical, RepoRegistryV2.Repo memory repo) = registry
            .resolveRepo(alice, "demo");
        require(resolved == expected, "resolved id");
        require(isCanonical, "new locator is not canonical");
        require(repo.owner == alice, "resolved owner");
        require(registry.computeRepoId(bob, "demo") != expected, "owner domain");
        require(registry.computeRepoId(alice, "Demo") != expected, "name domain");
    }

    function testOwnershipTransferReservesTargetAndEmitsStarted() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.warp(1_000);

        uint64 executeAfter = uint64(1_000 + registry.OWNERSHIP_TRANSFER_DELAY());
        uint64 expiresAt = uint64(
            uint256(executeAfter) + registry.OWNERSHIP_TRANSFER_ACCEPTANCE_WINDOW()
        );
        vm.expectEmit(true, true, true, true);
        emit OwnershipTransferStarted(repoId, alice, bob, "demo", 1_000, executeAfter, expiresAt);
        vm.prank(alice);
        registry.beginOwnershipTransfer(repoId, bob);

        RepoRegistryV2.PendingOwnershipTransfer memory pending = registry
            .pendingOwnershipTransfer(repoId);
        require(pending.newOwner == bob, "pending target");
        require(pending.proposedAt == 1_000, "pending proposed at");
        require(pending.executeAfter == executeAfter, "pending execute after");
        require(pending.expiresAt == expiresAt, "pending expires at");

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.LocatorUnavailable.selector, bob, "demo")
        );
        vm.prank(bob);
        registry.createRepo("demo", "must remain reserved", "main");

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.TransferAlreadyPending.selector, repoId)
        );
        vm.prank(alice);
        registry.beginOwnershipTransfer(repoId, carol);
    }

    function testOwnershipTransferAcceptIsStableAndOldLocatorIsReadOnly() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.warp(2_000);
        vm.prank(alice);
        registry.setCollaborator(alice, "demo", bob, RepoRegistryV2.Role.Reader);
        vm.prank(alice);
        registry.setCollaborator(alice, "demo", carol, RepoRegistryV2.Role.Maintainer);
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, oneUri(), "", false);

        RepoRegistryV2.Repo memory repoBefore = registry.getRepoById(repoId);
        RepoRegistryV2.Ref memory refBefore = registry.resolveRef(
            alice,
            "demo",
            "refs/heads/main"
        );

        vm.prank(alice);
        registry.beginOwnershipTransfer(repoId, bob);
        RepoRegistryV2.PendingOwnershipTransfer memory pending = registry
            .pendingOwnershipTransfer(repoId);

        vm.warp(uint256(pending.executeAfter) - 1);
        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.TransferTooEarly.selector,
                repoId,
                pending.executeAfter
            )
        );
        vm.prank(bob);
        registry.acceptOwnership(repoId);

        vm.warp(pending.executeAfter);
        vm.expectEmit(true, true, true, true);
        emit OwnershipTransferred(repoId, alice, bob, bob, "demo", pending.executeAfter);
        vm.prank(bob);
        registry.acceptOwnership(repoId);

        (bytes32 oldResolved, bool oldCanonical, RepoRegistryV2.Repo memory oldRead) = registry
            .resolveRepo(alice, "demo");
        (bytes32 newResolved, bool newCanonical, RepoRegistryV2.Repo memory newRead) = registry
            .resolveRepo(bob, "demo");
        require(oldResolved == repoId && newResolved == repoId, "repo id changed");
        require(!oldCanonical && newCanonical, "locator state");
        require(oldRead.owner == bob && newRead.owner == bob, "canonical owner");
        require(registry.computeRepoId(bob, "demo") != repoId, "id was rederived");

        RepoRegistryV2.Repo memory repoAfter = registry.getRepoById(repoId);
        require(repoAfter.createdAt == repoBefore.createdAt, "createdAt changed");
        require(repoAfter.updatedAt == repoBefore.updatedAt, "updatedAt changed");
        require(sameString(repoAfter.description, repoBefore.description), "metadata changed");
        RepoRegistryV2.Ref memory refAfter = registry.resolveRef(
            alice,
            "demo",
            "refs/heads/main"
        );
        require(sameString(refAfter.commitSha, refBefore.commitSha), "ref sha changed");
        require(refAfter.updatedAt == refBefore.updatedAt, "ref timestamp changed");
        require(refAfter.updatedBy == refBefore.updatedBy, "ref updater changed");
        require(refAfter.packUris.length == refBefore.packUris.length, "ref packs changed");

        require(
            registry.getCollaborator(bob, "demo", bob) == RepoRegistryV2.Role.None,
            "new owner collaborator retained"
        );
        require(
            registry.getCollaborator(alice, "demo", carol) == RepoRegistryV2.Role.Maintainer,
            "other collaborator changed"
        );
        (,, address[] memory collaborators, RepoRegistryV2.Role[] memory roles) = registry
            .listCollaboratorsPageById(repoId, 0, 1);
        require(collaborators.length == 1 && collaborators[0] == carol, "collaborator index");
        require(roles[0] == RepoRegistryV2.Role.Maintainer, "collaborator role");

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.RepoMoved.selector, repoId, bob, "demo")
        );
        vm.prank(bob);
        registry.updateRef(alice, "demo", "refs/heads/old-url", SHA2, oneUri(), "", false);

        vm.prank(bob);
        registry.updateRef(bob, "demo", "refs/heads/new-url", SHA2, oneUri(), "", false);
    }

    function testTransferCancelRejectAndExpiryReleaseReservations() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.warp(3_000);
        vm.prank(alice);
        registry.beginOwnershipTransfer(repoId, bob);

        vm.expectEmit(true, true, false, true);
        emit OwnershipTransferCancelled(repoId, alice, bob);
        vm.prank(alice);
        registry.cancelOwnershipTransfer(repoId);
        vm.prank(bob);
        registry.createRepo("demo", "released after cancel", "main");

        vm.prank(alice);
        registry.createRepo("reject-me", "reject target", "main");
        bytes32 rejectId = registry.computeRepoId(alice, "reject-me");
        vm.prank(alice);
        registry.beginOwnershipTransfer(rejectId, carol);
        vm.prank(carol);
        registry.rejectOwnershipTransfer(rejectId);
        vm.prank(carol);
        registry.createRepo("reject-me", "released after reject", "main");

        vm.prank(alice);
        registry.createRepo("expire-me", "expiry target", "main");
        bytes32 expireId = registry.computeRepoId(alice, "expire-me");
        vm.prank(alice);
        registry.beginOwnershipTransfer(expireId, carol);
        RepoRegistryV2.PendingOwnershipTransfer memory pending = registry
            .pendingOwnershipTransfer(expireId);
        vm.warp(uint256(pending.expiresAt) + 1);

        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.TransferExpired.selector,
                expireId,
                pending.expiresAt
            )
        );
        vm.prank(carol);
        registry.acceptOwnership(expireId);

        vm.prank(bob);
        registry.expireOwnershipTransfer(expireId);
        vm.prank(carol);
        registry.createRepo("expire-me", "released after expiry", "main");
    }

    function testSameRepoHistoricalAliasCanBecomeCanonicalAgain() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.prank(alice);
        registry.beginOwnershipTransfer(repoId, bob);
        RepoRegistryV2.PendingOwnershipTransfer memory first = registry
            .pendingOwnershipTransfer(repoId);
        vm.warp(first.executeAfter);
        vm.prank(bob);
        registry.acceptOwnership(repoId);

        vm.prank(bob);
        registry.beginOwnershipTransfer(repoId, alice);
        RepoRegistryV2.PendingOwnershipTransfer memory second = registry
            .pendingOwnershipTransfer(repoId);
        (bytes32 duringPending, bool pendingCanonical,) = registry.resolveRepo(alice, "demo");
        require(duringPending == repoId && !pendingCanonical, "reserved alias stopped reading");
        vm.warp(second.executeAfter);
        vm.prank(alice);
        registry.acceptOwnership(repoId);

        (bytes32 aliceId, bool aliceCanonical, RepoRegistryV2.Repo memory repo) = registry
            .resolveRepo(alice, "demo");
        (bytes32 bobId, bool bobCanonical,) = registry.resolveRepo(bob, "demo");
        require(aliceId == repoId && bobId == repoId, "return changed identity");
        require(aliceCanonical && !bobCanonical, "return locator states");
        require(repo.owner == alice, "return owner");
    }

    function testHistoricalAliasCannotBeReusedByAnotherRepo() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.prank(alice);
        registry.beginOwnershipTransfer(repoId, bob);
        RepoRegistryV2.PendingOwnershipTransfer memory pending = registry
            .pendingOwnershipTransfer(repoId);
        vm.warp(pending.executeAfter);
        vm.prank(bob);
        registry.acceptOwnership(repoId);

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.LocatorUnavailable.selector, alice, "demo")
        );
        vm.prank(alice);
        registry.createRepo("demo", "alias cannot be recreated", "main");

        vm.prank(carol);
        registry.createRepo("demo", "another identity", "main");
        bytes32 otherRepoId = registry.computeRepoId(carol, "demo");
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.LocatorUnavailable.selector, alice, "demo")
        );
        vm.prank(carol);
        registry.beginOwnershipTransfer(otherRepoId, alice);
    }
}

contract RepoRegistryV2PaginationTest is RepoRegistryV2NativeTestBase {
    function testListReposPageDrainsStableIdsAndTracksOwnershipTransfer() public {
        vm.startPrank(alice);
        bytes32 secondId = registry.createRepo("second", "Second repository", "main");
        bytes32 thirdId = registry.createRepo("third", "Third repository", "develop");
        vm.stopPrank();
        bytes32 firstId = registry.computeRepoId(alice, "demo");

        (
            uint256 nextCursor,
            bool hasMore,
            bytes32[] memory firstIds,
            RepoRegistryV2.Repo[] memory firstRepos
        ) = registry.listReposPage(alice, 0, 2);
        require(nextCursor == 2 && hasMore, "first repo cursor");
        require(firstIds.length == 2 && firstRepos.length == 2, "first repo page length");
        require(firstIds[0] == firstId && sameString(firstRepos[0].name, "demo"), "first repo");
        require(firstIds[1] == secondId && sameString(firstRepos[1].name, "second"), "second repo");

        (
            uint256 finalCursor,
            bool finalHasMore,
            bytes32[] memory finalIds,
            RepoRegistryV2.Repo[] memory finalRepos
        ) = registry.listReposPage(alice, nextCursor, 2);
        require(finalCursor == 3 && !finalHasMore, "final repo cursor");
        require(finalIds.length == 1 && finalRepos.length == 1, "final repo page length");
        require(finalIds[0] == thirdId && sameString(finalRepos[0].name, "third"), "third repo");

        vm.prank(alice);
        registry.beginOwnershipTransfer(secondId, bob);
        RepoRegistryV2.PendingOwnershipTransfer memory pending = registry
            .pendingOwnershipTransfer(secondId);
        vm.warp(pending.executeAfter);
        vm.prank(bob);
        registry.acceptOwnership(secondId);

        (uint256 aliceCursor, bool aliceHasMore, bytes32[] memory aliceIds,) = registry
            .listReposPage(alice, 0, 2);
        require(aliceCursor == 2 && !aliceHasMore && aliceIds.length == 2, "alice transfer page");
        require(aliceIds[0] == firstId && aliceIds[1] == thirdId, "alice swap-pop index");

        (uint256 bobCursor, bool bobHasMore, bytes32[] memory bobIds, RepoRegistryV2.Repo[] memory bobRepos) = registry
            .listReposPage(bob, 0, 1);
        require(bobCursor == 1 && !bobHasMore && bobIds.length == 1, "bob transfer page");
        require(bobIds[0] == secondId && bobRepos[0].owner == bob, "bob transferred repo");

        // The first transfer swap-pops thirdId into a new index. A second
        // transfer proves the moved entry's reverse index was updated.
        vm.prank(alice);
        registry.beginOwnershipTransfer(thirdId, bob);
        pending = registry.pendingOwnershipTransfer(thirdId);
        vm.warp(pending.executeAfter);
        vm.prank(bob);
        registry.acceptOwnership(thirdId);

        (uint256 remainingCursor, bool remainingHasMore, bytes32[] memory remainingIds,) = registry
            .listReposPage(alice, 0, registry.MAX_QUERY_PAGE_SIZE());
        require(remainingCursor == 1 && !remainingHasMore && remainingIds.length == 1, "alice remaining page");
        require(remainingIds[0] == firstId, "alice remaining repo");

        (uint256 bobFinalCursor, bool bobFinalHasMore, bytes32[] memory bobFinalIds,) = registry
            .listReposPage(bob, 0, registry.MAX_QUERY_PAGE_SIZE());
        require(bobFinalCursor == 2 && !bobFinalHasMore && bobFinalIds.length == 2, "bob final page");
        require(bobFinalIds[0] == secondId && bobFinalIds[1] == thirdId, "bob final repos");
    }

    function testListReposPageAcceptsEmptyEndAndRejectsInvalidBounds() public {
        uint256 maxPageSize = registry.MAX_QUERY_PAGE_SIZE();
        (uint256 emptyCursor, bool hasMore, bytes32[] memory ids, RepoRegistryV2.Repo[] memory repos) = registry
            .listReposPage(bob, 0, 1);
        require(emptyCursor == 0 && !hasMore && ids.length == 0 && repos.length == 0, "empty owner page");

        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.InvalidPageSize.selector,
                uint256(0),
                maxPageSize
            )
        );
        registry.listReposPage(alice, 0, 0);

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.InvalidCursor.selector, uint256(2), uint256(1))
        );
        registry.listReposPage(alice, 2, 1);

        (uint256 endCursor, bool endHasMore, bytes32[] memory endIds, RepoRegistryV2.Repo[] memory endRepos) = registry
            .listReposPage(alice, 1, maxPageSize);
        require(endCursor == 1 && !endHasMore && endIds.length == 0 && endRepos.length == 0, "non-empty end page");

        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.InvalidPageSize.selector,
                maxPageSize + 1,
                maxPageSize
            )
        );
        registry.listReposPage(alice, 0, maxPageSize + 1);
    }

    function testListRefsPageByStableIdDrainsEveryRef() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.startPrank(alice);
        registry.updateRef(alice, "demo", "refs/heads/one", SHA1, oneUri(), "", false);
        registry.updateRef(alice, "demo", "refs/heads/two", SHA1, oneUri(), "", false);
        registry.updateRef(alice, "demo", "refs/heads/three", SHA2, oneUri(), "", false);
        vm.stopPrank();

        (
            uint256 nextCursor,
            bool hasMore,
            string[] memory firstNames,
            RepoRegistryV2.Ref[] memory firstValues
        ) = registry.listRefsPageById(repoId, 0, 2);
        require(nextCursor == 2 && hasMore, "first ref cursor");
        require(firstNames.length == 2 && firstValues.length == 2, "first ref page length");
        require(sameString(firstNames[0], "refs/heads/one"), "first ref name");
        require(sameString(firstNames[1], "refs/heads/two"), "second ref name");
        require(sameString(firstValues[0].commitSha, SHA1), "first ref value");

        (
            uint256 finalCursor,
            bool finalHasMore,
            string[] memory finalNames,
            RepoRegistryV2.Ref[] memory finalValues
        ) = registry.listRefsPageById(repoId, nextCursor, 2);
        require(finalCursor == 3 && !finalHasMore, "final ref cursor");
        require(finalNames.length == 1 && finalValues.length == 1, "final ref page length");
        require(sameString(finalNames[0], "refs/heads/three"), "final ref name");
        require(sameString(finalValues[0].commitSha, SHA2), "final ref value");

        (uint256 emptyCursor, bool emptyHasMore, string[] memory emptyNames,) = registry
            .listRefsPageById(repoId, finalCursor, 2);
        require(emptyCursor == 3 && !emptyHasMore && emptyNames.length == 0, "empty final page");
    }

    function testListCollaboratorsPageByStableIdDrainsEveryRole() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        address dave = address(0xDA7E);
        vm.startPrank(alice);
        registry.setCollaborator(alice, "demo", bob, RepoRegistryV2.Role.Reader);
        registry.setCollaborator(alice, "demo", carol, RepoRegistryV2.Role.Maintainer);
        registry.setCollaborator(alice, "demo", dave, RepoRegistryV2.Role.Reader);
        vm.stopPrank();

        (
            uint256 nextCursor,
            bool hasMore,
            address[] memory firstCollaborators,
            RepoRegistryV2.Role[] memory firstRoles
        ) = registry.listCollaboratorsPageById(repoId, 0, 2);
        require(nextCursor == 2 && hasMore, "first collaborator cursor");
        require(firstCollaborators.length == 2 && firstRoles.length == 2, "first collaborator page");
        require(firstCollaborators[0] == bob && firstRoles[0] == RepoRegistryV2.Role.Reader, "bob role");
        require(
            firstCollaborators[1] == carol && firstRoles[1] == RepoRegistryV2.Role.Maintainer,
            "carol role"
        );

        (
            uint256 finalCursor,
            bool finalHasMore,
            address[] memory finalCollaborators,
            RepoRegistryV2.Role[] memory finalRoles
        ) = registry.listCollaboratorsPageById(repoId, nextCursor, 2);
        require(finalCursor == 3 && !finalHasMore, "final collaborator cursor");
        require(finalCollaborators.length == 1 && finalRoles.length == 1, "final collaborator page");
        require(
            finalCollaborators[0] == dave && finalRoles[0] == RepoRegistryV2.Role.Reader,
            "dave role"
        );
    }

    function testStableIdPaginationRejectsInvalidBoundsAndUnknownRepo() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.InvalidPageSize.selector,
                uint256(0),
                registry.MAX_QUERY_PAGE_SIZE()
            )
        );
        registry.listRefsPageById(repoId, 0, 0);

        uint256 oversized = registry.MAX_QUERY_PAGE_SIZE() + 1;
        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.InvalidPageSize.selector,
                oversized,
                registry.MAX_QUERY_PAGE_SIZE()
            )
        );
        registry.listCollaboratorsPageById(repoId, 0, oversized);

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.InvalidCursor.selector, uint256(1), uint256(0))
        );
        registry.listRefsPageById(repoId, 1, 1);

        bytes32 unknown = keccak256("unknown");
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.RepoNotFound.selector, unknown));
        registry.listRefsPageById(unknown, 0, 1);
    }
}

contract RepoRegistryV2MetadataTest is RepoRegistryV2NativeTestBase {

    function testUpdateRepoInfoPartialPatchAndIgnoredValues() public {
        vm.warp(100);
        vm.prank(alice);
        registry.updateRepoInfo("demo", true, "Updated description", false, sizedString(65));

        RepoRegistryV2.Repo memory afterDescription = registry.getRepo(alice, "demo");
        require(sameString(afterDescription.description, "Updated description"), "description patch");
        require(sameString(afterDescription.defaultBranch, "main"), "branch must be preserved");
        require(afterDescription.updatedAt == 100, "description timestamp");

        vm.warp(101);
        vm.prank(alice);
        registry.updateRepoInfo("demo", false, sizedString(1025), true, "develop");

        RepoRegistryV2.Repo memory afterBranch = registry.getRepo(alice, "demo");
        require(sameString(afterBranch.description, "Updated description"), "description must be preserved");
        require(sameString(afterBranch.defaultBranch, "develop"), "branch patch");
        require(afterBranch.updatedAt == 101, "branch timestamp");
    }

    function testUpdateRepoInfoCanExplicitlyClearFields() public {
        vm.warp(110);
        vm.prank(alice);
        registry.updateRepoInfo("demo", true, "", true, "");

        RepoRegistryV2.Repo memory repo = registry.getRepo(alice, "demo");
        require(bytes(repo.description).length == 0, "description clear");
        require(bytes(repo.defaultBranch).length == 0, "branch clear");
        require(repo.updatedAt == 110, "clear timestamp");
    }

    function testUpdateRepoInfoNoOpAndSameValuesAdvanceTimestamp() public {
        RepoRegistryV2.Repo memory initial = registry.getRepo(alice, "demo");

        vm.warp(120);
        vm.prank(alice);
        registry.updateRepoInfo("demo", false, sizedString(1025), false, sizedString(65));

        RepoRegistryV2.Repo memory afterNoOp = registry.getRepo(alice, "demo");
        require(sameString(afterNoOp.description, initial.description), "no-op description");
        require(sameString(afterNoOp.defaultBranch, initial.defaultBranch), "no-op branch");
        require(afterNoOp.updatedAt == 120, "no-op timestamp");

        vm.warp(121);
        vm.prank(alice);
        registry.updateRepoInfo(
            "demo",
            true,
            afterNoOp.description,
            true,
            afterNoOp.defaultBranch
        );

        RepoRegistryV2.Repo memory afterSameValues = registry.getRepo(alice, "demo");
        require(afterSameValues.updatedAt == 121, "same-value timestamp");
    }

    function testUpdateRepoInfoIsOwnerOnlyNotCollaboratorWritable() public {
        vm.prank(alice);
        registry.setCollaborator(alice, "demo", carol, RepoRegistryV2.Role.Maintainer);
        vm.prank(alice);
        registry.setCollaborator(alice, "demo", bob, RepoRegistryV2.Role.Reader);

        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.LocatorNotFound.selector,
                carol,
                "demo"
            )
        );
        vm.prank(carol);
        registry.updateRepoInfo("demo", true, "maintainer edit", false, "");

        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.LocatorNotFound.selector,
                bob,
                "demo"
            )
        );
        vm.prank(bob);
        registry.updateRepoInfo("demo", true, "reader edit", false, "");

        address stranger = address(0xD00D);
        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.LocatorNotFound.selector,
                stranger,
                "demo"
            )
        );
        vm.prank(stranger);
        registry.updateRepoInfo("demo", true, "stranger edit", false, "");

        vm.prank(alice);
        registry.updateRepoInfo("demo", true, "owner edit", false, "");
        require(
            sameString(registry.getRepo(alice, "demo").description, "owner edit"),
            "owner update"
        );
    }

    function testUpdateRepoInfoUsesSenderOwnedSameNameNamespace() public {
        vm.prank(alice);
        registry.setCollaborator(alice, "demo", bob, RepoRegistryV2.Role.Reader);
        vm.prank(bob);
        registry.createRepo("demo", "Bob's repository", "trunk");

        vm.warp(130);
        vm.prank(bob);
        registry.updateRepoInfo("demo", true, "Bob's update", true, "stable");

        RepoRegistryV2.Repo memory aliceRepo = registry.getRepo(alice, "demo");
        RepoRegistryV2.Repo memory bobRepo = registry.getRepo(bob, "demo");
        require(sameString(aliceRepo.description, "A demo repository"), "alice namespace changed");
        require(sameString(aliceRepo.defaultBranch, "main"), "alice branch changed");
        require(sameString(bobRepo.description, "Bob's update"), "bob description");
        require(sameString(bobRepo.defaultBranch, "stable"), "bob branch");
        require(bobRepo.updatedAt == 130, "bob timestamp");
    }

    function testUpdateRepoInfoAllowedWhileFrozen() public {
        vm.prank(alice);
        registry.setFrozen(alice, "demo", true);

        vm.warp(140);
        vm.prank(alice);
        registry.updateRepoInfo("demo", true, "Frozen metadata update", true, "release");

        RepoRegistryV2.Repo memory repo = registry.getRepo(alice, "demo");
        require(repo.moderationStatus == 1, "repo unexpectedly unfrozen");
        require(sameString(repo.description, "Frozen metadata update"), "frozen description");
        require(sameString(repo.defaultBranch, "release"), "frozen branch");
        require(repo.updatedAt == 140, "frozen timestamp");
    }

    function testUpdateRepoInfoOversizeRejectedAtomically() public {
        RepoRegistryV2.Repo memory beforeUpdate = registry.getRepo(alice, "demo");
        vm.warp(150);
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.InputTooLong.selector, uint256(1025), uint256(1024))
        );
        vm.prank(alice);
        registry.updateRepoInfo("demo", true, sizedString(1025), true, "develop");
        _requireRepoMetadataUnchanged(beforeUpdate);

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.InputTooLong.selector, uint256(65), uint256(64))
        );
        vm.prank(alice);
        registry.updateRepoInfo("demo", true, "must roll back", true, sizedString(65));
        _requireRepoMetadataUnchanged(beforeUpdate);
    }

    function testUpdateRepoInfoEmitsFinalValuesAndFieldMasks() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");

        vm.expectEmit(true, true, false, true);
        emit RepoInfoUpdated(repoId, alice, "demo", 0, "A demo repository", "main");
        vm.prank(alice);
        registry.updateRepoInfo("demo", false, "ignored", false, "ignored");

        vm.expectEmit(true, true, false, true);
        emit RepoInfoUpdated(repoId, alice, "demo", 1, "Event description", "main");
        vm.prank(alice);
        registry.updateRepoInfo("demo", true, "Event description", false, "ignored");

        vm.expectEmit(true, true, false, true);
        emit RepoInfoUpdated(repoId, alice, "demo", 2, "Event description", "release");
        vm.prank(alice);
        registry.updateRepoInfo("demo", false, "ignored", true, "release");
    }

    function _requireRepoMetadataUnchanged(RepoRegistryV2.Repo memory expected) private view {
        RepoRegistryV2.Repo memory actual = registry.getRepo(alice, "demo");
        require(sameString(actual.description, expected.description), "description changed on revert");
        require(sameString(actual.defaultBranch, expected.defaultBranch), "branch changed on revert");
        require(actual.updatedAt == expected.updatedAt, "timestamp changed on revert");
    }

}

contract RepoRegistryV2Test is RepoRegistryV2NativeTestBase {

    function testOwnerCanUpdateResolveAndList() public {
        string[] memory uris = new string[](1);
        uris[0] = "ipfs://cid";
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, uris, "", false);

        RepoRegistryV2.Ref memory current = registry.resolveRef(alice, "demo", "refs/heads/main");
        require(keccak256(bytes(current.commitSha)) == keccak256(bytes(SHA1)), "sha");
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        (,, string[] memory names, RepoRegistryV2.Ref[] memory refs) = registry
            .listRefsPageById(repoId, 0, 1);
        require(names.length == 1 && refs.length == 1, "list length");
        require(keccak256(bytes(names[0])) == keccak256(bytes("refs/heads/main")), "ref name");
    }

    function testNormalUpdateAppendsPacksAndForceReplaces() public {
        string[] memory first = new string[](1);
        first[0] = "ipfs://first";
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, first, "", false);

        string[] memory second = new string[](1);
        second[0] = "ipfs://second";
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA2, second, SHA1, false);
        RepoRegistryV2.Ref memory appended = registry.resolveRef(alice, "demo", "refs/heads/main");
        require(appended.packUris.length == 2, "normal update must append");

        string[] memory replacement = new string[](1);
        replacement[0] = "ipfs://replacement";
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, replacement, "", true);
        RepoRegistryV2.Ref memory forced = registry.resolveRef(alice, "demo", "refs/heads/main");
        require(forced.packUris.length == 1, "force update must replace");
        require(keccak256(bytes(forced.packUris[0])) == keccak256(bytes("ipfs://replacement")), "replacement URI");
    }

    function testNormalUpdateCannotGrowStoredPackUrisPastLimit() public {
        uint256 maximum = registry.MAX_PACK_URIS();
        string[] memory maximumUris = new string[](maximum);
        for (uint256 i; i < maximum; ++i) maximumUris[i] = "ipfs://existing";

        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, maximumUris, "", true);

        string[] memory additional = new string[](1);
        additional[0] = "ipfs://additional";
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.TooManyPackUris.selector, maximum + 1, maximum)
        );
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA2, additional, SHA1, false);

        RepoRegistryV2.Ref memory unchanged = registry.resolveRef(alice, "demo", "refs/heads/main");
        require(unchanged.packUris.length == maximum, "pack URI limit changed");
        require(keccak256(bytes(unchanged.commitSha)) == keccak256(bytes(SHA1)), "sha changed on revert");
    }

    function testExpectedShaAndForce() public {
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, oneUri(), "", false);

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.ShaMismatch.selector, "", SHA1));
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA2, oneUri(), "", false);

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.ShaMismatch.selector, SHA2, SHA1));
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA2, oneUri(), SHA2, false);

        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA2, oneUri(), SHA2, true);
    }

    function testOnlyOwnerCanWriteAndDelete() public {
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.Unauthorized.selector, bob));
        vm.prank(bob);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, oneUri(), "", false);

        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, oneUri(), "", false);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.Unauthorized.selector, bob));
        vm.prank(bob);
        registry.deleteRef(alice, "demo", "refs/heads/main");
        vm.prank(alice);
        registry.deleteRef(alice, "demo", "refs/heads/main");
    }

    function testMaintainerCanWriteReaderCannotAndOwnerCanRevoke() public {
        vm.prank(alice);
        registry.setCollaborator(alice, "demo", carol, RepoRegistryV2.Role.Maintainer);
        vm.prank(alice);
        registry.setCollaborator(alice, "demo", bob, RepoRegistryV2.Role.Reader);
        require(
            registry.getCollaborator(alice, "demo", carol) == RepoRegistryV2.Role.Maintainer,
            "maintainer role"
        );
        require(
            registry.getCollaborator(alice, "demo", bob) == RepoRegistryV2.Role.Reader,
            "reader role"
        );

        vm.prank(carol);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, oneUri(), "", false);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.Unauthorized.selector, bob));
        vm.prank(bob);
        registry.updateRef(alice, "demo", "refs/heads/other", SHA2, oneUri(), "", false);

        vm.prank(alice);
        registry.setCollaborator(alice, "demo", carol, RepoRegistryV2.Role.None);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.Unauthorized.selector, carol));
        vm.prank(carol);
        registry.deleteRef(alice, "demo", "refs/heads/main");
    }

    function testFrozenRepoRejectsOwnerAndMaintainerWritesAndAdminCanUnfreeze() public {
        vm.prank(alice);
        registry.setCollaborator(alice, "demo", carol, RepoRegistryV2.Role.Maintainer);
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, oneUri(), "", false);
        vm.prank(alice);
        registry.setFrozen(alice, "demo", true);

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.FrozenRepo.selector, registry.computeRepoId(alice, "demo")));
        vm.prank(carol);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA2, oneUri(), SHA1, false);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.FrozenRepo.selector, registry.computeRepoId(alice, "demo")));
        vm.prank(alice);
        registry.deleteRef(alice, "demo", "refs/heads/main");

        registry.setFrozen(alice, "demo", false);
        vm.prank(carol);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA2, oneUri(), SHA1, false);
    }

    function testCommitShaAndPackUriValidation() public {
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.InvalidCommitSha.selector, "abc123"));
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", "abc123", oneUri(), "", false);

        string[] memory badUris = new string[](1);
        badUris[0] = "https://example.invalid/pack";
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.InvalidPackUri.selector, badUris[0])
        );
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, badUris, "", false);

        badUris[0] = "ipfs:///missing-cid";
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.InvalidPackUri.selector, badUris[0])
        );
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA1, badUris, "", false);

        string[] memory goodUris = new string[](1);
        goodUris[0] = "ipfs://bafybeigdyrzt";
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/main", SHA2, goodUris, "", false);

        string[] memory noUris = new string[](0);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.EmptyPackUris.selector));
        vm.prank(alice);
        registry.updateRef(alice, "demo", "refs/heads/empty", SHA1, noUris, "", false);
    }

    function testRepoAndRefValidationMatchesV1Shape() public {
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.InvalidRepoName.selector, "bad/name"));
        vm.prank(alice);
        registry.createRepo("bad/name", "", "main");

        string[] memory empty = new string[](1);
        empty[0] = "ipfs://cid";
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.InvalidRefName.selector, "main"));
        vm.prank(alice);
        registry.updateRef(alice, "demo", "main", SHA1, oneUri(), "", false);
    }

    function testFuzzInvalidCommitShaLength(uint8 rawLength) public {
        uint256 length = uint256(rawLength) % 80;
        if (length == 40 || length == 64) return;
        bytes memory value = new bytes(length);
        for (uint256 i; i < length; ++i) value[i] = bytes1("a");
        string memory invalidSha = string(value);

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.InvalidCommitSha.selector, invalidSha)
        );
        vm.prank(alice);
        registry.updateRef(
            alice,
            "demo",
            "refs/heads/fuzz",
            invalidSha,
            oneUri(),
            "",
            false
        );
    }

    function testDeploymentAdminIsStable() public view {
        require(registry.admin() == address(this), "admin changed");
    }

    function testRepositoryIdentityAndCanonicalLocatorAgree() public view {
        (bytes32 repoId,, RepoRegistryV2.Repo memory repo) = registry.resolveRepo(alice, "demo");
        RepoRegistryV2.Repo memory byId = registry.getRepoById(repoId);
        (bytes32 canonicalId, bool isCanonical,) = registry.resolveRepo(repo.owner, repo.name);
        require(canonicalId == repoId, "canonical locator changed identity");
        require(isCanonical, "repo owner/name is not canonical");
        require(byId.owner == repo.owner, "repo owner mismatch");
        require(sameString(byId.name, repo.name), "repo name mismatch");
    }
}

contract RepoRegistryV2ImportTest is RepoRegistryV2TestBase {
    string internal constant SOURCE_CHAIN = "injective-888";
    address internal constant SOURCE_CONTRACT = address(0xC05);
    bytes32 internal constant SNAPSHOT_HASH = bytes32(uint256(0x5151));

    function setUp() public override {
        registry = new RepoRegistryV2(address(this));
    }

    event ImportRepoApplied(
        bytes32 indexed sessionId,
        uint256 indexed sequence,
        bytes32 indexed repoId,
        bytes32 payloadHash,
        address owner,
        string name,
        uint8 moderationStatus,
        uint256 expectedRefs,
        uint256 expectedCollaborators
    );
    event ImportFinalized(
        bytes32 indexed sessionId,
        bytes32 indexed snapshotHash,
        uint256 repositories,
        uint256 refs,
        uint256 collaborators,
        uint256 batches
    );

    function importedRepoId(address owner, string memory name) internal view returns (bytes32) {
        return keccak256(
            abi.encode(
                "igit:v2:import",
                registry.identityChainId(),
                address(registry),
                SOURCE_CHAIN,
                SOURCE_CONTRACT,
                owner,
                name
            )
        );
    }

    function startImport(
        bytes32 snapshotHash,
        uint256 repositories,
        uint256 refs,
        uint256 collaborators,
        uint256 batches
    ) internal {
        bytes32 sessionId = registry.createImportSession(
            snapshotHash,
            SOURCE_CHAIN,
            SOURCE_CONTRACT,
            4_242,
            repositories,
            refs,
            collaborators,
            batches
        );
        require(sessionId == snapshotHash, "session id");
        require(registry.activeImportSession() == snapshotHash, "active session");
    }

    function oneImportRef() internal view returns (RepoRegistryV2.ImportRef[] memory refs) {
        refs = new RepoRegistryV2.ImportRef[](1);
        refs[0] = RepoRegistryV2.ImportRef({
            refName: "refs/heads/main",
            commitSha: SHA1,
            packUris: oneUri(),
            updatedAt: 123,
            updatedBy: bob
        });
    }

    function oneImportCollaborator()
        internal
        view
        returns (RepoRegistryV2.ImportCollaborator[] memory collaborators)
    {
        collaborators = new RepoRegistryV2.ImportCollaborator[](1);
        collaborators[0] = RepoRegistryV2.ImportCollaborator({
            account: carol,
            role: RepoRegistryV2.Role.Maintainer
        });
    }

    function computeRepoPayloadHash(
        bytes32 repoId,
        address owner,
        string memory name,
        string memory description,
        string memory defaultBranch,
        uint8 moderationStatus,
        uint64 createdAt,
        uint64 updatedAt,
        uint256 expectedRefs,
        uint256 expectedCollaborators
    ) internal pure returns (bytes32) {
        return sha256(
            abi.encode(
                repoId,
                owner,
                name,
                description,
                defaultBranch,
                moderationStatus,
                createdAt,
                updatedAt,
                expectedRefs,
                expectedCollaborators
            )
        );
    }

    function refsPayloadHash(RepoRegistryV2.ImportRef[] memory refs) internal pure returns (bytes32) {
        return sha256(abi.encode(refs));
    }

    function collaboratorsPayloadHash(
        RepoRegistryV2.ImportCollaborator[] memory collaborators
    ) internal pure returns (bytes32) {
        return sha256(abi.encode(collaborators));
    }

    function testNativeRepoPermanentlyClosesImportWindow() public {
        vm.prank(alice);
        registry.createRepo("native", "", "main");

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.ImportWindowClosed.selector));
        registry.createImportSession(
            SNAPSHOT_HASH,
            SOURCE_CHAIN,
            SOURCE_CONTRACT,
            4_242,
            0,
            0,
            0,
            0
        );
    }

    function testImportProgressSurvivesFinalizationAndWindowStaysClosed() public {
        startImport(SNAPSHOT_HASH, 1, 0, 0, 1);
        (
            bool existsBefore,
            bool activeBefore,
            bool finalizedBefore,
            uint256 nextBefore,
            uint256 batchesBefore,
            uint256 reposBefore,
            uint256 expectedReposBefore,
            ,
            ,
            ,
            ,
            uint256 incompleteBefore
        ) = registry.importProgress(SNAPSHOT_HASH);
        require(existsBefore && activeBefore && !finalizedBefore, "active progress flags");
        require(nextBefore == 0 && batchesBefore == 1, "active progress sequence");
        require(reposBefore == 0 && expectedReposBefore == 1, "active progress repos");
        require(incompleteBefore == 0, "active progress incomplete");

        bytes32 repoId = importedRepoId(bob, "legacy");
        bytes32 payloadHash = computeRepoPayloadHash(
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            0,
            0
        );
        registry.importRepo(
            SNAPSHOT_HASH,
            0,
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            0,
            0,
            payloadHash
        );
        registry.finalizeImport(SNAPSHOT_HASH);

        (
            bool existsAfter,
            bool activeAfter,
            bool finalizedAfter,
            uint256 nextAfter,
            uint256 batchesAfter,
            uint256 reposAfter,
            uint256 expectedReposAfter,
            ,
            ,
            ,
            ,
            uint256 incompleteAfter
        ) = registry.importProgress(SNAPSHOT_HASH);
        require(existsAfter && !activeAfter && finalizedAfter, "final progress flags");
        require(nextAfter == 1 && batchesAfter == 1, "final progress sequence");
        require(reposAfter == 1 && expectedReposAfter == 1, "final progress repos");
        require(incompleteAfter == 0, "final progress incomplete");

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.ImportWindowClosed.selector));
        registry.createImportSession(
            bytes32(uint256(0x6161)),
            SOURCE_CHAIN,
            SOURCE_CONTRACT,
            4_243,
            0,
            0,
            0,
            0
        );
    }

    function testImportedDelistedRepoRemainsWritableAndInvalidStatusFailsClosed() public {
        startImport(SNAPSHOT_HASH, 1, 0, 0, 1);
        bytes32 repoId = importedRepoId(bob, "legacy");
        bytes32 invalidHash = computeRepoPayloadHash(
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            3,
            1,
            2,
            0,
            0
        );
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.InvalidImportStatus.selector, uint8(3))
        );
        registry.importRepo(
            SNAPSHOT_HASH,
            0,
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            3,
            1,
            2,
            0,
            0,
            invalidHash
        );

        bytes32 payloadHash = computeRepoPayloadHash(
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            2,
            1,
            2,
            0,
            0
        );
        registry.importRepo(
            SNAPSHOT_HASH,
            0,
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            2,
            1,
            2,
            0,
            0,
            payloadHash
        );
        registry.finalizeImport(SNAPSHOT_HASH);

        require(registry.getRepoById(repoId).moderationStatus == 2, "delisted status lost");
        vm.prank(bob);
        registry.updateRef(bob, "legacy", "refs/heads/main", SHA1, oneUri(), "", false);
        require(registry.resolveRef(bob, "legacy", "refs/heads/main").exists, "delisted push");
    }

    function testOrderedImportPreservesHistoricalMetadataAndFinalizes() public {
        startImport(SNAPSHOT_HASH, 1, 1, 1, 3);
        bytes32 repoId = importedRepoId(bob, "legacy");
        string memory historicalDescription = sizedString(
            registry.MAX_DESCRIPTION_LENGTH() + 128
        );
        string memory historicalBranch = sizedString(registry.MAX_NAME_LENGTH() + 16);
        uint8 moderationStatus = 0;
        bytes32 repoPayloadHash = computeRepoPayloadHash(
            repoId,
            bob,
            "legacy",
            historicalDescription,
            historicalBranch,
            moderationStatus,
            11,
            22,
            1,
            1
        );
        RepoRegistryV2.ImportRef[] memory refs = oneImportRef();
        RepoRegistryV2.ImportCollaborator[] memory collaborators = oneImportCollaborator();

        vm.expectEmit(true, true, true, true);
        emit ImportRepoApplied(
            SNAPSHOT_HASH,
            0,
            repoId,
            repoPayloadHash,
            bob,
            "legacy",
            moderationStatus,
            1,
            1
        );
        registry.importRepo(
            SNAPSHOT_HASH,
            0,
            repoId,
            bob,
            "legacy",
            historicalDescription,
            historicalBranch,
            moderationStatus,
            11,
            22,
            1,
            1,
            repoPayloadHash
        );
        registry.importRefs(
            SNAPSHOT_HASH,
            1,
            repoId,
            refs,
            refsPayloadHash(refs)
        );
        registry.importCollaborators(
            SNAPSHOT_HASH,
            2,
            repoId,
            collaborators,
            collaboratorsPayloadHash(collaborators)
        );

        vm.expectEmit(true, true, false, true);
        emit ImportFinalized(SNAPSHOT_HASH, SNAPSHOT_HASH, 1, 1, 1, 3);
        registry.finalizeImport(SNAPSHOT_HASH);
        require(registry.activeImportSession() == bytes32(0), "session remains active");

        RepoRegistryV2.Repo memory imported = registry.getRepo(bob, "legacy");
        require(imported.owner == bob && imported.exists, "imported repo identity");
        require(sameString(imported.description, historicalDescription), "description bytes");
        require(sameString(imported.defaultBranch, historicalBranch), "branch bytes");
        require(imported.createdAt == 11 && imported.updatedAt == 22, "timestamps");

        RepoRegistryV2.Ref memory importedRef = registry.resolveRef(
            bob,
            "legacy",
            "refs/heads/main"
        );
        require(sameString(importedRef.commitSha, SHA1), "ref sha");
        require(importedRef.updatedAt == 123 && importedRef.updatedBy == bob, "ref history");
        require(
            registry.getCollaborator(bob, "legacy", carol) == RepoRegistryV2.Role.Maintainer,
            "collaborator role"
        );

        (,, bytes32[] memory ids, RepoRegistryV2.Repo[] memory repos) = registry.listReposPage(
            bob,
            0,
            1
        );
        require(ids.length == 1 && ids[0] == repoId, "owner index id");
        require(repos.length == 1 && repos[0].owner == bob, "owner index repo");
    }

    function testImportLocksPublicStateAndFailedBatchDoesNotAdvance() public {
        startImport(SNAPSHOT_HASH, 1, 0, 0, 1);
        bytes32 repoId = importedRepoId(bob, "legacy");
        bytes32 payloadHash = computeRepoPayloadHash(
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            0,
            0
        );

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.ImportStateLocked.selector, SNAPSHOT_HASH)
        );
        registry.getRepo(alice, "demo");

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.ImportSequenceMismatch.selector, 0, 1)
        );
        registry.importRepo(
            SNAPSHOT_HASH,
            1,
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            0,
            0,
            payloadHash
        );

        bytes32 wrongId = bytes32(uint256(7));
        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.ImportRepoIdMismatch.selector,
                repoId,
                wrongId
            )
        );
        registry.importRepo(
            SNAPSHOT_HASH,
            0,
            wrongId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            0,
            0,
            payloadHash
        );

        registry.importRepo(
            SNAPSHOT_HASH,
            0,
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            0,
            0,
            payloadHash
        );
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2.ImportSequenceMismatch.selector, 1, 0)
        );
        registry.importRepo(
            SNAPSHOT_HASH,
            0,
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            0,
            0,
            payloadHash
        );

        registry.finalizeImport(SNAPSHOT_HASH);
        require(registry.getRepoById(repoId).exists, "final repo unavailable");
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.ImportWindowClosed.selector));
        registry.createImportSession(
            SNAPSHOT_HASH,
            SOURCE_CHAIN,
            SOURCE_CONTRACT,
            4_242,
            1,
            0,
            0,
            1
        );
    }

    function testImportCountAndFinalizationChecksAreEnforced() public {
        startImport(SNAPSHOT_HASH, 1, 1, 0, 2);
        bytes32 repoId = importedRepoId(bob, "legacy");
        bytes32 payloadHash = computeRepoPayloadHash(
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            1,
            0
        );

        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.ImportCountMismatch.selector,
                bytes32("refs"),
                1,
                2
            )
        );
        registry.importRepo(
            SNAPSHOT_HASH,
            0,
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            2,
            0,
            payloadHash
        );

        registry.importRepo(
            SNAPSHOT_HASH,
            0,
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            1,
            0,
            payloadHash
        );
        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.ImportCountMismatch.selector,
                bytes32("batches"),
                2,
                1
            )
        );
        registry.finalizeImport(SNAPSHOT_HASH);

        RepoRegistryV2.ImportRef[] memory emptyRefs = new RepoRegistryV2.ImportRef[](0);
        bytes32 emptyRefsPayloadHash = refsPayloadHash(emptyRefs);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.EmptyImportBatch.selector));
        registry.importRefs(
            SNAPSHOT_HASH,
            1,
            repoId,
            emptyRefs,
            emptyRefsPayloadHash
        );
        RepoRegistryV2.ImportRef[] memory refs = oneImportRef();
        registry.importRefs(
            SNAPSHOT_HASH,
            1,
            repoId,
            refs,
            refsPayloadHash(refs)
        );
        registry.finalizeImport(SNAPSHOT_HASH);
        require(registry.resolveRef(bob, "legacy", "refs/heads/main").exists, "ref missing");
    }

    function testOnlyAdminCanStartOrApplyImport() public {
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.Unauthorized.selector, alice));
        vm.prank(alice);
        registry.createImportSession(
            SNAPSHOT_HASH,
            SOURCE_CHAIN,
            SOURCE_CONTRACT,
            4_242,
            0,
            0,
            0,
            0
        );

        startImport(SNAPSHOT_HASH, 1, 0, 0, 1);
        bytes32 repoId = importedRepoId(bob, "legacy");
        bytes32 payloadHash = computeRepoPayloadHash(
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            0,
            0
        );
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2.Unauthorized.selector, alice));
        vm.prank(alice);
        registry.importRepo(
            SNAPSHOT_HASH,
            0,
            repoId,
            bob,
            "legacy",
            "description",
            "main",
            0,
            1,
            2,
            0,
            0,
            payloadHash
        );
    }
}
