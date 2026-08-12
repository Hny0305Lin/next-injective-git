// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

interface IRepoRegistryV2EconomicRegistry {
    struct Repo {
        address owner;
        string name;
        string description;
        string defaultBranch;
        uint64 createdAt;
        uint64 updatedAt;
        uint8 moderationStatus;
        bool exists;
    }

    function getRepoById(bytes32 repoId) external view returns (Repo memory);
}

/// @title EVM V2 repository sponsorship and revenue split module
/// @notice Splits native INJ atomically while keying durable state by stable repo IDs.
contract RepoRegistryV2EconomicModule {
    uint8 private constant STATUS_FROZEN = 1;

    uint16 public constant MAX_PLATFORM_FEE_BPS = 500;
    uint256 public constant MAX_SPLIT_RECIPIENTS = 20;
    uint256 public constant MAX_SPONSOR_MESSAGE_LENGTH = 256;

    IRepoRegistryV2EconomicRegistry public immutable registry;
    address public immutable admin;
    address payable public treasury;
    uint16 public platformFeeBps;

    struct Split {
        address payable recipient;
        uint16 bps;
    }

    mapping(bytes32 repoId => Split[] splits) private _splits;
    mapping(bytes32 repoId => uint256 total) private _sponsorTotals;
    bool private _settling;

    error InvalidRegistry();
    error InvalidAdmin();
    error InvalidTreasury();
    error RepoNotFound(bytes32 repoId);
    error FrozenRepo(bytes32 repoId);
    error Unauthorized(address caller);
    error NoFunds();
    error InvalidMessageLength(uint256 length, uint256 maximum);
    error SplitLengthMismatch(uint256 recipients, uint256 basisPoints);
    error TooManySplitRecipients(uint256 count, uint256 maximum);
    error InvalidSplitRecipient(address recipient);
    error DuplicateSplitRecipient(address recipient);
    error InvalidSplitBps(address recipient, uint256 bps);
    error SplitTotalTooHigh(uint256 total, uint256 maximum);
    error PlatformFeeTooHigh(uint256 bps, uint256 maximum);
    error PayoutFailed(address recipient, uint256 amount);
    error ReentrantSettlement();

    event RevenueSplitsUpdated(bytes32 indexed repoId, address indexed owner, uint256 totalBps);
    event SponsorSettled(
        bytes32 indexed repoId,
        address indexed sponsor,
        address indexed owner,
        uint256 amount,
        uint256 platformFee,
        string message
    );
    event FeeConfigUpdated(address indexed treasury, uint16 platformFeeBps, address indexed updatedBy);

    constructor(address registryAddress, address initialAdmin, address payable initialTreasury, uint16 initialFeeBps) {
        if (registryAddress == address(0) || registryAddress.code.length == 0) revert InvalidRegistry();
        if (initialAdmin == address(0)) revert InvalidAdmin();
        if (initialTreasury == address(0)) revert InvalidTreasury();
        if (initialFeeBps > MAX_PLATFORM_FEE_BPS) {
            revert PlatformFeeTooHigh(initialFeeBps, MAX_PLATFORM_FEE_BPS);
        }
        registry = IRepoRegistryV2EconomicRegistry(registryAddress);
        admin = initialAdmin;
        treasury = initialTreasury;
        platformFeeBps = initialFeeBps;
    }

    /// @notice Replace a repository's revenue splits. The current owner is
    /// read from the core registry, so ownership transfer changes authority
    /// without copying or deleting economic state.
    function setRevenueSplits(bytes32 repoId, address payable[] calldata recipients, uint16[] calldata bps) external {
        if (_settling) revert ReentrantSettlement();
        IRepoRegistryV2EconomicRegistry.Repo memory repository = _repo(repoId);
        if (msg.sender != repository.owner) revert Unauthorized(msg.sender);
        uint256 count = recipients.length;
        if (count != bps.length) revert SplitLengthMismatch(count, bps.length);
        if (count > MAX_SPLIT_RECIPIENTS) revert TooManySplitRecipients(count, MAX_SPLIT_RECIPIENTS);

        delete _splits[repoId];
        uint256 total;
        for (uint256 i; i < count; ++i) {
            address payable recipient = recipients[i];
            uint256 share = bps[i];
            if (recipient == address(0) || recipient == repository.owner) {
                revert InvalidSplitRecipient(recipient);
            }
            if (share == 0) revert InvalidSplitBps(recipient, share);
            for (uint256 j; j < i; ++j) {
                if (recipients[j] == recipient) revert DuplicateSplitRecipient(recipient);
            }
            total += share;
            if (total > 10_000) revert SplitTotalTooHigh(total, 10_000);
            _splits[repoId].push(Split({recipient: recipient, bps: uint16(share)}));
        }
        emit RevenueSplitsUpdated(repoId, repository.owner, total);
    }

    /// @notice Sponsor a repository with native INJ. This call retains no
    /// balance: platform fee, explicit shares, and owner remainder are paid
    /// atomically or the entire transaction reverts.
    function sponsor(bytes32 repoId, string calldata message) external payable {
        if (_settling) revert ReentrantSettlement();
        if (msg.value == 0) revert NoFunds();
        uint256 messageLength = bytes(message).length;
        if (messageLength > MAX_SPONSOR_MESSAGE_LENGTH) {
            revert InvalidMessageLength(messageLength, MAX_SPONSOR_MESSAGE_LENGTH);
        }
        IRepoRegistryV2EconomicRegistry.Repo memory repository = _repo(repoId);
        if (repository.moderationStatus == STATUS_FROZEN) revert FrozenRepo(repoId);

        uint256 fee = (msg.value * platformFeeBps) / 10_000;
        uint256 distributable = msg.value - fee;
        uint256 assigned;
        _settling = true;
        _sponsorTotals[repoId] += msg.value;
        _pay(treasury, fee);

        Split[] storage splits = _splits[repoId];
        for (uint256 i; i < splits.length; ++i) {
            uint256 share = (distributable * splits[i].bps) / 10_000;
            assigned += share;
            _pay(splits[i].recipient, share);
        }
        _pay(payable(repository.owner), distributable - assigned);
        _settling = false;
        emit SponsorSettled(repoId, msg.sender, repository.owner, msg.value, fee, message);
    }

    function setFeeConfig(address payable newTreasury, uint16 newPlatformFeeBps) external {
        if (_settling) revert ReentrantSettlement();
        if (msg.sender != admin) revert Unauthorized(msg.sender);
        if (newTreasury == address(0)) revert InvalidTreasury();
        if (newPlatformFeeBps > MAX_PLATFORM_FEE_BPS) {
            revert PlatformFeeTooHigh(newPlatformFeeBps, MAX_PLATFORM_FEE_BPS);
        }
        treasury = newTreasury;
        platformFeeBps = newPlatformFeeBps;
        emit FeeConfigUpdated(newTreasury, newPlatformFeeBps, msg.sender);
    }

    function revenueSplits(bytes32 repoId) external view returns (Split[] memory) {
        _repo(repoId);
        return _splits[repoId];
    }

    function sponsorTotal(bytes32 repoId) external view returns (uint256) {
        _repo(repoId);
        return _sponsorTotals[repoId];
    }

    function _repo(bytes32 repoId) private view returns (IRepoRegistryV2EconomicRegistry.Repo memory repository) {
        try registry.getRepoById(repoId) returns (IRepoRegistryV2EconomicRegistry.Repo memory value) {
            repository = value;
        } catch {
            revert RepoNotFound(repoId);
        }
        if (!repository.exists) revert RepoNotFound(repoId);
    }

    function _pay(address payable recipient, uint256 amount) private {
        if (amount == 0) return;
        (bool ok,) = recipient.call{value: amount}("");
        if (!ok) revert PayoutFailed(recipient, amount);
    }

    receive() external payable {
        revert NoFunds();
    }
}
