// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

import {RepoRegistryV2} from "./RepoRegistryV2.sol";

/// @title Deployment-level V1 import controller for RepoRegistryV2
/// @notice Keeps migration commitment and recovery state outside the nearly
/// full compatibility registry. It is intended for a reviewed bootstrap
/// deployment; normal users call the registry address exposed by `registry`.
contract RepoRegistryV2ImportController {
    string private constant BATCH_DOMAIN = "igit:v2:import:batch";
    address public immutable admin;
    RepoRegistryV2 public registry;
    bytes32 public registryCodeHash;
    bytes32 public activeCommitment;
    bytes32 public rollingCommitment;
    uint256 public nextSequence;
    uint256 public expectedBatches;
    uint256 public generation;
    bool public importActive;
    bool public published;

    error Unauthorized(address caller);
    error ImportAlreadyActive();
    error ImportNotActive();
    error ImportAlreadyPublished();
    error InvalidRegistry(address registry);
    error RegistryAlreadyBound();
    error SequenceMismatch(uint256 expected, uint256 actual);
    error BatchCountMismatch(uint256 expected, uint256 actual);
    error CommitmentMismatch(bytes32 expected, bytes32 actual);

    event RegistryCreated(address indexed registry, uint256 indexed generation);
    event RegistryReplaced(
        address indexed previousRegistry,
        address indexed replacementRegistry,
        uint256 indexed generation
    );
    event ImportSessionStarted(
        bytes32 indexed sessionId,
        bytes32 indexed snapshotHash,
        bytes32 indexed commitment,
        uint256 expectedBatches
    );
    event ImportBatchForwarded(
        bytes32 indexed sessionId,
        uint256 indexed sequence,
        bytes32 indexed repoId,
        bytes32 payloadHash,
        bytes32 rollingCommitment
    );
    event ImportPublished(bytes32 indexed sessionId, bytes32 indexed commitment);
    event ImportAborted(
        bytes32 indexed sessionId,
        address indexed abandonedRegistry,
        address indexed replacementRegistry,
        uint256 generation
    );

    constructor() {
        admin = msg.sender;
    }

    modifier onlyAdmin() {
        if (msg.sender != admin) revert Unauthorized(msg.sender);
        _;
    }

    /// @notice Create a session and bind it to the planner's ordered batch
    /// commitment. The underlying registry remains the source of repo state.
    function createImportSession(
        bytes32 snapshotHash,
        string calldata sourceChainId,
        address sourceContract,
        uint64 sourceHeight,
        uint256 expectedRepositories,
        uint256 expectedRefs,
        uint256 expectedCollaborators,
        uint256 batches,
        bytes32 commitment
    ) external onlyAdmin returns (bytes32 sessionId) {
        if (address(registry) == address(0)) revert InvalidRegistry(address(0));
        if (published) revert ImportAlreadyPublished();
        if (importActive) revert ImportAlreadyActive();
        sessionId = registry.createImportSession(
            snapshotHash,
            sourceChainId,
            sourceContract,
            sourceHeight,
            expectedRepositories,
            expectedRefs,
            expectedCollaborators,
            batches
        );
        activeCommitment = commitment;
        rollingCommitment = sha256(abi.encode(BATCH_DOMAIN, snapshotHash, address(registry)));
        nextSequence = 0;
        expectedBatches = batches;
        importActive = true;
        emit ImportSessionStarted(sessionId, snapshotHash, commitment, batches);
    }

    function importRepo(
        bytes32 sessionId,
        uint256 sequence,
        bytes32 repoId,
        address owner,
        string calldata name,
        string calldata description,
        string calldata defaultBranch,
        uint8 moderationStatus,
        uint64 createdAt,
        uint64 updatedAt,
        uint256 expectedRefs,
        uint256 expectedCollaborators,
        bytes32 payloadHash
    ) external onlyAdmin {
        _requireBatch(sessionId, sequence);
        registry.importRepo(
            sessionId,
            sequence,
            repoId,
            owner,
            name,
            description,
            defaultBranch,
            moderationStatus,
            createdAt,
            updatedAt,
            expectedRefs,
            expectedCollaborators,
            payloadHash
        );
        _recordBatch(sessionId, sequence, repoId, payloadHash, "import_repo");
    }

    function importRefs(
        bytes32 sessionId,
        uint256 sequence,
        bytes32 repoId,
        RepoRegistryV2.ImportRef[] calldata refs,
        bytes32 payloadHash
    ) external onlyAdmin {
        _requireBatch(sessionId, sequence);
        registry.importRefs(sessionId, sequence, repoId, refs, payloadHash);
        _recordBatch(sessionId, sequence, repoId, payloadHash, "import_refs");
    }

    function importCollaborators(
        bytes32 sessionId,
        uint256 sequence,
        bytes32 repoId,
        RepoRegistryV2.ImportCollaborator[] calldata collaborators,
        bytes32 payloadHash
    ) external onlyAdmin {
        _requireBatch(sessionId, sequence);
        registry.importCollaborators(sessionId, sequence, repoId, collaborators, payloadHash);
        _recordBatch(sessionId, sequence, repoId, payloadHash, "import_collaborators");
    }

    function finalizeImport(bytes32 sessionId) external onlyAdmin {
        _requireActiveSession(sessionId);
        if (nextSequence != expectedBatches) {
            revert BatchCountMismatch(expectedBatches, nextSequence);
        }
        if (rollingCommitment != activeCommitment) {
            revert CommitmentMismatch(activeCommitment, rollingCommitment);
        }
        registry.finalizeImport(sessionId);
        emit ImportPublished(sessionId, activeCommitment);
        published = true;
        importActive = false;
        activeCommitment = bytes32(0);
        expectedBatches = 0;
    }

    /// @notice Bind a registry deployed with this controller as its admin.
    /// Deployment is intentionally two-phase so registry creation bytecode is
    /// not embedded in this controller.
    function setRegistry(address candidate) external onlyAdmin {
        if (address(registry) != address(0)) revert RegistryAlreadyBound();
        _bindRegistry(candidate);
    }

    /// @notice Abandon a failed pre-release deployment and replace its empty
    /// registry. This is explicit deployment-level recovery, not a silent
    /// rollback of a live repository; the profile must move to the replacement.
    function abortImport(bytes32 sessionId, address replacement) external onlyAdmin {
        _requireActiveSession(sessionId);
        address abandoned = address(registry);
        _replaceRegistry(abandoned, replacement);
        importActive = false;
        activeCommitment = bytes32(0);
        rollingCommitment = bytes32(0);
        nextSequence = 0;
        expectedBatches = 0;
        emit ImportAborted(sessionId, abandoned, address(registry), generation);
    }

    /// @notice Preserve the registry deployment admin's moderation capability
    /// after the controller becomes the registry's immutable admin.
    function setFrozen(address owner, string calldata repo, bool frozen) external onlyAdmin {
        registry.setFrozen(owner, repo, frozen);
    }

    function _bindRegistry(address candidate) internal {
        if (candidate == address(0) || candidate.code.length == 0) {
            revert InvalidRegistry(candidate);
        }
        bytes32 candidateCodeHash = candidate.codehash;
        if (registryCodeHash != bytes32(0) && candidateCodeHash != registryCodeHash) {
            revert InvalidRegistry(candidate);
        }
        RepoRegistryV2 target = RepoRegistryV2(candidate);
        if (
            target.admin() != address(this) ||
            target.activeImportSession() != bytes32(0) ||
            target.importWindowClosed()
        ) {
            revert InvalidRegistry(candidate);
        }
        if (registryCodeHash == bytes32(0)) {
            registryCodeHash = candidateCodeHash;
        }
        registry = target;
        emit RegistryCreated(candidate, generation);
    }

    function _replaceRegistry(address previous, address replacement) internal {
        if (replacement == previous) revert InvalidRegistry(replacement);
        unchecked {
            generation += 1;
        }
        _bindRegistry(replacement);
        emit RegistryReplaced(previous, replacement, generation);
    }

    function _requireActiveSession(bytes32 sessionId) internal view {
        if (!importActive || registry.activeImportSession() != sessionId) {
            revert ImportNotActive();
        }
    }

    function _requireBatch(bytes32 sessionId, uint256 sequence) internal view {
        _requireActiveSession(sessionId);
        if (sequence != nextSequence) revert SequenceMismatch(nextSequence, sequence);
    }

    function _recordBatch(
        bytes32 sessionId,
        uint256 sequence,
        bytes32 repoId,
        bytes32 payloadHash,
        string memory kind
    ) internal {
        bytes32 kindHash = keccak256(bytes(kind));
        rollingCommitment = sha256(
            abi.encode(rollingCommitment, sequence, kindHash, repoId, payloadHash)
        );
        unchecked {
            nextSequence += 1;
        }
        emit ImportBatchForwarded(sessionId, sequence, repoId, payloadHash, rollingCommitment);
    }
}
