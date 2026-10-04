// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {IBootstrapModule, ISuiteDirectory} from "./ISuite.sol";

abstract contract SuiteModule is IBootstrapModule {
    uint256 public constant MAX_IMPORT_BATCH_ITEMS = 128;
    uint256 public constant MAX_IMPORT_PAYLOAD_BYTES = 96_000;

    address public immutable override suiteDirectory;
    address public immutable override bootstrapCoordinator;

    bool public bootstrapStarted;
    bool public override bootstrapFinalized;
    uint256 public expectedImportCount;
    uint256 public expectedImportBatches;
    uint256 public importedCount;
    uint256 public nextImportSequence;
    bytes32 public expectedImportRoot;
    bytes32 public rollingImportRoot;

    error InvalidSuiteDirectory();
    error InvalidBootstrapCoordinator();
    error UnauthorizedBootstrap(address caller);
    error SuiteNotActive();
    error BootstrapAlreadyStarted();
    error BootstrapNotStarted();
    error BootstrapAlreadyFinalized();
    error InvalidImportRoot();
    error ImportSequenceMismatch(uint256 expected, uint256 actual);
    error ImportBatchTooLarge(uint256 count, uint256 maximum);
    error ImportPayloadTooLarge(uint256 length, uint256 maximum);
    error ImportPayloadHashMismatch(bytes32 expected, bytes32 actual);
    error ImportCountMismatch(uint256 expected, uint256 actual);
    error ImportBatchCountMismatch(uint256 expected, uint256 actual);
    error ImportRootMismatch(bytes32 expected, bytes32 actual);

    event BootstrapStarted(uint256 expectedCount, uint256 expectedBatches, bytes32 expectedRoot);
    event BootstrapBatchImported(uint256 indexed sequence, uint256 count, bytes32 payloadHash, bytes32 rollingRoot);
    event BootstrapFinalized(uint256 importedCount, uint256 importedBatches, bytes32 importRoot);

    function moduleId() public pure virtual override returns (bytes32);

    constructor(address directory_, address coordinator_) {
        if (directory_ == address(0) || directory_.code.length == 0) revert InvalidSuiteDirectory();
        if (coordinator_ == address(0) || coordinator_.code.length == 0) revert InvalidBootstrapCoordinator();
        suiteDirectory = directory_;
        bootstrapCoordinator = coordinator_;
    }

    modifier onlyBootstrap() {
        if (msg.sender != bootstrapCoordinator) revert UnauthorizedBootstrap(msg.sender);
        _;
    }

    modifier onlyActiveSuite() {
        if (ISuiteDirectory(suiteDirectory).state() != 1) revert SuiteNotActive();
        _;
    }

    function beginBootstrap(
        uint256 expectedCount,
        uint256 expectedBatches,
        bytes32 expectedRoot,
        bytes32 snapshotRoot
    ) external onlyBootstrap {
        if (bootstrapStarted) revert BootstrapAlreadyStarted();
        if (expectedRoot == bytes32(0) || snapshotRoot == bytes32(0)) revert InvalidImportRoot();
        bootstrapStarted = true;
        expectedImportCount = expectedCount;
        expectedImportBatches = expectedBatches;
        expectedImportRoot = expectedRoot;
        rollingImportRoot = keccak256(abi.encode(moduleId(), snapshotRoot));
        emit BootstrapStarted(expectedCount, expectedBatches, expectedRoot);
    }

    function bootstrapImport(
        uint256 sequence,
        uint256 count,
        bytes32 payloadHash,
        bytes calldata payload
    ) external onlyBootstrap {
        if (!bootstrapStarted) revert BootstrapNotStarted();
        if (bootstrapFinalized) revert BootstrapAlreadyFinalized();
        if (sequence != nextImportSequence) revert ImportSequenceMismatch(nextImportSequence, sequence);
        if (count == 0 || count > MAX_IMPORT_BATCH_ITEMS) {
            revert ImportBatchTooLarge(count, MAX_IMPORT_BATCH_ITEMS);
        }
        if (payload.length == 0 || payload.length > MAX_IMPORT_PAYLOAD_BYTES) {
            revert ImportPayloadTooLarge(payload.length, MAX_IMPORT_PAYLOAD_BYTES);
        }
        bytes32 actualPayloadHash = keccak256(payload);
        if (actualPayloadHash != payloadHash) revert ImportPayloadHashMismatch(payloadHash, actualPayloadHash);
        uint256 actualCount = _importPayload(payload);
        if (actualCount != count) revert ImportCountMismatch(count, actualCount);

        rollingImportRoot = keccak256(
            abi.encode(rollingImportRoot, moduleId(), sequence, count, payloadHash)
        );
        unchecked {
            importedCount += count;
            nextImportSequence = sequence + 1;
        }
        if (importedCount > expectedImportCount || nextImportSequence > expectedImportBatches) {
            revert ImportCountMismatch(expectedImportCount, importedCount);
        }
        emit BootstrapBatchImported(sequence, count, payloadHash, rollingImportRoot);
    }

    function finalizeBootstrap() external onlyBootstrap {
        if (!bootstrapStarted) revert BootstrapNotStarted();
        if (bootstrapFinalized) revert BootstrapAlreadyFinalized();
        if (importedCount != expectedImportCount) revert ImportCountMismatch(expectedImportCount, importedCount);
        if (nextImportSequence != expectedImportBatches) {
            revert ImportBatchCountMismatch(expectedImportBatches, nextImportSequence);
        }
        if (rollingImportRoot != expectedImportRoot) {
            revert ImportRootMismatch(expectedImportRoot, rollingImportRoot);
        }
        _beforeBootstrapFinalized();
        bootstrapFinalized = true;
        emit BootstrapFinalized(importedCount, nextImportSequence, rollingImportRoot);
    }

    function _importPayload(bytes calldata payload) internal virtual returns (uint256);

    function _beforeBootstrapFinalized() internal virtual {}
}
