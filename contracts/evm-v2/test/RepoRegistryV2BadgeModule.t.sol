// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

import {RepoRegistryV2} from "../src/RepoRegistryV2.sol";
import {RepoRegistryV2BadgeModule} from "../src/RepoRegistryV2BadgeModule.sol";

interface BadgeVm {
    function prank(address sender) external;
    function expectRevert(bytes calldata revertData) external;
    function expectEmit(bool checkTopic1, bool checkTopic2, bool checkTopic3, bool checkData) external;
    function warp(uint256 newTimestamp) external;
}

contract RepoRegistryV2BadgeModuleTest {
    BadgeVm internal constant vm = BadgeVm(address(uint160(uint256(keccak256("hevm cheat code")))));
    RepoRegistryV2 internal registry;
    RepoRegistryV2BadgeModule internal badges;
    address internal alice = address(0xA11CE);
    address internal bob = address(0xB0B);
    address internal carol = address(0xCA701);

    event BadgeAwarded(
        uint256 indexed id,
        bytes32 indexed repoId,
        address indexed recipient,
        address awardedBy,
        string reason,
        uint64 awardedAt
    );

    function setUp() public {
        registry = new RepoRegistryV2(address(this));
        badges = new RepoRegistryV2BadgeModule(address(registry));
        vm.prank(alice);
        registry.createRepo("demo", "A demo repository", "main");
    }

    function testAwardAndBoundedRecipientAndRepoPages() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.warp(1234);
        vm.expectEmit(true, true, true, true);
        emit BadgeAwarded(1, repoId, bob, alice, "fixed CI", 1234);
        vm.prank(alice);
        uint256 id = badges.awardBadge(repoId, bob, "fixed CI");
        require(id == 1, "badge id");

        vm.prank(alice);
        badges.awardBadge(repoId, carol, "reviewed the patch");

        (uint256 next, bool more, RepoRegistryV2BadgeModule.Badge[] memory byRepo) = badges
            .listBadgesByRepoPage(repoId, 0, 1);
        require(next == 1 && more && byRepo.length == 1, "first repo page");
        require(byRepo[0].repoId == repoId && byRepo[0].recipient == bob, "stable identity");

        RepoRegistryV2BadgeModule.Badge[] memory byRecipient;
        (next, more, byRecipient) = badges.listBadgesByRecipientPage(carol, 0, 64);
        require(next == 1 && !more && byRecipient.length == 1, "recipient page");
        require(keccak256(bytes(byRecipient[0].reason)) == keccak256(bytes("reviewed the patch")), "reason");
    }

    function testOwnerAndActiveStatusAreReadFromCoreRegistry() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2BadgeModule.Unauthorized.selector, bob));
        vm.prank(bob);
        badges.awardBadge(repoId, carol, "not owner");

        vm.prank(alice);
        registry.setFrozen(alice, "demo", true);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2BadgeModule.RepoNotActive.selector, repoId, 1));
        vm.prank(alice);
        badges.awardBadge(repoId, bob, "frozen");
    }

    function testDelistedStatusAlsoRejectsBadgeLikeV1() public {
        RepoRegistryV2 importedRegistry = new RepoRegistryV2(address(this));
        RepoRegistryV2BadgeModule importedBadges = new RepoRegistryV2BadgeModule(
            address(importedRegistry)
        );
        bytes32 snapshotHash = bytes32(uint256(0xD31157ED));
        bytes32 repoId = keccak256(
            abi.encode(
                "igit:v2:import",
                importedRegistry.identityChainId(),
                address(importedRegistry),
                "injective-888",
                address(0xC05),
                alice,
                "delisted"
            )
        );
        bytes32 payloadHash = sha256(
            abi.encode(repoId, alice, "delisted", "", "main", uint8(2), uint64(1), uint64(2), uint256(0), uint256(0))
        );
        importedRegistry.createImportSession(
            snapshotHash,
            "injective-888",
            address(0xC05),
            1,
            1,
            0,
            0,
            1
        );
        importedRegistry.importRepo(
            snapshotHash,
            0,
            repoId,
            alice,
            "delisted",
            "",
            "main",
            2,
            1,
            2,
            0,
            0,
            payloadHash
        );
        importedRegistry.finalizeImport(snapshotHash);

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2BadgeModule.RepoNotActive.selector, repoId, 2)
        );
        vm.prank(alice);
        importedBadges.awardBadge(repoId, bob, "delisted parity");
    }

    function testOwnershipTransferKeepsBadgeIdentityAndChangesAwardAuthority() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.prank(alice);
        badges.awardBadge(repoId, carol, "before transfer");

        vm.prank(alice);
        registry.beginOwnershipTransfer(repoId, bob);
        RepoRegistryV2.PendingOwnershipTransfer memory pending = registry.pendingOwnershipTransfer(repoId);
        vm.warp(pending.executeAfter);
        vm.prank(bob);
        registry.acceptOwnership(repoId);

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2BadgeModule.Unauthorized.selector, alice));
        vm.prank(alice);
        badges.awardBadge(repoId, carol, "old owner");
        vm.prank(bob);
        badges.awardBadge(repoId, alice, "new owner");

        (, , RepoRegistryV2BadgeModule.Badge[] memory page) = badges.listBadgesByRepoPage(repoId, 0, 64);
        require(page.length == 2, "badges survive transfer");
        require(page[0].awardedBy == alice && page[1].awardedBy == bob, "award authority");
    }

    function testRejectsSelfZeroAndInvalidReasonLengths() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2BadgeModule.InvalidRecipient.selector, alice));
        vm.prank(alice);
        badges.awardBadge(repoId, alice, "self");

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2BadgeModule.InvalidRecipient.selector, address(0)));
        vm.prank(alice);
        badges.awardBadge(repoId, address(0), "zero");

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2BadgeModule.InvalidReasonLength.selector, 0, 256));
        vm.prank(alice);
        badges.awardBadge(repoId, bob, "");

        bytes memory raw = new bytes(257);
        for (uint256 i; i < raw.length; ++i) raw[i] = bytes1("a");
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2BadgeModule.InvalidReasonLength.selector, 257, 256));
        vm.prank(alice);
        badges.awardBadge(repoId, bob, string(raw));
    }
}
