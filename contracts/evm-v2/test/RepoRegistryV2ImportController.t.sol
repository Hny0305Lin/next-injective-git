// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

import {RepoRegistryV2} from "../src/RepoRegistryV2.sol";
import {RepoRegistryV2ImportController} from "../src/RepoRegistryV2ImportController.sol";

interface VmController {
    function prank(address sender) external;
    function expectRevert(bytes calldata revertData) external;
}

contract RegistryLookalike {
    address public immutable admin;

    constructor(address initialAdmin) {
        admin = initialAdmin;
    }

    function activeImportSession() external pure returns (bytes32) {
        return bytes32(0);
    }

    function importWindowClosed() external pure returns (bool) {
        return false;
    }
}

abstract contract RepoRegistryV2ImportControllerTestBase {
    VmController internal constant vm =
        VmController(address(uint160(uint256(keccak256("hevm cheat code")))));

    string internal constant BATCH_DOMAIN = "igit:v2:import:batch";
    string internal constant IMPORT_DOMAIN = "igit:v2:import";
    string internal constant SOURCE_CHAIN = "injective-888";
    address internal constant SOURCE_CONTRACT = address(0xC05);
    bytes32 internal constant SNAPSHOT_HASH = bytes32(uint256(0x5151));
    address internal constant ALICE = address(0xA11CE);

    RepoRegistryV2ImportController internal controller;
    RepoRegistryV2 internal registry;

    function setUp() public virtual {
        controller = new RepoRegistryV2ImportController();
        registry = new RepoRegistryV2(address(controller));
        controller.setRegistry(address(registry));
    }

    function seed(bytes32 snapshotHash, RepoRegistryV2 target) internal pure returns (bytes32) {
        return sha256(abi.encode(BATCH_DOMAIN, snapshotHash, address(target)));
    }

    function importedRepoId(RepoRegistryV2 target, address owner, string memory name)
        internal
        view
        returns (bytes32)
    {
        return keccak256(
            abi.encode(
                IMPORT_DOMAIN,
                target.identityChainId(),
                address(target),
                SOURCE_CHAIN,
                SOURCE_CONTRACT,
                owner,
                name
            )
        );
    }

    function repoPayloadHash(bytes32 repoId, address owner, string memory name)
        internal
        pure
        returns (bytes32)
    {
        return sha256(abi.encode(repoId, owner, name, "legacy", "main", uint8(0), uint64(1), uint64(2), 0, 0));
    }

    function oneRepoCommitment(bytes32 snapshotHash, RepoRegistryV2 target, bytes32 repoId, bytes32 payloadHash)
        internal
        pure
        returns (bytes32)
    {
        return sha256(
            abi.encode(seed(snapshotHash, target), uint256(0), keccak256(bytes("import_repo")), repoId, payloadHash)
        );
    }

    function createZeroBatchSession(bytes32 snapshotHash, bytes32 commitment) internal {
        controller.createImportSession(
            snapshotHash,
            SOURCE_CHAIN,
            SOURCE_CONTRACT,
            4_242,
            0,
            0,
            0,
            0,
            commitment
        );
    }

    function createOneRepoSession(bytes32 commitment) internal {
        controller.createImportSession(
            SNAPSHOT_HASH,
            SOURCE_CHAIN,
            SOURCE_CONTRACT,
            4_242,
            1,
            0,
            0,
            1,
            commitment
        );
    }

    function applyOneRepo(bytes32 repoId, bytes32 payloadHash, uint256 sequence) internal {
        controller.importRepo(
            SNAPSHOT_HASH,
            sequence,
            repoId,
            ALICE,
            "legacy",
            "legacy",
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

contract RepoRegistryV2ImportControllerCoreTest is RepoRegistryV2ImportControllerTestBase {
    function testOrderedCommitmentFinalizesAndPublishes() public {
        bytes32 repoId = importedRepoId(registry, ALICE, "legacy");
        bytes32 payloadHash = repoPayloadHash(repoId, ALICE, "legacy");
        createOneRepoSession(oneRepoCommitment(SNAPSHOT_HASH, registry, repoId, payloadHash));

        applyOneRepo(repoId, payloadHash, 0);
        controller.finalizeImport(SNAPSHOT_HASH);

        RepoRegistryV2.Repo memory imported = registry.getRepoById(repoId);
        require(imported.owner == ALICE, "owner");
        require(imported.exists, "repo not published");
        require(controller.published(), "controller not published");
        require(controller.nextSequence() == 1, "sequence");
    }

    function testWrongSequenceDoesNotReachRegistry() public {
        bytes32 repoId = importedRepoId(registry, ALICE, "legacy");
        bytes32 payloadHash = repoPayloadHash(repoId, ALICE, "legacy");
        createOneRepoSession(oneRepoCommitment(SNAPSHOT_HASH, registry, repoId, payloadHash));

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2ImportController.SequenceMismatch.selector, uint256(0), uint256(1))
        );
        applyOneRepo(repoId, payloadHash, 1);
        require(controller.nextSequence() == 0, "sequence advanced");
        require(registry.activeImportSession() == SNAPSHOT_HASH, "registry session changed");
    }

    function testCommitmentMismatchBlocksPublication() public {
        bytes32 repoId = importedRepoId(registry, ALICE, "legacy");
        bytes32 payloadHash = repoPayloadHash(repoId, ALICE, "legacy");
        createOneRepoSession(bytes32(uint256(1)));
        applyOneRepo(repoId, payloadHash, 0);

        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2ImportController.CommitmentMismatch.selector,
                bytes32(uint256(1)),
                oneRepoCommitment(SNAPSHOT_HASH, registry, repoId, payloadHash)
            )
        );
        controller.finalizeImport(SNAPSHOT_HASH);
        require(!controller.published(), "published despite mismatch");
    }

    function testPayloadMismatchDoesNotAdvanceControllerCommitment() public {
        bytes32 repoId = importedRepoId(registry, ALICE, "legacy");
        bytes32 payloadHash = repoPayloadHash(repoId, ALICE, "legacy");
        createOneRepoSession(oneRepoCommitment(SNAPSHOT_HASH, registry, repoId, payloadHash));

        bytes32 wrongPayload = bytes32(uint256(0xDEAD));
        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2.ImportPayloadMismatch.selector,
                payloadHash,
                wrongPayload
            )
        );
        applyOneRepo(repoId, wrongPayload, 0);

        require(controller.nextSequence() == 0, "sequence advanced");
        require(controller.rollingCommitment() == seed(SNAPSHOT_HASH, registry), "commitment advanced");
    }

    function testZeroBatchCommitmentUsesDomainSeparatedSeed() public {
        bytes32 emptySnapshot = bytes32(uint256(0xABCD));
        createZeroBatchSession(emptySnapshot, seed(emptySnapshot, registry));
        controller.finalizeImport(emptySnapshot);
        require(controller.published(), "zero batch did not publish");
    }

    function testControllerAdminAndRegistryBindingAreStrict() public {
        RepoRegistryV2ImportController second = new RepoRegistryV2ImportController();
        RepoRegistryV2 wrongAdminRegistry = new RepoRegistryV2(address(this));

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2ImportController.InvalidRegistry.selector, address(wrongAdminRegistry))
        );
        second.setRegistry(address(wrongAdminRegistry));

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ImportController.RegistryAlreadyBound.selector));
        controller.setRegistry(address(wrongAdminRegistry));

        RepoRegistryV2 candidate = new RepoRegistryV2(address(second));
        vm.prank(address(second));
        candidate.createImportSession(
            SNAPSHOT_HASH,
            SOURCE_CHAIN,
            SOURCE_CONTRACT,
            4_242,
            0,
            0,
            0,
            0
        );
        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2ImportController.InvalidRegistry.selector, address(candidate))
        );
        second.setRegistry(address(candidate));
    }

    function testControllerRejectsRegistryThatAlreadyPublishedState() public {
        RepoRegistryV2ImportController second = new RepoRegistryV2ImportController();
        RepoRegistryV2 candidate = new RepoRegistryV2(address(second));
        vm.prank(address(second));
        candidate.createRepo("already-live", "", "main");

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2ImportController.InvalidRegistry.selector, address(candidate))
        );
        second.setRegistry(address(candidate));
    }

    function testControllerRejectsRegistryThatAlreadyFinalizedImport() public {
        RepoRegistryV2ImportController second = new RepoRegistryV2ImportController();
        RepoRegistryV2 candidate = new RepoRegistryV2(address(second));
        bytes32 snapshot = bytes32(uint256(0xBEEF));

        vm.prank(address(second));
        candidate.createImportSession(
            snapshot,
            SOURCE_CHAIN,
            SOURCE_CONTRACT,
            4_242,
            0,
            0,
            0,
            0
        );
        vm.prank(address(second));
        candidate.finalizeImport(snapshot);

        vm.expectRevert(
            abi.encodeWithSelector(RepoRegistryV2ImportController.InvalidRegistry.selector, address(candidate))
        );
        second.setRegistry(address(candidate));
    }
}

contract RepoRegistryV2ImportControllerRecoveryTest is RepoRegistryV2ImportControllerTestBase {
    function testAbortMovesToFreshRegistryAndOldCommitmentCannotPublish() public {
        bytes32 oldSeed = seed(SNAPSHOT_HASH, registry);
        createZeroBatchSession(SNAPSHOT_HASH, oldSeed);
        RepoRegistryV2 abandoned = registry;
        RepoRegistryV2 replacement = new RepoRegistryV2(address(controller));

        controller.abortImport(SNAPSHOT_HASH, address(replacement));
        require(address(controller.registry()) == address(replacement), "replacement not active");
        require(controller.generation() == 1, "generation");
        require(abandoned.activeImportSession() == SNAPSHOT_HASH, "old session missing");

        createZeroBatchSession(SNAPSHOT_HASH, oldSeed);
        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2ImportController.CommitmentMismatch.selector,
                oldSeed,
                seed(SNAPSHOT_HASH, replacement)
            )
        );
        controller.finalizeImport(SNAPSHOT_HASH);
    }

    function testNonAdminCannotAbortActiveImport() public {
        createZeroBatchSession(SNAPSHOT_HASH, seed(SNAPSHOT_HASH, registry));
        RepoRegistryV2 replacement = new RepoRegistryV2(address(controller));

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ImportController.Unauthorized.selector, ALICE));
        vm.prank(ALICE);
        controller.abortImport(SNAPSHOT_HASH, address(replacement));

        require(address(controller.registry()) == address(registry), "non-admin changed registry");
        require(controller.importActive(), "non-admin ended import");
    }

    function testAbortRejectsDifferentRegistryRuntime() public {
        createZeroBatchSession(SNAPSHOT_HASH, seed(SNAPSHOT_HASH, registry));
        RegistryLookalike replacement = new RegistryLookalike(address(controller));

        vm.expectRevert(
            abi.encodeWithSelector(
                RepoRegistryV2ImportController.InvalidRegistry.selector,
                address(replacement)
            )
        );
        controller.abortImport(SNAPSHOT_HASH, address(replacement));

        require(address(controller.registry()) == address(registry), "runtime mismatch changed registry");
        require(controller.importActive(), "runtime mismatch ended import");
    }

    function testFinalizationPreventsFurtherSessionOrAbort() public {
        bytes32 snapshot = bytes32(uint256(0x1234));
        createZeroBatchSession(snapshot, seed(snapshot, registry));
        controller.finalizeImport(snapshot);

        bytes32 commitment = seed(snapshot, registry);
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ImportController.ImportAlreadyPublished.selector));
        createZeroBatchSession(snapshot, commitment);

        RepoRegistryV2 replacement = new RepoRegistryV2(address(controller));
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ImportController.ImportNotActive.selector));
        controller.abortImport(snapshot, address(replacement));
        require(address(controller.registry()) == address(registry), "published registry changed");
    }
}
