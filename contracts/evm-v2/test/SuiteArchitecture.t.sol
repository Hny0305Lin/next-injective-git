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
        vm.expectRevert(
            abi.encodeWithSelector(
                SuiteModule.ImportBatchTooLarge.selector,
                core.MAX_IMPORT_BATCH_ITEMS() + 1,
                core.MAX_IMPORT_BATCH_ITEMS()
            )
        );
        vm.prank(address(coordinator));
        core.bootstrapImport(0, core.MAX_IMPORT_BATCH_ITEMS() + 1, keccak256(payload), payload);

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
