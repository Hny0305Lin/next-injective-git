// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {IModerationPolicy, IRepositoryCore, ISuiteDirectory, SuiteIds} from "./suite/ISuite.sol";
import {SuiteModule} from "./suite/SuiteModule.sol";

contract BadgeModule is SuiteModule {
    uint256 public constant MAX_REASON_LENGTH = 256;
    uint256 public constant MAX_QUERY_PAGE_SIZE = 64;

    struct Badge {
        uint256 id;
        bytes32 repoId;
        address recipient;
        address awardedBy;
        string reason;
        uint64 awardedAt;
        bool exists;
    }

    uint256 public nextBadgeId = 1;
    mapping(uint256 id => Badge badge) private _badges;
    mapping(address recipient => uint256[] ids) private _recipientBadgeIds;
    mapping(bytes32 repoId => uint256[] ids) private _repoBadgeIds;

    error RepositoryNotFound(bytes32 repoId);
    error Unauthorized(address caller);
    error InvalidRecipient(address recipient);
    error OwnerCannotReceiveBadge(address owner);
    error InvalidReasonLength(uint256 length, uint256 maximum);
    error BadgeNotFound(uint256 badgeId);
    error InvalidPageSize(uint256 requested, uint256 maximum);
    error InvalidCursor(uint256 cursor, uint256 total);
    error TimestampOverflow(uint256 timestamp);
    error InvalidImportRecord();

    event BadgeAwarded(
        uint256 indexed badgeId,
        bytes32 indexed repoId,
        address indexed recipient,
        address awardedBy,
        string reason
    );

    constructor(address directory, address coordinator) SuiteModule(directory, coordinator) {}

    function moduleId() public pure override returns (bytes32) {
        return SuiteIds.BADGE;
    }

    function awardBadge(bytes32 repoId, address recipient, string calldata reason)
        external
        onlyActiveSuite
        returns (uint256 badgeId)
    {
        IRepositoryCore.Repository memory repository = _repository(repoId);
        if (msg.sender != repository.owner) revert Unauthorized(msg.sender);
        IModerationPolicy(_moderation()).requireBadgeAward(repoId, msg.sender);
        if (recipient == address(0)) revert InvalidRecipient(recipient);
        if (recipient == repository.owner) revert OwnerCannotReceiveBadge(repository.owner);
        uint256 length = bytes(reason).length;
        if (length == 0 || length > MAX_REASON_LENGTH) revert InvalidReasonLength(length, MAX_REASON_LENGTH);
        badgeId = nextBadgeId++;
        Badge memory badge = Badge(badgeId, repoId, recipient, msg.sender, reason, _now64(), true);
        _storeBadge(badge);
        emit BadgeAwarded(badgeId, repoId, recipient, msg.sender, reason);
    }

    function getBadge(uint256 badgeId) external view returns (Badge memory) {
        Badge memory badge = _badges[badgeId];
        if (!badge.exists) revert BadgeNotFound(badgeId);
        return badge;
    }

    function listBadgesByRecipientPage(address recipient, uint256 cursor, uint256 limit)
        external
        view
        returns (Badge[] memory page, uint256 nextCursor)
    {
        return _page(_recipientBadgeIds[recipient], cursor, limit);
    }

    function listBadgesByRepositoryPage(bytes32 repoId, uint256 cursor, uint256 limit)
        external
        view
        returns (Badge[] memory page, uint256 nextCursor)
    {
        _repository(repoId);
        return _page(_repoBadgeIds[repoId], cursor, limit);
    }

    function _importPayload(bytes calldata payload) internal override returns (uint256 count) {
        Badge[] memory badges = abi.decode(payload, (Badge[]));
        count = badges.length;
        for (uint256 i; i < count; ++i) {
            Badge memory badge = badges[i];
            if (
                badge.id == 0 || !badge.exists || badge.recipient == address(0) || badge.awardedBy == address(0)
                    || badge.id == type(uint256).max || badge.awardedAt == 0 || _badges[badge.id].exists
                    || badge.recipient == badge.awardedBy
            ) revert InvalidImportRecord();
            _repository(badge.repoId);
            uint256 length = bytes(badge.reason).length;
            if (length == 0 || length > MAX_REASON_LENGTH) revert InvalidImportRecord();
            _storeBadge(badge);
            if (badge.id >= nextBadgeId) nextBadgeId = badge.id + 1;
        }
    }

    function _storeBadge(Badge memory badge) private {
        _badges[badge.id] = badge;
        _recipientBadgeIds[badge.recipient].push(badge.id);
        _repoBadgeIds[badge.repoId].push(badge.id);
    }

    function _page(uint256[] storage ids, uint256 cursor, uint256 limit)
        private
        view
        returns (Badge[] memory page, uint256 nextCursor)
    {
        if (limit == 0 || limit > MAX_QUERY_PAGE_SIZE) revert InvalidPageSize(limit, MAX_QUERY_PAGE_SIZE);
        if (cursor > ids.length) revert InvalidCursor(cursor, ids.length);
        uint256 end = cursor + limit;
        if (end > ids.length) end = ids.length;
        page = new Badge[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) page[i - cursor] = _badges[ids[i]];
        return (page, end);
    }

    function _repository(bytes32 repoId) private view returns (IRepositoryCore.Repository memory repository) {
        address core = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.CORE);
        if (core == address(0)) revert SuiteNotActive();
        try IRepositoryCore(core).getRepository(repoId) returns (IRepositoryCore.Repository memory value) {
            return value;
        } catch {
            revert RepositoryNotFound(repoId);
        }
    }

    function _moderation() private view returns (address module) {
        module = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.MODERATION);
        if (module == address(0)) revert SuiteNotActive();
    }

    function _now64() private view returns (uint64) {
        if (block.timestamp > type(uint64).max) revert TimestampOverflow(block.timestamp);
        return uint64(block.timestamp);
    }
}
