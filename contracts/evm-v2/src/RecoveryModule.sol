// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {
    IRecoveryOwnershipHook,
    IOwnershipTransferState,
    IRecoveryState,
    IRepositoryCore,
    ISuiteDirectory,
    SuiteIds
} from "./suite/ISuite.sol";
import {SuiteModule} from "./suite/SuiteModule.sol";

contract RecoveryModule is SuiteModule, IRecoveryState, IRecoveryOwnershipHook {
    uint256 public constant MAX_GUARDIANS = 10;
    uint64 public constant RECOVERY_DELAY = 7 days;
    uint64 public constant RECOVERY_WINDOW = 30 days;

    struct GuardianConfig {
        address configuredBy;
        uint8 threshold;
        address[] guardians;
    }

    struct Proposal {
        address proposedBy;
        address newOwner;
        uint64 executeAfter;
        uint64 expiresAt;
        uint64 nonce;
        uint8 approvals;
    }

    struct ImportGuardianConfig {
        bytes32 repoId;
        address configuredBy;
        uint8 threshold;
        address[] guardians;
    }

    mapping(bytes32 repoId => GuardianConfig config) private _configs;
    mapping(bytes32 repoId => mapping(address guardian => bool enabled)) private _isGuardian;
    mapping(bytes32 repoId => Proposal proposal) private _proposals;
    mapping(bytes32 repoId => mapping(uint64 nonce => mapping(address guardian => bool approved))) private _approvals;
    mapping(bytes32 repoId => uint64 nonce) private _proposalNonces;

    error RepositoryNotFound(bytes32 repoId);
    error Unauthorized(address caller);
    error InvalidGuardian(address guardian);
    error DuplicateGuardian(address guardian);
    error InvalidGuardianThreshold(uint256 threshold, uint256 count);
    error InvalidRecoveryOwner(address owner);
    error GuardianConfigStale(address configuredBy, address currentOwner);
    error RecoveryAlreadyPending(bytes32 repoId);
    error RecoveryNotPending(bytes32 repoId);
    error RecoveryAlreadyApproved(address guardian);
    error RecoveryApprovalThreshold(uint256 required, uint256 actual);
    error RecoveryTooEarly(uint64 executeAfter);
    error RecoveryExpired(uint64 expiresAt);
    error OwnershipTransferPending(bytes32 repoId);
    error TimestampOverflow(uint256 timestamp);
    error InvalidImportRecord();

    event GuardiansConfigured(bytes32 indexed repoId, address indexed owner, uint8 threshold, address[] guardians);
    event RecoveryProposed(
        bytes32 indexed repoId,
        address indexed proposedBy,
        address indexed newOwner,
        uint64 executeAfter,
        uint64 expiresAt,
        uint64 nonce
    );
    event RecoveryApproved(bytes32 indexed repoId, address indexed guardian, uint8 approvals, uint64 nonce);
    event RecoveryCancelled(bytes32 indexed repoId, address indexed cancelledBy, uint64 nonce);
    event RecoveryExecuted(bytes32 indexed repoId, address indexed oldOwner, address indexed newOwner, uint64 nonce);

    constructor(address directory, address coordinator) SuiteModule(directory, coordinator) {}

    function moduleId() public pure override returns (bytes32) {
        return SuiteIds.RECOVERY;
    }

    function setGuardians(bytes32 repoId, address[] calldata guardians, uint8 threshold) external onlyActiveSuite {
        IRepositoryCore.Repository memory repository = _repository(repoId);
        if (msg.sender != repository.owner) revert Unauthorized(msg.sender);
        _setGuardians(repoId, repository.owner, guardians, threshold);
        delete _proposals[repoId];
    }

    function proposeRecovery(bytes32 repoId, address newOwner) external onlyActiveSuite {
        IRepositoryCore.Repository memory repository = _repository(repoId);
        GuardianConfig storage config = _activeConfig(repoId, repository.owner);
        if (!_isGuardian[repoId][msg.sender]) revert Unauthorized(msg.sender);
        if (newOwner == address(0) || newOwner == repository.owner) revert InvalidRecoveryOwner(newOwner);
        if (_proposals[repoId].newOwner != address(0)) revert RecoveryAlreadyPending(repoId);
        (address pendingOwner,,) = IOwnershipTransferState(_core()).pendingOwnershipTransfer(repoId);
        if (pendingOwner != address(0)) {
            revert OwnershipTransferPending(repoId);
        }
        uint64 nonce = ++_proposalNonces[repoId];
        uint64 executeAfter = _addTime(_now64(), RECOVERY_DELAY);
        uint64 expiresAt = _addTime(executeAfter, RECOVERY_WINDOW);
        _proposals[repoId] = Proposal(msg.sender, newOwner, executeAfter, expiresAt, nonce, 1);
        _approvals[repoId][nonce][msg.sender] = true;
        emit RecoveryProposed(repoId, msg.sender, newOwner, executeAfter, expiresAt, nonce);
        emit RecoveryApproved(repoId, msg.sender, 1, nonce);
        if (config.threshold == 0) revert InvalidGuardianThreshold(0, config.guardians.length);
    }

    function approveRecovery(bytes32 repoId) external onlyActiveSuite {
        IRepositoryCore.Repository memory repository = _repository(repoId);
        _activeConfig(repoId, repository.owner);
        Proposal storage proposal = _proposals[repoId];
        if (proposal.newOwner == address(0)) revert RecoveryNotPending(repoId);
        if (_now64() > proposal.expiresAt) revert RecoveryExpired(proposal.expiresAt);
        if (!_isGuardian[repoId][msg.sender]) revert Unauthorized(msg.sender);
        if (_approvals[repoId][proposal.nonce][msg.sender]) revert RecoveryAlreadyApproved(msg.sender);
        _approvals[repoId][proposal.nonce][msg.sender] = true;
        proposal.approvals++;
        emit RecoveryApproved(repoId, msg.sender, proposal.approvals, proposal.nonce);
    }

    function cancelRecovery(bytes32 repoId) external override onlyActiveSuite {
        IRepositoryCore.Repository memory repository = _repository(repoId);
        Proposal memory proposal = _proposals[repoId];
        address core = _core();
        address economic = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.ECONOMIC);
        bool coreCaller = msg.sender == core || msg.sender == economic;
        if (coreCaller) {
            _clearConfig(repoId);
            delete _proposals[repoId];
            return;
        }
        if (proposal.newOwner == address(0)) revert RecoveryNotPending(repoId);
        if (msg.sender != repository.owner && msg.sender != proposal.newOwner) revert Unauthorized(msg.sender);
        delete _proposals[repoId];
        emit RecoveryCancelled(repoId, msg.sender, proposal.nonce);
    }

    function executeRecovery(bytes32 repoId) external onlyActiveSuite {
        IRepositoryCore.Repository memory repository = _repository(repoId);
        GuardianConfig storage config = _activeConfig(repoId, repository.owner);
        Proposal memory proposal = _proposals[repoId];
        if (proposal.newOwner == address(0)) revert RecoveryNotPending(repoId);
        if (msg.sender != proposal.newOwner) revert Unauthorized(msg.sender);
        uint64 now64 = _now64();
        if (now64 < proposal.executeAfter) revert RecoveryTooEarly(proposal.executeAfter);
        if (now64 > proposal.expiresAt) revert RecoveryExpired(proposal.expiresAt);
        if (proposal.approvals < config.threshold) {
            revert RecoveryApprovalThreshold(config.threshold, proposal.approvals);
        }
        delete _proposals[repoId];
        _clearConfig(repoId);
        IRepositoryCore(_core()).recoverOwnership(repoId, proposal.newOwner);
        emit RecoveryExecuted(repoId, repository.owner, proposal.newOwner, proposal.nonce);
    }

    function guardianConfig(bytes32 repoId) external view returns (GuardianConfig memory) {
        _repository(repoId);
        return _configs[repoId];
    }

    function recoveryProposal(bytes32 repoId) external view returns (Proposal memory) {
        _repository(repoId);
        return _proposals[repoId];
    }

    function hasApproved(bytes32 repoId, uint64 nonce, address guardian) external view returns (bool) {
        return _approvals[repoId][nonce][guardian];
    }

    function hasPendingRecovery(bytes32 repoId) external view override returns (bool) {
        return _proposals[repoId].newOwner != address(0);
    }

    function _importPayload(bytes calldata payload) internal override returns (uint256 count) {
        ImportGuardianConfig[] memory records = abi.decode(payload, (ImportGuardianConfig[]));
        count = records.length;
        for (uint256 i; i < count; ++i) {
            IRepositoryCore.Repository memory repository = _repository(records[i].repoId);
            if (repository.owner != records[i].configuredBy || _configs[records[i].repoId].threshold != 0) {
                revert InvalidImportRecord();
            }
            _setGuardians(records[i].repoId, records[i].configuredBy, records[i].guardians, records[i].threshold);
        }
    }

    function _setGuardians(bytes32 repoId, address owner, address[] memory guardians, uint8 threshold) private {
        if (guardians.length == 0 || guardians.length > MAX_GUARDIANS || threshold == 0 || threshold > guardians.length) {
            revert InvalidGuardianThreshold(threshold, guardians.length);
        }
        _clearConfig(repoId);
        GuardianConfig storage config = _configs[repoId];
        config.configuredBy = owner;
        config.threshold = threshold;
        for (uint256 i; i < guardians.length; ++i) {
            address guardian = guardians[i];
            if (guardian == address(0) || guardian == owner) revert InvalidGuardian(guardian);
            if (_isGuardian[repoId][guardian]) revert DuplicateGuardian(guardian);
            _isGuardian[repoId][guardian] = true;
            config.guardians.push(guardian);
        }
        emit GuardiansConfigured(repoId, owner, threshold, guardians);
    }

    function _clearConfig(bytes32 repoId) private {
        address[] storage guardians = _configs[repoId].guardians;
        for (uint256 i; i < guardians.length; ++i) delete _isGuardian[repoId][guardians[i]];
        delete _configs[repoId];
    }

    function _activeConfig(bytes32 repoId, address currentOwner) private view returns (GuardianConfig storage config) {
        config = _configs[repoId];
        if (config.threshold == 0) revert InvalidGuardianThreshold(0, 0);
        if (config.configuredBy != currentOwner) revert GuardianConfigStale(config.configuredBy, currentOwner);
    }

    function _repository(bytes32 repoId) private view returns (IRepositoryCore.Repository memory repository) {
        try IRepositoryCore(_core()).getRepository(repoId) returns (IRepositoryCore.Repository memory value) {
            return value;
        } catch {
            revert RepositoryNotFound(repoId);
        }
    }

    function _core() private view returns (address core) {
        core = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.CORE);
        if (core == address(0)) revert SuiteNotActive();
    }

    function _now64() private view returns (uint64) {
        if (block.timestamp > type(uint64).max) revert TimestampOverflow(block.timestamp);
        return uint64(block.timestamp);
    }

    function _addTime(uint64 timestamp, uint64 delay) private pure returns (uint64) {
        uint256 result = uint256(timestamp) + delay;
        if (result > type(uint64).max) revert TimestampOverflow(result);
        return uint64(result);
    }
}
