// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

import {RepoRegistryV2} from "../src/RepoRegistryV2.sol";
import {RepoRegistryV2EconomicModule} from "../src/RepoRegistryV2EconomicModule.sol";

interface EconomicVm {
    function deal(address account, uint256 balance) external;
    function prank(address sender) external;
    function expectRevert(bytes calldata revertData) external;
    function warp(uint256 newTimestamp) external;
}

interface IEconomicReentryTarget {
    function setRevenueSplits(bytes32 repoId, address payable[] calldata recipients, uint16[] calldata bps) external;
}

contract RejectingEconomicRecipient {
    receive() external payable {
        revert("reject");
    }
}

contract ReenteringEconomicOwner {
    IEconomicReentryTarget internal economics;
    bytes32 internal repoId;
    bool public attempted;
    bool public blocked;

    function configure(IEconomicReentryTarget target, bytes32 targetRepoId) external {
        economics = target;
        repoId = targetRepoId;
    }

    receive() external payable {
        attempted = true;
        try economics.setRevenueSplits(repoId, new address payable[](0), new uint16[](0)) {
            blocked = false;
        } catch (bytes memory reason) {
            blocked = bytes4(reason) == RepoRegistryV2EconomicModule.ReentrantSettlement.selector;
        }
    }
}

contract RepoRegistryV2EconomicModuleTest {
    EconomicVm internal constant vm = EconomicVm(address(uint160(uint256(keccak256("hevm cheat code")))));
    RepoRegistryV2 internal registry;
    RepoRegistryV2EconomicModule internal economics;
    address internal alice = address(0xA11CE);
    address internal bob = address(0xB0B);
    address internal carol = address(0xCA701);
    address payable internal treasury = payable(address(0x7EA5));

    function setUp() public {
        registry = new RepoRegistryV2(address(this));
        economics = new RepoRegistryV2EconomicModule(address(registry), address(this), treasury, 300);
        vm.prank(alice);
        registry.createRepo("demo", "A demo repository", "main");
        vm.deal(carol, 10 ether);
    }

    function testSponsorSplitsFeeSharesAndOwnerRemainder() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        address payable[] memory recipients = new address payable[](1);
        recipients[0] = payable(bob);
        uint16[] memory shares = new uint16[](1);
        shares[0] = 2_000;
        vm.prank(alice);
        economics.setRevenueSplits(repoId, recipients, shares);

        uint256 aliceBefore = alice.balance;
        uint256 bobBefore = bob.balance;
        uint256 treasuryBefore = treasury.balance;
        vm.prank(carol);
        economics.sponsor{value: 1 ether}(repoId, "great work");

        uint256 fee = 0.03 ether;
        uint256 bobShare = ((1 ether - fee) * 2_000) / 10_000;
        require(treasury.balance - treasuryBefore == fee, "fee");
        require(bob.balance - bobBefore == bobShare, "split");
        require(alice.balance - aliceBefore == 1 ether - fee - bobShare, "owner remainder");
        require(economics.sponsorTotal(repoId) == 1 ether, "lifetime total");
        require(address(economics).balance == 0, "module retained funds");
    }

    function testFrozenRejectsSponsorButDelistedRemainsCompatible() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.prank(alice);
        registry.setFrozen(alice, "demo", true);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.FrozenRepo.selector, repoId));
        vm.prank(carol);
        economics.sponsor{value: 1 ether}(repoId, "frozen");
    }

    function testDelistedImportedRepoCanStillReceiveSponsorshipLikeV1() public {
        RepoRegistryV2 imported = new RepoRegistryV2(address(this));
        RepoRegistryV2EconomicModule importedEconomics = new RepoRegistryV2EconomicModule(
            address(imported), address(this), treasury, 300
        );
        bytes32 snapshotHash = keccak256("snapshot");
        bytes32 sessionId = imported.createImportSession(
            snapshotHash, "injective-888", address(0x1234), 99, 1, 0, 0, 1
        );
        bytes32 repoId = keccak256(
            abi.encode(
                "igit:v2:import",
                imported.identityChainId(),
                address(imported),
                "injective-888",
                address(0x1234),
                alice,
                "delisted"
            )
        );
        bytes32 payloadHash = sha256(
            abi.encode(repoId, alice, "delisted", "description", "main", uint8(2), uint64(1), uint64(2), 0, 0)
        );
        imported.importRepo(
            sessionId, 0, repoId, alice, "delisted", "description", "main", 2, 1, 2, 0, 0, payloadHash
        );
        imported.finalizeImport(sessionId);

        uint256 aliceBefore = alice.balance;
        vm.prank(carol);
        importedEconomics.sponsor{value: 1 ether}(repoId, "still payable");
        require(alice.balance - aliceBefore == 0.97 ether, "delisted owner payout");
        require(importedEconomics.sponsorTotal(repoId) == 1 ether, "delisted total");
    }

    function testSplitValidationAndOwnerAuthority() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        address payable[] memory recipients = new address payable[](1);
        recipients[0] = payable(bob);
        uint16[] memory shares = new uint16[](1);
        shares[0] = 1_000;

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.Unauthorized.selector, bob));
        vm.prank(bob);
        economics.setRevenueSplits(repoId, recipients, shares);

        recipients[0] = payable(alice);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.InvalidSplitRecipient.selector, alice));
        vm.prank(alice);
        economics.setRevenueSplits(repoId, recipients, shares);

        recipients = new address payable[](2);
        recipients[0] = payable(bob);
        recipients[1] = payable(bob);
        shares = new uint16[](2);
        shares[0] = 1_000;
        shares[1] = 2_000;
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.DuplicateSplitRecipient.selector, bob));
        vm.prank(alice);
        economics.setRevenueSplits(repoId, recipients, shares);
    }

    function testOwnershipTransferPreservesEconomicStateAndMovesAuthorityAndRemainder() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        address payable[] memory recipients = new address payable[](1);
        recipients[0] = payable(carol);
        uint16[] memory shares = new uint16[](1);
        shares[0] = 1_000;
        vm.prank(alice);
        economics.setRevenueSplits(repoId, recipients, shares);

        vm.prank(alice);
        registry.beginOwnershipTransfer(repoId, bob);
        RepoRegistryV2.PendingOwnershipTransfer memory pending = registry.pendingOwnershipTransfer(repoId);
        vm.warp(pending.executeAfter);
        vm.prank(bob);
        registry.acceptOwnership(repoId);

        RepoRegistryV2EconomicModule.Split[] memory stored = economics.revenueSplits(repoId);
        require(stored.length == 1 && stored[0].recipient == carol, "split survives transfer");
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.Unauthorized.selector, alice));
        vm.prank(alice);
        economics.setRevenueSplits(repoId, new address payable[](0), new uint16[](0));
        vm.prank(bob);
        economics.setRevenueSplits(repoId, new address payable[](0), new uint16[](0));

        uint256 bobBefore = bob.balance;
        vm.prank(carol);
        economics.sponsor{value: 1 ether}(repoId, "after transfer");
        require(bob.balance - bobBefore == 0.97 ether, "new owner remainder");
    }

    function testPayoutFailureRevertsAllBalancesAndTotals() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        RejectingEconomicRecipient rejecting = new RejectingEconomicRecipient();
        address payable[] memory recipients = new address payable[](1);
        recipients[0] = payable(address(rejecting));
        uint16[] memory shares = new uint16[](1);
        shares[0] = 1_000;
        vm.prank(alice);
        economics.setRevenueSplits(repoId, recipients, shares);

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.PayoutFailed.selector, address(rejecting), 0.097 ether));
        vm.prank(carol);
        economics.sponsor{value: 1 ether}(repoId, "atomic");
        require(economics.sponsorTotal(repoId) == 0, "total rolled back");
        require(address(economics).balance == 0, "value rolled back");
    }

    function testAdminFeeConfigurationIsCapped() public {
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.Unauthorized.selector, alice));
        vm.prank(alice);
        economics.setFeeConfig(payable(bob), 100);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.PlatformFeeTooHigh.selector, 501, 500));
        economics.setFeeConfig(payable(bob), 501);
        economics.setFeeConfig(payable(bob), 500);
        require(economics.treasury() == bob && economics.platformFeeBps() == 500, "fee config");
    }

    function testConstructorAndSponsorInputBounds() public {
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.InvalidRegistry.selector));
        new RepoRegistryV2EconomicModule(address(0), address(this), treasury, 0);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.InvalidAdmin.selector));
        new RepoRegistryV2EconomicModule(address(registry), address(0), treasury, 0);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.InvalidTreasury.selector));
        new RepoRegistryV2EconomicModule(address(registry), address(this), payable(address(0)), 0);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.PlatformFeeTooHigh.selector, 501, 500));
        new RepoRegistryV2EconomicModule(address(registry), address(this), treasury, 501);

        bytes32 repoId = registry.computeRepoId(alice, "demo");
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2EconomicModule.NoFunds.selector));
        economics.sponsor(repoId, "no value");
        bytes memory raw = new bytes(257);
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2EconomicModule.InvalidMessageLength.selector, 257, 256)
        );
        vm.prank(carol);
        economics.sponsor{value: 1 ether}(repoId, string(raw));
        require(economics.sponsorTotal(repoId) == 0, "invalid sponsor changed total");
    }

    function testEveryRevenueSplitValidationRevertsWithoutChangingState() public {
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        address payable[] memory initialRecipients = new address payable[](1);
        initialRecipients[0] = payable(bob);
        uint16[] memory initialBps = new uint16[](1);
        initialBps[0] = 1_000;
        vm.prank(alice);
        economics.setRevenueSplits(repoId, initialRecipients, initialBps);

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2EconomicModule.SplitLengthMismatch.selector, 1, 0)
        );
        vm.prank(alice);
        economics.setRevenueSplits(repoId, initialRecipients, new uint16[](0));

        address payable[] memory tooMany = new address payable[](21);
        uint16[] memory tooManyBps = new uint16[](21);
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2EconomicModule.TooManySplitRecipients.selector, 21, 20)
        );
        vm.prank(alice);
        economics.setRevenueSplits(repoId, tooMany, tooManyBps);

        address payable[] memory invalidRecipient = new address payable[](1);
        invalidRecipient[0] = payable(address(0));
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2EconomicModule.InvalidSplitRecipient.selector, address(0))
        );
        vm.prank(alice);
        economics.setRevenueSplits(repoId, invalidRecipient, initialBps);

        uint16[] memory zeroBps = new uint16[](1);
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2EconomicModule.InvalidSplitBps.selector, bob, 0)
        );
        vm.prank(alice);
        economics.setRevenueSplits(repoId, initialRecipients, zeroBps);

        address payable[] memory highRecipients = new address payable[](2);
        highRecipients[0] = payable(bob);
        highRecipients[1] = payable(carol);
        uint16[] memory highBps = new uint16[](2);
        highBps[0] = 6_000;
        highBps[1] = 5_000;
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2EconomicModule.SplitTotalTooHigh.selector, 11_000, 10_000)
        );
        vm.prank(alice);
        economics.setRevenueSplits(repoId, highRecipients, highBps);

        RepoRegistryV2EconomicModule.Split[] memory stored = economics.revenueSplits(repoId);
        require(stored.length == 1, "invalid update changed split count");
        require(stored[0].recipient == bob && stored[0].bps == 1_000, "invalid update changed split");
    }

    function testOwnerCannotMutateSplitsFromSettlementCallback() public {
        ReenteringEconomicOwner reenteringOwner = new ReenteringEconomicOwner();
        vm.prank(address(reenteringOwner));
        registry.createRepo("callback", "callback owner", "main");
        bytes32 repoId = registry.computeRepoId(address(reenteringOwner), "callback");
        reenteringOwner.configure(IEconomicReentryTarget(address(economics)), repoId);

        vm.prank(carol);
        economics.sponsor{value: 1 ether}(repoId, "settlement lock");

        require(reenteringOwner.attempted(), "owner callback not exercised");
        require(reenteringOwner.blocked(), "split reentry was not blocked");
        require(economics.sponsorTotal(repoId) == 1 ether, "settlement total missing");
        require(address(economics).balance == 0, "module retained funds");
    }

    function testFuzzSponsorConservesValue(uint96 rawAmount, uint16 rawFeeBps, uint16 rawSplitBps) public {
        uint256 amount = uint256(rawAmount) + 1;
        uint16 feeBps = uint16(uint256(rawFeeBps) % 501);
        uint16 splitBps = uint16((uint256(rawSplitBps) % 9_999) + 1);
        RepoRegistryV2EconomicModule fuzzEconomics = new RepoRegistryV2EconomicModule(
            address(registry), address(this), treasury, feeBps
        );
        bytes32 repoId = registry.computeRepoId(alice, "demo");
        address payable[] memory recipients = new address payable[](1);
        recipients[0] = payable(bob);
        uint16[] memory shares = new uint16[](1);
        shares[0] = splitBps;
        vm.prank(alice);
        fuzzEconomics.setRevenueSplits(repoId, recipients, shares);

        vm.deal(carol, amount);
        uint256 aliceBefore = alice.balance;
        uint256 bobBefore = bob.balance;
        uint256 treasuryBefore = treasury.balance;
        vm.prank(carol);
        fuzzEconomics.sponsor{value: amount}(repoId, "fuzz");

        uint256 paid = (alice.balance - aliceBefore) + (bob.balance - bobBefore) +
            (treasury.balance - treasuryBefore);
        require(paid == amount, "value was not conserved");
        require(fuzzEconomics.sponsorTotal(repoId) == amount, "total mismatch");
        require(address(fuzzEconomics).balance == 0, "module retained fuzz funds");
    }
}
