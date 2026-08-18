// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {BadgeModule} from "../src/BadgeModule.sol";
import {BootstrapCoordinator} from "../src/BootstrapCoordinator.sol";
import {EconomicModule} from "../src/EconomicModule.sol";
import {ModerationModule} from "../src/ModerationModule.sol";
import {RecoveryModule} from "../src/RecoveryModule.sol";
import {ReleaseModule} from "../src/ReleaseModule.sol";
import {RepositoryCore} from "../src/RepositoryCore.sol";
import {SuiteDirectory} from "../src/SuiteDirectory.sol";
import {UsernameModule} from "../src/UsernameModule.sol";
import {SuiteIds} from "../src/suite/ISuite.sol";
import {SuiteModule} from "../src/suite/SuiteModule.sol";

struct FuzzSelector {
    address addr;
    bytes4[] selectors;
}

interface SuiteVm {
    function deal(address account, uint256 newBalance) external;
    function prank(address sender) external;
    function expectRevert(bytes calldata revertData) external;
    function expectRevert(bytes4 selector) external;
    function warp(uint256 timestamp) external;
    function deployCode(string calldata artifact, bytes calldata constructorArgs) external returns (address);
}

contract WronglyBoundModule {
    address public immutable suiteDirectory;
    address public immutable bootstrapCoordinator;

    constructor(address directory, address coordinator) {
        suiteDirectory = directory;
        bootstrapCoordinator = coordinator;
    }

    function moduleId() external pure returns (bytes32) {
        return SuiteIds.BADGE;
    }
}

contract RevertingBindingModule {
    function suiteDirectory() external pure returns (address) {
        revert("malicious binding getter");
    }
}

contract ReentrantSponsorReceiver {
    EconomicModule public immutable economic;
    bytes32 public immutable repoId;
    uint256 public attempts;
    bytes4 public rejection;

    constructor(EconomicModule economic_, bytes32 repoId_) {
        economic = economic_;
        repoId = repoId_;
    }

    receive() external payable {
        attempts++;
        try economic.sponsor{value: 1}(repoId, "reenter") {}
        catch (bytes memory reason) {
            if (reason.length >= 4) {
                bytes4 selector;
                assembly {
                    selector := mload(add(reason, 32))
                }
                rejection = selector;
            }
        }
    }
}

abstract contract SuiteArchitectureTestBase {
    SuiteVm internal constant vm = SuiteVm(address(uint160(uint256(keccak256("hevm cheat code")))));
    address internal constant ALICE = address(0xA11CE);
    address internal constant BOB = address(0xB0B);
    bytes32 internal constant SNAPSHOT_ROOT = bytes32(uint256(0x5151));

    SuiteDirectory internal directory;
    BootstrapCoordinator internal coordinator;
    RepositoryCore internal core;
    RecoveryModule internal recovery;
    ModerationModule internal moderation;
    EconomicModule internal economic;
    UsernameModule internal usernames;
    BadgeModule internal badges;
    ReleaseModule internal releases;

    function setUp() public virtual {
        directory = SuiteDirectory(
            vm.deployCode(
                "SuiteDirectory.sol:SuiteDirectory",
                abi.encode(address(this), block.chainid, SNAPSHOT_ROOT)
            )
        );
        coordinator = BootstrapCoordinator(
            vm.deployCode(
                "BootstrapCoordinator.sol:BootstrapCoordinator",
                abi.encode(address(directory), address(this))
            )
        );
        directory.setBootstrapCoordinator(address(coordinator), address(coordinator).codehash);

        core = RepositoryCore(
            vm.deployCode(
                "RepositoryCore.sol:RepositoryCore",
                abi.encode(address(directory), address(coordinator))
            )
        );
        recovery = RecoveryModule(
            vm.deployCode(
                "RecoveryModule.sol:RecoveryModule",
                abi.encode(address(directory), address(coordinator))
            )
        );
        moderation = ModerationModule(
            vm.deployCode(
                "ModerationModule.sol:ModerationModule",
                abi.encode(address(directory), address(coordinator), address(this), address(this))
            )
        );
        economic = EconomicModule(
            vm.deployCode(
                "EconomicModule.sol:EconomicModule",
                abi.encode(
                    address(directory),
                    address(coordinator),
                    address(this),
                    payable(address(this)),
                    uint16(300)
                )
            )
        );
        usernames = UsernameModule(
            vm.deployCode(
                "UsernameModule.sol:UsernameModule",
                abi.encode(address(directory), address(coordinator), address(this))
            )
        );
        badges = BadgeModule(
            vm.deployCode(
                "BadgeModule.sol:BadgeModule",
                abi.encode(address(directory), address(coordinator))
            )
        );
        releases = ReleaseModule(
            vm.deployCode(
                "ReleaseModule.sol:ReleaseModule",
                abi.encode(address(directory), address(coordinator), address(this))
            )
        );

        _register(SuiteIds.CORE, address(core));
        _register(SuiteIds.RECOVERY, address(recovery));
        _register(SuiteIds.MODERATION, address(moderation));
        _register(SuiteIds.ECONOMIC, address(economic));
        _register(SuiteIds.USERNAME, address(usernames));
        _register(SuiteIds.BADGE, address(badges));
        _register(SuiteIds.RELEASE, address(releases));
    }

    receive() external payable {}

    function _register(bytes32 id, address module) internal {
        coordinator.registerModule(id, module, module.codehash);
    }

    function _emptyRoot(bytes32 id) internal pure returns (bytes32) {
        return keccak256(abi.encode(id, SNAPSHOT_ROOT));
    }

    function _finalizeEmpty(bytes32 id) internal {
        coordinator.beginNextModule(id, 0, 0, _emptyRoot(id));
        coordinator.finalizeCurrentModule(id);
    }

    function _rootAfterBatch(bytes32 id, bytes32 current, uint256 sequence, uint256 count, bytes memory payload)
        internal
        pure
        returns (bytes32)
    {
        return keccak256(abi.encode(current, id, sequence, count, keccak256(payload)));
    }

    function _importCoreRepository(bytes32 repoId, address owner) internal {
        RepositoryCore.ImportRepository[] memory records = new RepositoryCore.ImportRepository[](1);
        records[0] = RepositoryCore.ImportRepository(
            repoId,
            owner,
            "imported",
            "historical",
            "main",
            bytes32(0),
            1,
            2
        );
        bytes memory payload = abi.encode(uint8(0), abi.encode(records));
        bytes32 root = _rootAfterBatch(SuiteIds.CORE, _emptyRoot(SuiteIds.CORE), 0, 1, payload);
        coordinator.beginNextModule(SuiteIds.CORE, 1, 1, root);
        coordinator.importBatch(SuiteIds.CORE, 0, 1, keccak256(payload), payload);
        coordinator.finalizeCurrentModule(SuiteIds.CORE);
    }

    function _activateEmptySuite() internal {
        _finalizeEmpty(SuiteIds.CORE);
        _finalizeEmpty(SuiteIds.RECOVERY);
        _finalizeEmpty(SuiteIds.MODERATION);
        _finalizeEmpty(SuiteIds.ECONOMIC);
        coordinator.attestUsernameEscrowReleased(bytes32(uint256(0xE5C0)));
        _finalizeEmpty(SuiteIds.USERNAME);
        _finalizeEmpty(SuiteIds.BADGE);
        _finalizeEmpty(SuiteIds.RELEASE);
        coordinator.activateSuite();
    }
}

contract SuiteArchitectureTest is SuiteArchitectureTestBase {
    function targetContracts() public view returns (address[] memory targets) {
        targets = new address[](1);
        targets[0] = address(this);
    }

    function targetSelectors() public view returns (FuzzSelector[] memory targets) {
        bytes4[] memory selectors = new bytes4[](1);
        selectors[0] = this.advanceInvariantState.selector;
        targets = new FuzzSelector[](1);
        targets[0] = FuzzSelector({addr: address(this), selectors: selectors});
    }

    function advanceInvariantState() external {
        if (coordinator.activated()) return;
        uint256 next = coordinator.nextModuleIndex();
        if (next < SuiteIds.REQUIRED_MODULES) {
            vm.expectRevert(
                abi.encodeWithSelector(
                    BootstrapCoordinator.ModulesIncomplete.selector, next, SuiteIds.REQUIRED_MODULES
                )
            );
            coordinator.activateSuite();
            bytes32 id = directory.requiredModuleAt(next);
            if (id == SuiteIds.USERNAME && coordinator.usernameEscrowEvidenceHash() == bytes32(0)) {
                coordinator.attestUsernameEscrowReleased(bytes32(uint256(0xE5C0)));
            }
            _finalizeEmpty(id);
            return;
        }
        coordinator.activateSuite();
    }

    function testDirectoryActivatesOnlyAfterOrderedModuleFinalization() public {
        require(uint8(directory.state()) == 0, "initial state");
        _activateEmptySuite();
        require(uint8(directory.state()) == 1, "active state");
        require(coordinator.activated(), "coordinator active");
        require(directory.registeredModuleCount() == 7, "module count");
        require(directory.verifyModule(SuiteIds.CORE), "core binding");
        require(directory.verifyModule(SuiteIds.RELEASE), "release binding");
    }

    function testWrongImportOrderAndDoubleFinalizeFailClosed() public {
        vm.expectRevert(
            abi.encodeWithSelector(
                BootstrapCoordinator.ModuleOrderMismatch.selector, SuiteIds.CORE, SuiteIds.RECOVERY
            )
        );
        coordinator.beginNextModule(SuiteIds.RECOVERY, 0, 0, _emptyRoot(SuiteIds.RECOVERY));

        _finalizeEmpty(SuiteIds.CORE);
        vm.expectRevert(
            abi.encodeWithSelector(
                BootstrapCoordinator.ModuleOrderMismatch.selector, SuiteIds.RECOVERY, SuiteIds.CORE
            )
        );
        coordinator.finalizeCurrentModule(SuiteIds.CORE);

        vm.expectRevert(SuiteModule.BootstrapAlreadyFinalized.selector);
        vm.prank(address(coordinator));
        core.finalizeBootstrap();
    }

    function testUsernameEscrowEvidenceIsAnActivationGate() public {
        _finalizeEmpty(SuiteIds.CORE);
        _finalizeEmpty(SuiteIds.RECOVERY);
        _finalizeEmpty(SuiteIds.MODERATION);
        _finalizeEmpty(SuiteIds.ECONOMIC);
        coordinator.beginNextModule(SuiteIds.USERNAME, 0, 0, _emptyRoot(SuiteIds.USERNAME));
        vm.expectRevert(BootstrapCoordinator.UsernameEscrowEvidenceRequired.selector);
        coordinator.finalizeCurrentModule(SuiteIds.USERNAME);
    }

    function testDirectoryRejectsWrongRuntimeHashAndWrongBinding() public {
        SuiteDirectory other = SuiteDirectory(
            vm.deployCode(
                "SuiteDirectory.sol:SuiteDirectory",
                abi.encode(address(this), block.chainid, bytes32(uint256(0x999)))
            )
        );
        BootstrapCoordinator otherCoordinator = BootstrapCoordinator(
            vm.deployCode(
                "BootstrapCoordinator.sol:BootstrapCoordinator",
                abi.encode(address(other), address(this))
            )
        );
        other.setBootstrapCoordinator(address(otherCoordinator), address(otherCoordinator).codehash);
        WronglyBoundModule wrong = new WronglyBoundModule(address(directory), address(coordinator));

        vm.expectRevert(
            abi.encodeWithSelector(
                SuiteDirectory.CodeHashMismatch.selector, bytes32(uint256(1)), address(wrong).codehash
            )
        );
        otherCoordinator.registerModule(SuiteIds.BADGE, address(wrong), bytes32(uint256(1)));

        vm.expectRevert(
            abi.encodeWithSelector(SuiteDirectory.ModuleBindingMismatch.selector, SuiteIds.BADGE, address(wrong))
        );
        otherCoordinator.registerModule(SuiteIds.BADGE, address(wrong), address(wrong).codehash);

        RevertingBindingModule malicious = new RevertingBindingModule();
        vm.expectRevert(abi.encodeWithSignature("Error(string)", "malicious binding getter"));
        otherCoordinator.registerModule(SuiteIds.BADGE, address(malicious), address(malicious).codehash);
    }

    function testAbandonedSuiteNeverServesRuntimeWrites() public {
        vm.expectRevert(SuiteModule.SuiteNotActive.selector);
        vm.prank(ALICE);
        core.createRepository("demo", "", "main");
        require(uint8(directory.state()) == 0, "suite remains bootstrapping");
    }

    function testModerationIsARequiredCoreAndEconomicPolicyHook() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("demo", "", "main");
        moderation.setRepositoryStatus(repoId, ModerationModule.RepoStatus.Frozen, "sha256:decision");

        string[] memory uris = new string[](1);
        uris[0] = "ipfs://bafy-policy";
        vm.expectRevert(abi.encodeWithSelector(ModerationModule.RepositoryFrozen.selector, repoId));
        vm.prank(ALICE);
        core.updateRef(
            repoId,
            "refs/heads/main",
            "0123456789abcdef0123456789abcdef01234567",
            uris,
            "",
            false
        );

        vm.expectRevert(abi.encodeWithSelector(ModerationModule.RepositoryFrozen.selector, repoId));
        vm.deal(BOB, 1);
        vm.prank(BOB);
        economic.sponsor{value: 1}(repoId, "blocked");

        (ModerationModule.StatusTrailEntry[] memory trail, uint256 nextCursor) =
            moderation.listRepositoryStatusTrailPage(repoId, 0, 64);
        require(trail.length == 1 && nextCursor == 1, "status trail length");
        require(trail[0].status == ModerationModule.RepoStatus.Frozen, "status trail state");
        require(trail[0].actor == address(this) && trail[0].reportId == 0, "status trail provenance");
    }

    function testModerationImportBindsHistoryToFinalStatus() public {
        bytes32 repoId = bytes32(uint256(0xA11CE));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);

        ModerationModule.ImportStatus[] memory statuses = new ModerationModule.ImportStatus[](1);
        statuses[0] = ModerationModule.ImportStatus(repoId, ModerationModule.RepoStatus.Frozen);
        bytes memory statusPayload = abi.encode(uint8(0), abi.encode(statuses));

        ModerationModule.StatusTrailEntry[] memory entries = new ModerationModule.StatusTrailEntry[](1);
        entries[0] = ModerationModule.StatusTrailEntry(
            ModerationModule.RepoStatus.Frozen,
            address(this),
            "sha256:historical-decision",
            2,
            0
        );
        ModerationModule.ImportStatusTrail[] memory trails = new ModerationModule.ImportStatusTrail[](1);
        trails[0] = ModerationModule.ImportStatusTrail(repoId, entries);
        bytes memory trailPayload = abi.encode(uint8(2), abi.encode(trails));

        bytes32 rolling = _rootAfterBatch(
            SuiteIds.MODERATION, _emptyRoot(SuiteIds.MODERATION), 0, 1, statusPayload
        );
        rolling = _rootAfterBatch(SuiteIds.MODERATION, rolling, 1, 1, trailPayload);
        coordinator.beginNextModule(SuiteIds.MODERATION, 2, 2, rolling);
        coordinator.importBatch(SuiteIds.MODERATION, 0, 1, keccak256(statusPayload), statusPayload);
        coordinator.importBatch(SuiteIds.MODERATION, 1, 1, keccak256(trailPayload), trailPayload);
        coordinator.finalizeCurrentModule(SuiteIds.MODERATION);

        (ModerationModule.StatusTrailEntry[] memory imported,) =
            moderation.listRepositoryStatusTrailPage(repoId, 0, 64);
        require(imported.length == 1, "imported status trail length");
        require(imported[0].status == ModerationModule.RepoStatus.Frozen, "imported final status");
    }

    function testModerationImportRejectsTrailWhoseFinalStatusDiffers() public {
        bytes32 repoId = bytes32(uint256(0xB0B));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);

        ModerationModule.ImportStatus[] memory statuses = new ModerationModule.ImportStatus[](1);
        statuses[0] = ModerationModule.ImportStatus(repoId, ModerationModule.RepoStatus.Frozen);
        bytes memory statusPayload = abi.encode(uint8(0), abi.encode(statuses));
        ModerationModule.StatusTrailEntry[] memory entries = new ModerationModule.StatusTrailEntry[](1);
        entries[0] = ModerationModule.StatusTrailEntry(
            ModerationModule.RepoStatus.Active,
            address(this),
            "sha256:stale-decision",
            2,
            0
        );
        ModerationModule.ImportStatusTrail[] memory trails = new ModerationModule.ImportStatusTrail[](1);
        trails[0] = ModerationModule.ImportStatusTrail(repoId, entries);
        bytes memory trailPayload = abi.encode(uint8(2), abi.encode(trails));
        bytes32 rolling = _rootAfterBatch(
            SuiteIds.MODERATION, _emptyRoot(SuiteIds.MODERATION), 0, 1, statusPayload
        );
        rolling = _rootAfterBatch(SuiteIds.MODERATION, rolling, 1, 1, trailPayload);
        coordinator.beginNextModule(SuiteIds.MODERATION, 2, 2, rolling);
        coordinator.importBatch(SuiteIds.MODERATION, 0, 1, keccak256(statusPayload), statusPayload);
        vm.expectRevert(ModerationModule.InvalidImportRecord.selector);
        coordinator.importBatch(SuiteIds.MODERATION, 1, 1, keccak256(trailPayload), trailPayload);
    }

    function testOnlyRecoveryCapabilityCanRecoverOwnership() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("recoverable", "", "main");
        vm.expectRevert(abi.encodeWithSelector(RepositoryCore.Unauthorized.selector, address(this)));
        core.recoverOwnership(repoId, BOB);
    }

    function testUsernameClaimsCarryNoEscrowLiability() public {
        _activateEmptySuite();
        require(address(usernames).balance == 0, "no migrated escrow");
        vm.prank(ALICE);
        usernames.registerUsername("alice");
        require(address(usernames).balance == 0, "no registration escrow");
        require(usernames.resolveUsername("alice").owner == ALICE, "forward lookup");
        require(keccak256(bytes(usernames.usernameOf(ALICE))) == keccak256(bytes("alice")), "reverse lookup");
    }

    function invariantDirectoryNeverActiveBeforeAllModulesFinalize() public view {
        if (uint8(directory.state()) == 1) {
            require(coordinator.nextModuleIndex() == 7, "all modules finalized");
            require(coordinator.usernameEscrowEvidenceHash() != bytes32(0), "escrow evidence");
            for (uint256 i; i < 7; ++i) {
                bytes32 id = directory.requiredModuleAt(i);
                require(directory.verifyModule(id), "module binding");
                require(SuiteModule(directory.moduleAddress(id)).bootstrapFinalized(), "module finalized");
            }
        }
    }

    function testRepresentativeCoreWriteGasCeiling() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        uint256 beforeGas = gasleft();
        core.createRepository("gas-demo", "representative", "main");
        require(beforeGas - gasleft() < 700_000, "create repository gas ceiling");
    }
}

contract SuiteArchitectureSecurityTest is SuiteArchitectureTestBase {
    function testBootstrapAuthorityAndImportRuntimeLimitsFailClosed() public {
        vm.expectRevert(abi.encodeWithSelector(BootstrapCoordinator.Unauthorized.selector, ALICE));
        vm.prank(ALICE);
        coordinator.beginNextModule(SuiteIds.CORE, 0, 0, _emptyRoot(SuiteIds.CORE));

        vm.expectRevert(abi.encodeWithSelector(SuiteModule.UnauthorizedBootstrap.selector, ALICE));
        vm.prank(ALICE);
        core.beginBootstrap(0, 0, _emptyRoot(SuiteIds.CORE), SNAPSHOT_ROOT);

        coordinator.beginNextModule(SuiteIds.CORE, 0, 0, _emptyRoot(SuiteIds.CORE));
        bytes memory payload = hex"01";
        uint256 maxBatchItems = core.MAX_IMPORT_BATCH_ITEMS();
        vm.expectRevert(
            abi.encodeWithSelector(
                SuiteModule.ImportBatchTooLarge.selector,
                maxBatchItems + 1,
                maxBatchItems
            )
        );
        vm.prank(address(coordinator));
        core.bootstrapImport(0, maxBatchItems + 1, keccak256(payload), payload);

        payload = new bytes(core.MAX_IMPORT_PAYLOAD_BYTES() + 1);
        vm.expectRevert(
            abi.encodeWithSelector(
                SuiteModule.ImportPayloadTooLarge.selector,
                payload.length,
                core.MAX_IMPORT_PAYLOAD_BYTES()
            )
        );
        vm.prank(address(coordinator));
        core.bootstrapImport(0, 1, keccak256(payload), payload);
    }

    function testEconomicSettlementRejectsRecipientReentrancy() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("reentrancy", "", "main");
        ReentrantSponsorReceiver receiver = new ReentrantSponsorReceiver(economic, repoId);
        address payable[] memory recipients = new address payable[](1);
        recipients[0] = payable(address(receiver));
        uint16[] memory bps = new uint16[](1);
        bps[0] = 10_000;
        vm.prank(ALICE);
        economic.setRevenueSplits(repoId, recipients, bps);

        vm.deal(BOB, 2 ether);
        vm.prank(BOB);
        economic.sponsor{value: 1 ether}(repoId, "outer");

        require(receiver.attempts() == 1, "one reentry attempt");
        require(receiver.rejection() == EconomicModule.ReentrantSettlement.selector, "reentry rejected");
        require(economic.sponsorTotal(repoId, "inj") == 1 ether, "outer settlement counted once");
    }

    function testRuntimeModuleAuthoritiesRejectUnprivilegedCallers() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("permissions", "", "main");

        vm.expectRevert(abi.encodeWithSelector(RepositoryCore.Unauthorized.selector, BOB));
        vm.prank(BOB);
        core.setCollaborator(repoId, address(0xCAFE), RepositoryCore.Role.Reader);

        vm.expectRevert(abi.encodeWithSelector(ModerationModule.Unauthorized.selector, ALICE));
        vm.prank(ALICE);
        moderation.setCommittee(ALICE);

        vm.expectRevert(abi.encodeWithSelector(EconomicModule.Unauthorized.selector, ALICE));
        vm.prank(ALICE);
        economic.setFeeConfig(payable(ALICE), 100);

        vm.expectRevert(abi.encodeWithSelector(UsernameModule.Unauthorized.selector, ALICE));
        vm.prank(ALICE);
        usernames.setReserved("reserved", true);

        vm.expectRevert(abi.encodeWithSelector(BadgeModule.Unauthorized.selector, BOB));
        vm.prank(BOB);
        badges.awardBadge(repoId, BOB, "unauthorized");

        vm.expectRevert(abi.encodeWithSelector(ReleaseModule.Unauthorized.selector, ALICE));
        vm.prank(ALICE);
        releases.registerArtifact("v1.0.0", "linux-amd64", bytes32(uint256(1)));
    }

}

contract SuiteProtocolParityTest is SuiteArchitectureTestBase {
    address internal constant CAROL = address(0xCA501);
    address internal constant DAVE = address(0xDA7E);

    function testOwnershipTransferAndRecoveryAreMutuallyExclusive() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("mutex", "", "main");

        address[] memory guardians = new address[](1);
        guardians[0] = BOB;
        vm.prank(ALICE);
        recovery.setGuardians(repoId, guardians, 1);

        vm.prank(ALICE);
        core.beginOwnershipTransfer(repoId, CAROL);
        RepositoryCore.PendingOwnershipTransfer memory transfer = core.pendingOwnershipTransfer(repoId);
        require(transfer.newOwner == CAROL, "transfer pending");
        vm.expectRevert(abi.encodeWithSelector(RecoveryModule.OwnershipTransferPending.selector, repoId));
        vm.prank(BOB);
        recovery.proposeRecovery(repoId, DAVE);

        vm.prank(ALICE);
        core.cancelOwnershipTransfer(repoId);
        vm.prank(BOB);
        recovery.proposeRecovery(repoId, DAVE);
        require(recovery.hasPendingRecovery(repoId), "recovery pending");
        vm.expectRevert(abi.encodeWithSelector(RepositoryCore.RecoveryPending.selector, repoId));
        vm.prank(ALICE);
        core.beginOwnershipTransfer(repoId, CAROL);
    }

    function testOwnershipTransferPreservesTimestampAndClearsRecoveryState() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("ownership-state", "", "main");
        RepositoryCore.Repository memory beforeTransfer = core.getRepository(repoId);

        address[] memory guardians = new address[](1);
        guardians[0] = BOB;
        vm.prank(ALICE);
        recovery.setGuardians(repoId, guardians, 1);

        vm.prank(ALICE);
        core.beginOwnershipTransfer(repoId, CAROL);
        RepositoryCore.PendingOwnershipTransfer memory pending = core.pendingOwnershipTransfer(repoId);
        vm.warp(pending.executeAfter);
        vm.prank(CAROL);
        core.acceptOwnershipTransfer(repoId);

        RepositoryCore.Repository memory afterTransfer = core.getRepository(repoId);
        require(afterTransfer.owner == CAROL, "owner moved");
        require(afterTransfer.updatedAt == beforeTransfer.updatedAt, "ownership move preserves timestamp");
        RecoveryModule.GuardianConfig memory config = recovery.guardianConfig(repoId);
        require(config.threshold == 0 && config.guardians.length == 0, "recovery state cleared");
        vm.expectRevert(abi.encodeWithSelector(RecoveryModule.InvalidGuardianThreshold.selector, 0, 0));
        vm.prank(BOB);
        recovery.proposeRecovery(repoId, DAVE);
    }

    function testModerationActionsAdvanceRepositoryTimestamp() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("moderation-timestamp", "", "main");
        uint64 before = core.getRepository(repoId).updatedAt;

        vm.warp(uint256(before) + 1);
        moderation.setRepositoryStatus(repoId, ModerationModule.RepoStatus.Frozen, "sha256:frozen");
        uint64 afterStatus = core.getRepository(repoId).updatedAt;
        require(afterStatus == block.timestamp && afterStatus > before, "status updates repository timestamp");

        vm.prank(BOB);
        uint256 reportId = moderation.submitReport(repoId, "sha256:report");
        vm.warp(uint256(afterStatus) + 1);
        moderation.resolveReport(reportId, ModerationModule.RepoStatus.Frozen, "sha256:decision");
        uint64 afterResolution = core.getRepository(repoId).updatedAt;
        require(afterResolution == block.timestamp && afterResolution > afterStatus, "resolution timestamp");

        vm.warp(uint256(afterResolution) + 1);
        vm.prank(ALICE);
        moderation.appealReport(reportId, "sha256:appeal");
        uint64 afterAppeal = core.getRepository(repoId).updatedAt;
        require(afterAppeal == block.timestamp && afterAppeal > afterResolution, "appeal timestamp");

        vm.warp(uint256(afterAppeal) + 1);
        moderation.resolveAppeal(reportId, ModerationModule.RepoStatus.Active, "sha256:appeal-decision");
        uint64 afterAppealResolution = core.getRepository(repoId).updatedAt;
        require(
            afterAppealResolution == block.timestamp && afterAppealResolution > afterAppeal,
            "appeal resolution timestamp"
        );
    }

    function testBadgeAndForkRequireActiveWhileDelistedRefsAndSponsorshipRemainAvailable() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("policy", "", "main");

        vm.expectRevert(abi.encodeWithSelector(BadgeModule.OwnerCannotReceiveBadge.selector, ALICE));
        vm.prank(ALICE);
        badges.awardBadge(repoId, ALICE, "self award");

        moderation.setRepositoryStatus(repoId, ModerationModule.RepoStatus.Frozen, "sha256:frozen");
        vm.expectRevert(
            abi.encodeWithSelector(
                ModerationModule.RepositoryNotActive.selector,
                repoId,
                ModerationModule.RepoStatus.Frozen
            )
        );
        vm.prank(ALICE);
        badges.awardBadge(repoId, BOB, "blocked while frozen");

        moderation.setRepositoryStatus(repoId, ModerationModule.RepoStatus.Delisted, "sha256:delisted");
        vm.expectRevert(
            abi.encodeWithSelector(
                ModerationModule.RepositoryNotActive.selector,
                repoId,
                ModerationModule.RepoStatus.Delisted
            )
        );
        vm.prank(BOB);
        core.forkRepository(repoId, "blocked-fork");
        vm.expectRevert(
            abi.encodeWithSelector(
                ModerationModule.RepositoryNotActive.selector,
                repoId,
                ModerationModule.RepoStatus.Delisted
            )
        );
        vm.prank(ALICE);
        badges.awardBadge(repoId, BOB, "blocked while delisted");

        string[] memory uris = _singleUri();
        vm.prank(ALICE);
        core.updateRef(
            repoId,
            "refs/heads/main",
            "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
            uris,
            "",
            false
        );
        vm.deal(BOB, 100);
        vm.prank(BOB);
        economic.sponsor{value: 100}(repoId, "delisted but payable");
        require(economic.sponsorTotal(repoId, "inj") == 100, "delisted sponsor total");
    }

    function testRepositoryAndRefValidationMatchesV1ArchiveRules() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("Repo.Release-1", "", "main");
        string memory uppercaseSha = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";

        vm.prank(ALICE);
        core.updateRef(repoId, "refs/heads/Feature@1", uppercaseSha, _singleUri(), "", false);
        RepositoryCore.GitRef memory gitRef = core.getRef(repoId, "refs/heads/Feature@1");
        require(keccak256(bytes(gitRef.commitSha)) == keccak256(bytes(uppercaseSha)), "uppercase SHA retained");

        vm.expectRevert(
            abi.encodeWithSelector(RepositoryCore.InvalidRefName.selector, "refs/heads/../main")
        );
        vm.prank(ALICE);
        core.updateRef(repoId, "refs/heads/../main", uppercaseSha, _singleUri(), "", false);

        vm.expectRevert(
            abi.encodeWithSelector(RepositoryCore.InvalidRefName.selector, "refs/heads/main~1")
        );
        vm.prank(ALICE);
        core.updateRef(repoId, "refs/heads/main~1", uppercaseSha, _singleUri(), "", false);
    }

    function testUsernameRejectsAddressLikeNamesAndReservedOriginalClaims() public {
        _activateEmptySuite();
        vm.expectRevert(abi.encodeWithSelector(UsernameModule.InvalidUsername.selector, "inj1alice"));
        vm.prank(ALICE);
        usernames.registerUsername("inj1alice");
    }

    function testReservedUsernameCountMatchesV1Limit() public {
        _activateEmptySuite();
        for (uint256 i; i < usernames.MAX_RESERVED_USERNAMES(); ++i) {
            usernames.setReserved(_reservedTestName(i), true);
        }
        require(usernames.reservedUsernameCount() == 128, "128 reservations accepted");

        vm.expectRevert(
            abi.encodeWithSelector(UsernameModule.TooManyReservedUsernames.selector, 129, 128)
        );
        usernames.setReserved(_reservedTestName(128), true);
    }

    function testOriginalUsernameClaimCannotBypassReservation() public {
        _finalizeEmpty(SuiteIds.CORE);
        _finalizeEmpty(SuiteIds.RECOVERY);
        _finalizeEmpty(SuiteIds.MODERATION);
        _finalizeEmpty(SuiteIds.ECONOMIC);

        UsernameModule.ImportOriginalOwner[] memory owners = new UsernameModule.ImportOriginalOwner[](1);
        owners[0] = UsernameModule.ImportOriginalOwner("alice", ALICE);
        bytes memory ownerPayload = abi.encode(uint8(0), abi.encode(owners));
        string[] memory reserved = new string[](1);
        reserved[0] = "alice";
        bytes memory reservedPayload = abi.encode(uint8(1), abi.encode(reserved));
        bytes32 rolling = _rootAfterBatch(
            SuiteIds.USERNAME, _emptyRoot(SuiteIds.USERNAME), 0, 1, ownerPayload
        );
        rolling = _rootAfterBatch(SuiteIds.USERNAME, rolling, 1, 1, reservedPayload);
        coordinator.beginNextModule(SuiteIds.USERNAME, 2, 2, rolling);
        coordinator.importBatch(SuiteIds.USERNAME, 0, 1, keccak256(ownerPayload), ownerPayload);
        coordinator.importBatch(SuiteIds.USERNAME, 1, 1, keccak256(reservedPayload), reservedPayload);
        coordinator.attestUsernameEscrowReleased(bytes32(uint256(0xE5C0)));
        coordinator.finalizeCurrentModule(SuiteIds.USERNAME);
        _finalizeEmpty(SuiteIds.BADGE);
        _finalizeEmpty(SuiteIds.RELEASE);
        coordinator.activateSuite();

        vm.expectRevert(abi.encodeWithSelector(UsernameModule.UsernameReserved.selector, "alice"));
        vm.prank(ALICE);
        usernames.claimOriginalUsername("alice");
    }

    function testRevenueSplitRejectsOwnerAndClearsOnOwnershipTransfer() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("split-owner", "", "main");
        address payable[] memory recipients = new address payable[](1);
        recipients[0] = payable(ALICE);
        uint16[] memory bps = new uint16[](1);
        bps[0] = 1_000;

        vm.expectRevert(
            abi.encodeWithSelector(EconomicModule.OwnerCannotReceiveRevenueSplit.selector, ALICE)
        );
        vm.prank(ALICE);
        economic.setRevenueSplits(repoId, recipients, bps);

        recipients[0] = payable(CAROL);
        vm.prank(ALICE);
        economic.setRevenueSplits(repoId, recipients, bps);
        vm.prank(ALICE);
        core.beginOwnershipTransfer(repoId, BOB);
        RepositoryCore.PendingOwnershipTransfer memory pending = core.pendingOwnershipTransfer(repoId);
        vm.warp(pending.executeAfter);
        vm.prank(BOB);
        core.acceptOwnershipTransfer(repoId);
        require(economic.revenueSplits(repoId).length == 0, "splits cleared on owner change");
    }

    function testRevenueSplitMatchesV1TwentyRecipientLimit() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("split-limit", "", "main");

        address payable[] memory recipients = new address payable[](20);
        uint16[] memory bps = new uint16[](20);
        for (uint256 i; i < recipients.length; ++i) {
            recipients[i] = payable(address(uint160(0x9000 + i)));
            bps[i] = 1;
        }
        vm.prank(ALICE);
        economic.setRevenueSplits(repoId, recipients, bps);
        require(economic.revenueSplits(repoId).length == 20, "twenty recipients accepted");

        recipients = new address payable[](21);
        bps = new uint16[](21);
        for (uint256 i; i < recipients.length; ++i) {
            recipients[i] = payable(address(uint160(0xA000 + i)));
            bps[i] = 1;
        }
        vm.expectRevert(
            abi.encodeWithSelector(EconomicModule.TooManySplitRecipients.selector, 21, 20)
        );
        vm.prank(ALICE);
        economic.setRevenueSplits(repoId, recipients, bps);
    }

    function testModerationPreservesSubmissionResolutionAndAppealCommitments() public {
        _activateEmptySuite();
        vm.prank(ALICE);
        bytes32 repoId = core.createRepository("report-fields", "", "main");
        vm.prank(BOB);
        uint256 reportId = moderation.submitReport(repoId, "sha256:submission");
        moderation.resolveReport(reportId, ModerationModule.RepoStatus.Frozen, "sha256:decision");
        vm.prank(ALICE);
        moderation.appealReport(reportId, "sha256:appeal");
        moderation.resolveAppeal(reportId, ModerationModule.RepoStatus.Active, "sha256:appeal-decision");

        ModerationModule.Report memory report = moderation.getReport(reportId);
        require(_same(report.reasonHash, "sha256:submission"), "submission reason preserved");
        (string memory submission, string memory resolution, string memory appeal) =
            moderation.reportCommitments(reportId);
        require(_same(submission, "sha256:submission"), "submission commitment");
        require(_same(resolution, "sha256:appeal-decision"), "latest resolution commitment");
        require(_same(appeal, "sha256:appeal"), "appeal commitment");
    }

    function _singleUri() private pure returns (string[] memory uris) {
        uris = new string[](1);
        uris[0] = "ipfs://bafy-parity";
    }

    function _same(string memory left, string memory right) private pure returns (bool) {
        return keccak256(bytes(left)) == keccak256(bytes(right));
    }

    function _reservedTestName(uint256 index) private pure returns (string memory) {
        bytes memory raw = new bytes(5);
        raw[0] = "r";
        raw[1] = bytes1(uint8(97 + (index / 676) % 26));
        raw[2] = bytes1(uint8(97 + (index / 26) % 26));
        raw[3] = bytes1(uint8(97 + index % 26));
        raw[4] = "x";
        return string(raw);
    }
}

contract SuiteImportValidationTest is SuiteArchitectureTestBase {
    function testEconomicImportAllowsOneEmptySplitRecord() public {
        bytes32 repoId = bytes32(uint256(0xEC00));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);
        _finalizeEmpty(SuiteIds.MODERATION);

        EconomicModule.Split[] memory splits = new EconomicModule.Split[](0);
        EconomicModule.ImportSplits[] memory records = new EconomicModule.ImportSplits[](1);
        records[0] = EconomicModule.ImportSplits(repoId, splits);
        bytes memory payload = abi.encode(uint8(0), abi.encode(records));
        bytes32 root = _rootAfterBatch(SuiteIds.ECONOMIC, _emptyRoot(SuiteIds.ECONOMIC), 0, 1, payload);
        root = _rootAfterBatch(SuiteIds.ECONOMIC, root, 1, 1, payload);
        coordinator.beginNextModule(SuiteIds.ECONOMIC, 2, 2, root);
        coordinator.importBatch(SuiteIds.ECONOMIC, 0, 1, keccak256(payload), payload);
        require(economic.revenueSplits(repoId).length == 0, "empty split record clears splits");
        vm.expectRevert(EconomicModule.InvalidImportRecord.selector);
        coordinator.importBatch(SuiteIds.ECONOMIC, 1, 1, keccak256(payload), payload);
    }

    function testModerationImportRejectsStatusTrailBeforeFinalStatus() public {
        bytes32 repoId = bytes32(uint256(0xB0A));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);

        ModerationModule.StatusTrailEntry[] memory entries = new ModerationModule.StatusTrailEntry[](1);
        entries[0] = ModerationModule.StatusTrailEntry(
            ModerationModule.RepoStatus.Active, address(this), "", 2, 0
        );
        ModerationModule.ImportStatusTrail[] memory trails = new ModerationModule.ImportStatusTrail[](1);
        trails[0] = ModerationModule.ImportStatusTrail(repoId, entries);
        bytes memory payload = abi.encode(uint8(2), abi.encode(trails));
        bytes32 root = _rootAfterBatch(SuiteIds.MODERATION, _emptyRoot(SuiteIds.MODERATION), 0, 1, payload);
        coordinator.beginNextModule(SuiteIds.MODERATION, 1, 1, root);
        vm.expectRevert(ModerationModule.InvalidImportRecord.selector);
        coordinator.importBatch(SuiteIds.MODERATION, 0, 1, keccak256(payload), payload);
    }

    function testCoreImportRejectsUnknownForkParent() public {
        RepositoryCore.ImportRepository[] memory repositories = new RepositoryCore.ImportRepository[](1);
        repositories[0] = RepositoryCore.ImportRepository(
            bytes32(uint256(0xF001)),
            ALICE,
            "Child",
            "historical",
            "main",
            bytes32(uint256(0xF000)),
            1,
            2
        );
        bytes memory payload = abi.encode(uint8(0), abi.encode(repositories));
        bytes32 root = _rootAfterBatch(SuiteIds.CORE, _emptyRoot(SuiteIds.CORE), 0, 1, payload);
        coordinator.beginNextModule(SuiteIds.CORE, 1, 1, root);
        vm.expectRevert(RepositoryCore.InvalidImportRecord.selector);
        coordinator.importBatch(SuiteIds.CORE, 0, 1, keccak256(payload), payload);
    }

    function testCoreImportRejectsRefWithoutProvenance() public {
        bytes32 repoId = bytes32(uint256(0xC0FE));
        RepositoryCore.ImportRepository[] memory repositories = new RepositoryCore.ImportRepository[](1);
        repositories[0] = RepositoryCore.ImportRepository(
            repoId, ALICE, "Imported.Repo", "historical", "main", bytes32(0), 1, 2
        );
        bytes memory repositoryPayload = abi.encode(uint8(0), abi.encode(repositories));
        RepositoryCore.ImportRef[] memory refs = new RepositoryCore.ImportRef[](1);
        string[] memory uris = new string[](1);
        uris[0] = "ipfs://bafy-import";
        refs[0] = RepositoryCore.ImportRef(
            repoId,
            "refs/heads/main",
            "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
            uris,
            0,
            address(0)
        );
        bytes memory refPayload = abi.encode(uint8(2), abi.encode(refs));
        bytes32 rolling = _rootAfterBatch(
            SuiteIds.CORE, _emptyRoot(SuiteIds.CORE), 0, 1, repositoryPayload
        );
        rolling = _rootAfterBatch(SuiteIds.CORE, rolling, 1, 1, refPayload);
        coordinator.beginNextModule(SuiteIds.CORE, 2, 2, rolling);
        coordinator.importBatch(SuiteIds.CORE, 0, 1, keccak256(repositoryPayload), repositoryPayload);
        vm.expectRevert(RepositoryCore.InvalidImportRecord.selector);
        coordinator.importBatch(SuiteIds.CORE, 1, 1, keccak256(refPayload), refPayload);
    }

    function testEconomicImportRejectsOwnerRecipient() public {
        bytes32 repoId = bytes32(uint256(0xEC01));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);
        _finalizeEmpty(SuiteIds.MODERATION);

        EconomicModule.Split[] memory splits = new EconomicModule.Split[](1);
        splits[0] = EconomicModule.Split(payable(ALICE), 1_000);
        EconomicModule.ImportSplits[] memory records = new EconomicModule.ImportSplits[](1);
        records[0] = EconomicModule.ImportSplits(repoId, splits);
        bytes memory payload = abi.encode(uint8(0), abi.encode(records));
        bytes32 root = _rootAfterBatch(
            SuiteIds.ECONOMIC, _emptyRoot(SuiteIds.ECONOMIC), 0, 1, payload
        );
        coordinator.beginNextModule(SuiteIds.ECONOMIC, 1, 1, root);
        vm.expectRevert(EconomicModule.InvalidImportRecord.selector);
        coordinator.importBatch(SuiteIds.ECONOMIC, 0, 1, keccak256(payload), payload);
    }

    function testBadgeImportRejectsHistoricalSelfAward() public {
        bytes32 repoId = bytes32(uint256(0xBAD6E));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);
        _finalizeEmpty(SuiteIds.MODERATION);
        _finalizeEmpty(SuiteIds.ECONOMIC);
        coordinator.attestUsernameEscrowReleased(bytes32(uint256(0xE5C0)));
        _finalizeEmpty(SuiteIds.USERNAME);

        BadgeModule.Badge[] memory records = new BadgeModule.Badge[](1);
        records[0] = BadgeModule.Badge(1, repoId, ALICE, ALICE, "self award", 2, true);
        bytes memory payload = abi.encode(records);
        bytes32 root = _rootAfterBatch(SuiteIds.BADGE, _emptyRoot(SuiteIds.BADGE), 0, 1, payload);
        coordinator.beginNextModule(SuiteIds.BADGE, 1, 1, root);
        vm.expectRevert(BadgeModule.InvalidImportRecord.selector);
        coordinator.importBatch(SuiteIds.BADGE, 0, 1, keccak256(payload), payload);
    }

    function testModerationImportRestoresOriginalReasonFromTrail() public {
        bytes32 repoId = bytes32(uint256(0xA11D17));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);

        ModerationModule.TrailEntry[] memory trail = new ModerationModule.TrailEntry[](2);
        trail[0] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Submitted,
            BOB,
            ModerationModule.RepoStatus.Active,
            "sha256:original",
            1
        );
        trail[1] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Resolved,
            address(this),
            ModerationModule.RepoStatus.Frozen,
            "sha256:decision",
            2
        );
        ModerationModule.ImportReport[] memory records = new ModerationModule.ImportReport[](1);
        records[0] = ModerationModule.ImportReport(
            ModerationModule.Report(
                7,
                repoId,
                BOB,
                ModerationModule.ReportStatus.Resolved,
                ModerationModule.RepoStatus.Frozen,
                "sha256:original",
                1,
                2,
                true
            ),
            trail
        );
        bytes memory payload = abi.encode(uint8(1), abi.encode(records));
        bytes32 root = _rootAfterBatch(
            SuiteIds.MODERATION, _emptyRoot(SuiteIds.MODERATION), 0, 1, payload
        );
        coordinator.beginNextModule(SuiteIds.MODERATION, 1, 1, root);
        coordinator.importBatch(SuiteIds.MODERATION, 0, 1, keccak256(payload), payload);
        coordinator.finalizeCurrentModule(SuiteIds.MODERATION);

        ModerationModule.Report memory report = moderation.getReport(7);
        require(keccak256(bytes(report.reasonHash)) == keccak256(bytes("sha256:original")), "original reason");
        (, string memory resolution,) = moderation.reportCommitments(7);
        require(keccak256(bytes(resolution)) == keccak256(bytes("sha256:decision")), "resolution reason");
    }

    function testModerationImportRejectsMismatchedSubmissionReason() public {
        bytes32 repoId = bytes32(uint256(0xA11D19));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);

        ModerationModule.TrailEntry[] memory trail = new ModerationModule.TrailEntry[](2);
        trail[0] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Submitted,
            BOB,
            ModerationModule.RepoStatus.Active,
            "sha256:original",
            1
        );
        trail[1] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Resolved,
            address(this),
            ModerationModule.RepoStatus.Frozen,
            "sha256:decision",
            2
        );
        ModerationModule.ImportReport[] memory records = new ModerationModule.ImportReport[](1);
        records[0] = ModerationModule.ImportReport(
            ModerationModule.Report(
                9,
                repoId,
                BOB,
                ModerationModule.ReportStatus.Resolved,
                ModerationModule.RepoStatus.Frozen,
                "sha256:decision",
                1,
                2,
                true
            ),
            trail
        );
        bytes memory payload = abi.encode(uint8(1), abi.encode(records));
        bytes32 root = _rootAfterBatch(
            SuiteIds.MODERATION, _emptyRoot(SuiteIds.MODERATION), 0, 1, payload
        );
        coordinator.beginNextModule(SuiteIds.MODERATION, 1, 1, root);
        vm.expectRevert(ModerationModule.InvalidImportRecord.selector);
        coordinator.importBatch(SuiteIds.MODERATION, 0, 1, keccak256(payload), payload);
    }

    function testModerationImportRejectsInvalidReportStateTrail() public {
        bytes32 repoId = bytes32(uint256(0xA11D1A));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);

        ModerationModule.TrailEntry[] memory trail = new ModerationModule.TrailEntry[](3);
        trail[0] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Submitted,
            BOB,
            ModerationModule.RepoStatus.Active,
            "sha256:original",
            1
        );
        trail[1] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Resolved,
            address(this),
            ModerationModule.RepoStatus.Frozen,
            "sha256:decision",
            2
        );
        trail[2] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Appealed,
            ALICE,
            ModerationModule.RepoStatus.Active,
            "sha256:appeal",
            3
        );
        ModerationModule.ImportReport[] memory records = new ModerationModule.ImportReport[](1);
        records[0] = ModerationModule.ImportReport(
            ModerationModule.Report(
                10,
                repoId,
                BOB,
                ModerationModule.ReportStatus.Resolved,
                ModerationModule.RepoStatus.Frozen,
                "sha256:original",
                1,
                3,
                true
            ),
            trail
        );
        bytes memory payload = abi.encode(uint8(1), abi.encode(records));
        bytes32 root = _rootAfterBatch(
            SuiteIds.MODERATION, _emptyRoot(SuiteIds.MODERATION), 0, 1, payload
        );
        coordinator.beginNextModule(SuiteIds.MODERATION, 1, 1, root);
        vm.expectRevert(ModerationModule.InvalidImportRecord.selector);
        coordinator.importBatch(SuiteIds.MODERATION, 0, 1, keccak256(payload), payload);
    }

    function testModerationImportRestoresAppealResolutionStateMachine() public {
        bytes32 repoId = bytes32(uint256(0xA11D1B));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);

        ModerationModule.TrailEntry[] memory trail = new ModerationModule.TrailEntry[](4);
        trail[0] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Submitted,
            BOB,
            ModerationModule.RepoStatus.Active,
            "sha256:original",
            1
        );
        trail[1] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Resolved,
            address(this),
            ModerationModule.RepoStatus.Frozen,
            "sha256:decision",
            2
        );
        trail[2] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Appealed,
            ALICE,
            ModerationModule.RepoStatus.Frozen,
            "sha256:appeal",
            3
        );
        trail[3] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.AppealResolved,
            address(this),
            ModerationModule.RepoStatus.Delisted,
            "sha256:appeal-decision",
            4
        );
        ModerationModule.ImportReport[] memory records = new ModerationModule.ImportReport[](1);
        records[0] = ModerationModule.ImportReport(
            ModerationModule.Report(
                11,
                repoId,
                BOB,
                ModerationModule.ReportStatus.AppealResolved,
                ModerationModule.RepoStatus.Delisted,
                "sha256:original",
                1,
                4,
                true
            ),
            trail
        );
        bytes memory payload = abi.encode(uint8(1), abi.encode(records));
        bytes32 root = _rootAfterBatch(
            SuiteIds.MODERATION, _emptyRoot(SuiteIds.MODERATION), 0, 1, payload
        );
        coordinator.beginNextModule(SuiteIds.MODERATION, 1, 1, root);
        coordinator.importBatch(SuiteIds.MODERATION, 0, 1, keccak256(payload), payload);
        coordinator.finalizeCurrentModule(SuiteIds.MODERATION);

        ModerationModule.Report memory report = moderation.getReport(11);
        require(report.status == ModerationModule.ReportStatus.AppealResolved, "appeal state");
        require(report.resolution == ModerationModule.RepoStatus.Delisted, "appeal resolution");
        (string memory submission, string memory resolution, string memory appeal) =
            moderation.reportCommitments(11);
        require(keccak256(bytes(submission)) == keccak256(bytes("sha256:original")), "imported submission");
        require(keccak256(bytes(resolution)) == keccak256(bytes("sha256:appeal-decision")), "imported resolution");
        require(keccak256(bytes(appeal)) == keccak256(bytes("sha256:appeal")), "imported appeal");
    }

    function testModerationImportRejectsMalformedFirstTrailEntry() public {
        bytes32 repoId = bytes32(uint256(0xA11D18));
        _importCoreRepository(repoId, ALICE);
        _finalizeEmpty(SuiteIds.RECOVERY);

        ModerationModule.TrailEntry[] memory trail = new ModerationModule.TrailEntry[](1);
        trail[0] = ModerationModule.TrailEntry(
            ModerationModule.TrailAction.Resolved,
            address(this),
            ModerationModule.RepoStatus.Frozen,
            "sha256:not-submitted",
            1
        );
        ModerationModule.ImportReport[] memory records = new ModerationModule.ImportReport[](1);
        records[0] = ModerationModule.ImportReport(
            ModerationModule.Report(
                8,
                repoId,
                BOB,
                ModerationModule.ReportStatus.Resolved,
                ModerationModule.RepoStatus.Frozen,
                "sha256:not-submitted",
                1,
                1,
                true
            ),
            trail
        );
        bytes memory payload = abi.encode(uint8(1), abi.encode(records));
        bytes32 root = _rootAfterBatch(
            SuiteIds.MODERATION, _emptyRoot(SuiteIds.MODERATION), 0, 1, payload
        );
        coordinator.beginNextModule(SuiteIds.MODERATION, 1, 1, root);
        vm.expectRevert(ModerationModule.InvalidImportRecord.selector);
        coordinator.importBatch(SuiteIds.MODERATION, 0, 1, keccak256(payload), payload);
    }
}
