// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {IBootstrapModule, SuiteIds} from "./suite/ISuite.sol";
import {SuiteDirectory} from "./SuiteDirectory.sol";

contract BootstrapCoordinator {
    struct ModuleProgress {
        bool started;
        bool finalized;
        uint256 expectedCount;
        uint256 expectedBatches;
        uint256 importedCount;
        uint256 nextSequence;
        bytes32 expectedRoot;
        bytes32 rollingRoot;
    }

    address public immutable suiteDirectory;
    bytes32 public immutable snapshotRoot;
    address public immutable operator;
    uint256 public nextModuleIndex;
    bytes32 public usernameEscrowEvidenceHash;
    bool public activated;

    mapping(bytes32 id => ModuleProgress progress) public moduleProgress;

    error InvalidDirectory();
    error InvalidOperator();
    error Unauthorized(address caller);
    error SuiteAlreadyActivated();
    error InvalidCodeHash();
    error ModuleOrderMismatch(bytes32 expected, bytes32 actual);
    error ModuleNotConfigured(bytes32 id);
    error ModuleAlreadyStarted(bytes32 id);
    error ModuleNotStarted(bytes32 id);
    error ModuleAlreadyFinalized(bytes32 id);
    error SequenceMismatch(uint256 expected, uint256 actual);
    error CountExceeded(uint256 expected, uint256 actual);
    error BatchCountExceeded(uint256 expected, uint256 actual);
    error RootMismatch(bytes32 expected, bytes32 actual);
    error UsernameEscrowEvidenceRequired();
    error UsernameEscrowEvidenceAlreadySet();
    error ModulesIncomplete(uint256 finalized, uint256 required);

    event ModuleRegistered(bytes32 indexed id, address indexed module, bytes32 indexed codeHash);
    event ModuleImportStarted(
        bytes32 indexed id,
        uint256 expectedCount,
        uint256 expectedBatches,
        bytes32 expectedRoot
    );
    event ModuleBatchImported(
        bytes32 indexed id,
        uint256 indexed sequence,
        uint256 count,
        bytes32 payloadHash,
        bytes32 rollingRoot
    );
    event ModuleImportFinalized(bytes32 indexed id, uint256 count, uint256 batches, bytes32 root);
    event UsernameEscrowReleaseAttested(bytes32 indexed evidenceHash);
    event SuiteActivationRequested(bytes32 indexed snapshotRoot);

    constructor(address directory, address operator_) {
        if (directory == address(0) || directory.code.length == 0) revert InvalidDirectory();
        if (operator_ == address(0)) revert InvalidOperator();
        SuiteDirectory target = SuiteDirectory(directory);
        suiteDirectory = directory;
        snapshotRoot = target.snapshotRoot();
        operator = operator_;
    }

    modifier onlyOperator() {
        if (msg.sender != operator) revert Unauthorized(msg.sender);
        if (activated) revert SuiteAlreadyActivated();
        _;
    }

    function registerModule(bytes32 id, address module, bytes32 codeHash) external onlyOperator {
        if (codeHash == bytes32(0)) revert InvalidCodeHash();
        SuiteDirectory(suiteDirectory).configureModule(id, module, codeHash);
        emit ModuleRegistered(id, module, codeHash);
    }

    function beginNextModule(
        bytes32 id,
        uint256 expectedCount,
        uint256 expectedBatches,
        bytes32 expectedRoot
    ) external onlyOperator {
        bytes32 expectedId = SuiteDirectory(suiteDirectory).requiredModuleAt(nextModuleIndex);
        if (id != expectedId) revert ModuleOrderMismatch(expectedId, id);
        address module = SuiteDirectory(suiteDirectory).moduleAddress(id);
        if (module == address(0)) revert ModuleNotConfigured(id);
        ModuleProgress storage progress = moduleProgress[id];
        if (progress.started) revert ModuleAlreadyStarted(id);
        progress.started = true;
        progress.expectedCount = expectedCount;
        progress.expectedBatches = expectedBatches;
        progress.expectedRoot = expectedRoot;
        progress.rollingRoot = keccak256(abi.encode(id, snapshotRoot));
        IBootstrapModule(module).beginBootstrap(expectedCount, expectedBatches, expectedRoot, snapshotRoot);
        emit ModuleImportStarted(id, expectedCount, expectedBatches, expectedRoot);
    }

    function importBatch(
        bytes32 id,
        uint256 sequence,
        uint256 count,
        bytes32 payloadHash,
        bytes calldata payload
    ) external onlyOperator {
        bytes32 expectedId = SuiteDirectory(suiteDirectory).requiredModuleAt(nextModuleIndex);
        if (id != expectedId) revert ModuleOrderMismatch(expectedId, id);
        ModuleProgress storage progress = moduleProgress[id];
        if (!progress.started) revert ModuleNotStarted(id);
        if (progress.finalized) revert ModuleAlreadyFinalized(id);
        if (sequence != progress.nextSequence) revert SequenceMismatch(progress.nextSequence, sequence);
        if (keccak256(payload) != payloadHash) revert RootMismatch(payloadHash, keccak256(payload));
        uint256 newCount = progress.importedCount + count;
        if (newCount > progress.expectedCount) revert CountExceeded(progress.expectedCount, newCount);
        uint256 newSequence = sequence + 1;
        if (newSequence > progress.expectedBatches) {
            revert BatchCountExceeded(progress.expectedBatches, newSequence);
        }

        IBootstrapModule(SuiteDirectory(suiteDirectory).moduleAddress(id)).bootstrapImport(
            sequence, count, payloadHash, payload
        );
        progress.importedCount = newCount;
        progress.nextSequence = newSequence;
        progress.rollingRoot = keccak256(
            abi.encode(progress.rollingRoot, id, sequence, count, payloadHash)
        );
        emit ModuleBatchImported(id, sequence, count, payloadHash, progress.rollingRoot);
    }

    function finalizeCurrentModule(bytes32 id) external onlyOperator {
        bytes32 expectedId = SuiteDirectory(suiteDirectory).requiredModuleAt(nextModuleIndex);
        if (id != expectedId) revert ModuleOrderMismatch(expectedId, id);
        ModuleProgress storage progress = moduleProgress[id];
        if (!progress.started) revert ModuleNotStarted(id);
        if (progress.finalized) revert ModuleAlreadyFinalized(id);
        if (progress.importedCount != progress.expectedCount) {
            revert CountExceeded(progress.expectedCount, progress.importedCount);
        }
        if (progress.nextSequence != progress.expectedBatches) {
            revert BatchCountExceeded(progress.expectedBatches, progress.nextSequence);
        }
        if (progress.rollingRoot != progress.expectedRoot) {
            revert RootMismatch(progress.expectedRoot, progress.rollingRoot);
        }
        if (id == SuiteIds.USERNAME && usernameEscrowEvidenceHash == bytes32(0)) {
            revert UsernameEscrowEvidenceRequired();
        }

        IBootstrapModule(SuiteDirectory(suiteDirectory).moduleAddress(id)).finalizeBootstrap();
        progress.finalized = true;
        unchecked {
            nextModuleIndex++;
        }
        emit ModuleImportFinalized(id, progress.importedCount, progress.nextSequence, progress.rollingRoot);
    }

    function attestUsernameEscrowReleased(bytes32 evidenceHash) external onlyOperator {
        if (evidenceHash == bytes32(0)) revert UsernameEscrowEvidenceRequired();
        if (usernameEscrowEvidenceHash != bytes32(0)) revert UsernameEscrowEvidenceAlreadySet();
        usernameEscrowEvidenceHash = evidenceHash;
        emit UsernameEscrowReleaseAttested(evidenceHash);
    }

    function activateSuite() external onlyOperator {
        if (!readyForActivation()) revert ModulesIncomplete(nextModuleIndex, SuiteIds.REQUIRED_MODULES);
        SuiteDirectory(suiteDirectory).activate();
        activated = true;
        emit SuiteActivationRequested(snapshotRoot);
    }

    function readyForActivation() public view returns (bool) {
        return !activated && nextModuleIndex == SuiteIds.REQUIRED_MODULES
            && usernameEscrowEvidenceHash != bytes32(0);
    }
}
