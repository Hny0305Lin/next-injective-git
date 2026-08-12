// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

interface IRepoRegistryV2BadgeRegistry {
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

/// @title EVM V2 contribution badge module
/// @notice Stores non-transferable contribution records against stable repo IDs.
contract RepoRegistryV2BadgeModule {
    uint8 private constant STATUS_ACTIVE = 0;

    uint256 public constant MAX_REASON_LENGTH = 256;
    uint256 public constant MAX_QUERY_PAGE_SIZE = 64;

    IRepoRegistryV2BadgeRegistry public immutable registry;

    struct Badge {
        uint256 id;
        bytes32 repoId;
        address recipient;
        string reason;
        address awardedBy;
        uint64 awardedAt;
    }

    uint256 public nextBadgeId = 1;
    mapping(uint256 id => Badge badge) private _badges;
    mapping(address recipient => uint256[] ids) private _recipientBadgeIds;
    mapping(bytes32 repoId => uint256[] ids) private _repoBadgeIds;

    error InvalidRegistry();
    error RepoNotFound(bytes32 repoId);
    error RepoNotActive(bytes32 repoId, uint8 status);
    error Unauthorized(address caller);
    error InvalidRecipient(address recipient);
    error InvalidReasonLength(uint256 length, uint256 maximum);
    error BadgeNotFound(uint256 id);
    error InvalidPageSize(uint256 requested, uint256 maximum);
    error InvalidCursor(uint256 cursor, uint256 total);
    error TimestampOverflow(uint256 timestamp);

    event BadgeAwarded(
        uint256 indexed id,
        bytes32 indexed repoId,
        address indexed recipient,
        address awardedBy,
        string reason,
        uint64 awardedAt
    );

    constructor(address registryAddress) {
        if (registryAddress == address(0) || registryAddress.code.length == 0) {
            revert InvalidRegistry();
        }
        registry = IRepoRegistryV2BadgeRegistry(registryAddress);
    }

    function awardBadge(bytes32 repoId, address recipient, string calldata reason) external returns (uint256 id) {
        IRepoRegistryV2BadgeRegistry.Repo memory repository = registry.getRepoById(repoId);
        if (!repository.exists) revert RepoNotFound(repoId);
        if (repository.owner != msg.sender) revert Unauthorized(msg.sender);
        if (repository.moderationStatus != STATUS_ACTIVE) {
            revert RepoNotActive(repoId, repository.moderationStatus);
        }
        if (recipient == address(0) || recipient == repository.owner) {
            revert InvalidRecipient(recipient);
        }
        uint256 reasonLength = bytes(reason).length;
        if (reasonLength == 0 || reasonLength > MAX_REASON_LENGTH) {
            revert InvalidReasonLength(reasonLength, MAX_REASON_LENGTH);
        }
        if (block.timestamp > type(uint64).max) revert TimestampOverflow(block.timestamp);

        id = nextBadgeId++;
        uint64 awardedAt = uint64(block.timestamp);
        _badges[id] = Badge({
            id: id,
            repoId: repoId,
            recipient: recipient,
            reason: reason,
            awardedBy: msg.sender,
            awardedAt: awardedAt
        });
        _recipientBadgeIds[recipient].push(id);
        _repoBadgeIds[repoId].push(id);
        emit BadgeAwarded(id, repoId, recipient, msg.sender, reason, awardedAt);
    }

    function getBadge(uint256 id) external view returns (Badge memory) {
        Badge memory badge = _badges[id];
        if (badge.id == 0) revert BadgeNotFound(id);
        return badge;
    }

    function listBadgesByRecipientPage(
        address recipient,
        uint256 cursor,
        uint256 limit
    ) external view returns (uint256 nextCursor, bool hasMore, Badge[] memory badges) {
        return _page(_recipientBadgeIds[recipient], cursor, limit);
    }

    function listBadgesByRepoPage(
        bytes32 repoId,
        uint256 cursor,
        uint256 limit
    ) external view returns (uint256 nextCursor, bool hasMore, Badge[] memory badges) {
        registry.getRepoById(repoId);
        return _page(_repoBadgeIds[repoId], cursor, limit);
    }

    function _page(
        uint256[] storage ids,
        uint256 cursor,
        uint256 limit
    ) private view returns (uint256 nextCursor, bool hasMore, Badge[] memory badges) {
        uint256 total = ids.length;
        if (limit == 0 || limit > MAX_QUERY_PAGE_SIZE) {
            revert InvalidPageSize(limit, MAX_QUERY_PAGE_SIZE);
        }
        if (cursor > total) revert InvalidCursor(cursor, total);
        uint256 end = cursor + limit;
        if (end > total) end = total;
        badges = new Badge[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) badges[i - cursor] = _badges[ids[i]];
        nextCursor = end;
        hasMore = end < total;
    }
}
