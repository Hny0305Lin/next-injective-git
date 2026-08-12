// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

/// @notice Read-only portion of the core registry used by the moderation
/// module.  The module deliberately does not assume that the near-limit core
/// registry has a moderation write hook; it records the effective decision in
/// its own bounded extension state until a reviewed bridge is deployed.
interface IRepoRegistryV2ModerationRegistry {
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

/// @title EVM V2 moderation report and appeal module
/// @notice Stores auditable reports and append-only decisions by stable repoId.
///
/// This is an independently deployable extension.  `setModerationStatus` and
/// report resolutions update the module's effective status, while the core
/// registry remains the source of owner/existence and its original status.
/// A future reviewed bridge may mirror these decisions into core status; this
/// module intentionally never performs a speculative or dual write.
contract RepoRegistryV2ModerationModule {
    uint8 public constant STATUS_ACTIVE = 0;
    uint8 public constant STATUS_FROZEN = 1;
    uint8 public constant STATUS_DELISTED = 2;

    uint8 public constant REPORT_OPEN = 0;
    uint8 public constant REPORT_RESOLVED = 1;
    uint8 public constant REPORT_APPEALED = 2;
    uint8 public constant REPORT_APPEAL_RESOLVED = 3;

    uint8 public constant TRAIL_REPORT_SUBMITTED = 0;
    uint8 public constant TRAIL_REPORT_RESOLVED = 1;
    uint8 public constant TRAIL_APPEALED = 2;
    uint8 public constant TRAIL_APPEAL_RESOLVED = 3;
    uint8 public constant TRAIL_DIRECT_STATUS = 4;

    uint256 public constant MAX_REASON_HASH_LENGTH = 128;
    uint256 public constant MAX_QUERY_PAGE_SIZE = 64;

    IRepoRegistryV2ModerationRegistry public immutable registry;
    address public immutable admin;
    address public moderationCommittee;

    struct Report {
        uint256 id;
        bytes32 repoId;
        address ownerAtReport;
        string repoNameAtReport;
        address reporter;
        string reasonHash;
        uint8 status;
        uint8 resolution;
        bool hasResolution;
        string resolutionHash;
        string appealHash;
        bool hasAppeal;
        uint64 createdAt;
        uint64 updatedAt;
    }

    struct TrailEntry {
        uint8 action;
        address actor;
        uint8 status;
        string reasonHash;
        uint64 timestamp;
    }

    struct StatusOverride {
        uint8 status;
        bool set;
    }

    uint256 public nextReportId = 1;
    mapping(uint256 id => Report report) private _reports;
    mapping(uint256 id => TrailEntry[] entries) private _trails;
    mapping(bytes32 repoId => uint256[] ids) private _repoReportIds;
    mapping(bytes32 repoId => StatusOverride overrideState) private _statusOverrides;

    error InvalidRegistry();
    error InvalidAdmin();
    error InvalidCommittee();
    error Unauthorized(address caller);
    error RepoNotFound(bytes32 repoId);
    error InvalidStatus(uint8 status);
    error InvalidReasonHashLength(uint256 length, uint256 maximum);
    error ReportNotFound(uint256 id);
    error ReportState(uint256 id, uint8 expected, uint8 actual);
    error AppealUnauthorized(address caller, address owner);
    error InvalidPageSize(uint256 requested, uint256 maximum);
    error InvalidCursor(uint256 cursor, uint256 total);
    error TimestampOverflow(uint256 timestamp);

    event ModerationCommitteeUpdated(address indexed committee, address indexed updatedBy);
    event ModerationStatusSet(
        bytes32 indexed repoId,
        uint8 status,
        string reasonHash,
        address indexed moderator
    );
    event ReportSubmitted(
        uint256 indexed reportId,
        bytes32 indexed repoId,
        address indexed reporter,
        address ownerAtReport,
        string repoNameAtReport,
        string reasonHash,
        uint64 timestamp
    );
    event ReportResolved(
        uint256 indexed reportId,
        bytes32 indexed repoId,
        uint8 status,
        string reasonHash,
        address indexed moderator,
        uint64 timestamp
    );
    event ReportAppealed(
        uint256 indexed reportId,
        bytes32 indexed repoId,
        address indexed owner,
        string reasonHash,
        uint64 timestamp
    );
    event AppealResolved(
        uint256 indexed reportId,
        bytes32 indexed repoId,
        uint8 status,
        string reasonHash,
        address indexed moderator,
        uint64 timestamp
    );

    constructor(address registryAddress, address initialAdmin, address initialCommittee) {
        if (registryAddress == address(0) || registryAddress.code.length == 0) {
            revert InvalidRegistry();
        }
        if (initialAdmin == address(0)) revert InvalidAdmin();
        registry = IRepoRegistryV2ModerationRegistry(registryAddress);
        admin = initialAdmin;
        moderationCommittee = initialCommittee;
    }

    modifier onlyAdmin() {
        if (msg.sender != admin) revert Unauthorized(msg.sender);
        _;
    }

    modifier onlyModerator() {
        address moderator = moderationCommittee == address(0) ? admin : moderationCommittee;
        if (msg.sender != moderator) {
            revert Unauthorized(msg.sender);
        }
        _;
    }

    /// @notice Replace the committee.  A zero committee restores the V1
    /// fallback rule in which only the admin moderates.
    function setModerationCommittee(address committee) external onlyAdmin {
        moderationCommittee = committee;
        emit ModerationCommitteeUpdated(committee, msg.sender);
    }

    /// @notice Return the module decision when one exists, otherwise the core
    /// registry's imported/native moderation status.
    function effectiveStatus(bytes32 repoId) external view returns (uint8 status, bool overridden) {
        IRepoRegistryV2ModerationRegistry.Repo memory repository = _repo(repoId);
        StatusOverride memory value = _statusOverrides[repoId];
        if (value.set) return (value.status, true);
        return (repository.moderationStatus, false);
    }

    /// @notice Moderator-only direct status decision, useful for an incident
    /// response that has no report.  This does not mutate the core registry.
    function setModerationStatus(bytes32 repoId, uint8 status, string calldata reasonHash)
        external
        onlyModerator
    {
        _repo(repoId);
        _validateStatus(status);
        _validateOptionalReasonHash(reasonHash);
        _setStatus(repoId, status);
        emit ModerationStatusSet(repoId, status, reasonHash, msg.sender);
    }

    /// @notice Anyone may submit a report for an existing repository.
    function submitReport(bytes32 repoId, string calldata reasonHash)
        external
        returns (uint256 reportId)
    {
        IRepoRegistryV2ModerationRegistry.Repo memory repository = _repo(repoId);
        _validateReasonHash(reasonHash);
        if (block.timestamp > type(uint64).max) revert TimestampOverflow(block.timestamp);
        reportId = nextReportId++;
        uint64 nowTs = uint64(block.timestamp);
        _reports[reportId] = Report({
            id: reportId,
            repoId: repoId,
            ownerAtReport: repository.owner,
            repoNameAtReport: repository.name,
            reporter: msg.sender,
            reasonHash: reasonHash,
            status: REPORT_OPEN,
            resolution: STATUS_ACTIVE,
            hasResolution: false,
            resolutionHash: "",
            appealHash: "",
            hasAppeal: false,
            createdAt: nowTs,
            updatedAt: nowTs
        });
        _repoReportIds[repoId].push(reportId);
        _appendTrail(reportId, TRAIL_REPORT_SUBMITTED, msg.sender, STATUS_ACTIVE, reasonHash);
        emit ReportSubmitted(
            reportId,
            repoId,
            msg.sender,
            repository.owner,
            repository.name,
            reasonHash,
            nowTs
        );
    }

    /// @notice Resolve an open report and record the resulting status.
    function resolveReport(uint256 reportId, uint8 status, string calldata reasonHash)
        external
        onlyModerator
    {
        Report storage report = _report(reportId);
        if (report.status != REPORT_OPEN) {
            revert ReportState(reportId, REPORT_OPEN, report.status);
        }
        _repo(report.repoId);
        _validateStatus(status);
        _validateReasonHash(reasonHash);
        _setStatus(report.repoId, status);
        report.status = REPORT_RESOLVED;
        report.resolution = status;
        report.hasResolution = true;
        report.resolutionHash = reasonHash;
        report.updatedAt = _now64();
        _appendTrail(reportId, TRAIL_REPORT_RESOLVED, msg.sender, status, reasonHash);
        emit ReportResolved(reportId, report.repoId, status, reasonHash, msg.sender, report.updatedAt);
    }

    /// @notice The owner recorded when the report was submitted may appeal,
    /// matching V1's stored-owner authorization.  V2 keeps the report attached
    /// to stable repoId, but does not silently transfer this appeal right.
    function appealReport(uint256 reportId, string calldata reasonHash) external {
        Report storage report = _report(reportId);
        if (report.status != REPORT_RESOLVED) {
            revert ReportState(reportId, REPORT_RESOLVED, report.status);
        }
        _repo(report.repoId);
        if (msg.sender != report.ownerAtReport) {
            revert AppealUnauthorized(msg.sender, report.ownerAtReport);
        }
        _validateReasonHash(reasonHash);
        report.status = REPORT_APPEALED;
        report.appealHash = reasonHash;
        report.hasAppeal = true;
        report.updatedAt = _now64();
        _appendTrail(reportId, TRAIL_APPEALED, msg.sender, report.resolution, reasonHash);
        emit ReportAppealed(reportId, report.repoId, msg.sender, reasonHash, report.updatedAt);
    }

    /// @notice Resolve an appealed report and record the final decision.
    function resolveAppeal(uint256 reportId, uint8 status, string calldata reasonHash)
        external
        onlyModerator
    {
        Report storage report = _report(reportId);
        if (report.status != REPORT_APPEALED) {
            revert ReportState(reportId, REPORT_APPEALED, report.status);
        }
        _repo(report.repoId);
        _validateStatus(status);
        _validateReasonHash(reasonHash);
        _setStatus(report.repoId, status);
        report.status = REPORT_APPEAL_RESOLVED;
        report.resolution = status;
        report.hasResolution = true;
        report.resolutionHash = reasonHash;
        report.updatedAt = _now64();
        _appendTrail(reportId, TRAIL_APPEAL_RESOLVED, msg.sender, status, reasonHash);
        emit AppealResolved(reportId, report.repoId, status, reasonHash, msg.sender, report.updatedAt);
    }

    function getReport(uint256 reportId) external view returns (Report memory) {
        return _report(reportId);
    }

    function getReportTrailPage(uint256 reportId, uint256 cursor, uint256 limit)
        external
        view
        returns (uint256 nextCursor, bool hasMore, TrailEntry[] memory entries)
    {
        _report(reportId);
        return _pageTrail(_trails[reportId], cursor, limit);
    }

    function listReportsByRepoPage(bytes32 repoId, uint256 cursor, uint256 limit)
        external
        view
        returns (uint256 nextCursor, bool hasMore, Report[] memory reports)
    {
        _repo(repoId);
        uint256[] storage ids = _repoReportIds[repoId];
        uint256 total = ids.length;
        _requirePage(cursor, limit, total);
        uint256 end = cursor + limit;
        if (end > total) end = total;
        reports = new Report[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) reports[i - cursor] = _reports[ids[i]];
        nextCursor = end;
        hasMore = end < total;
    }

    function reportCountByRepo(bytes32 repoId) external view returns (uint256) {
        _repo(repoId);
        return _repoReportIds[repoId].length;
    }

    function _setStatus(bytes32 repoId, uint8 status) private {
        _statusOverrides[repoId] = StatusOverride({status: status, set: true});
    }

    function _repo(bytes32 repoId)
        private
        view
        returns (IRepoRegistryV2ModerationRegistry.Repo memory repository)
    {
        try registry.getRepoById(repoId) returns (IRepoRegistryV2ModerationRegistry.Repo memory value) {
            repository = value;
        } catch {
            revert RepoNotFound(repoId);
        }
        if (!repository.exists) revert RepoNotFound(repoId);
    }

    function _report(uint256 reportId) private view returns (Report storage report) {
        report = _reports[reportId];
        if (report.id == 0) revert ReportNotFound(reportId);
    }

    function _appendTrail(
        uint256 reportId,
        uint8 action,
        address actor,
        uint8 status,
        string memory reasonHash
    ) private {
        _trails[reportId].push(
            TrailEntry({
                action: action,
                actor: actor,
                status: status,
                reasonHash: reasonHash,
                timestamp: _now64()
            })
        );
    }

    function _validateStatus(uint8 status) private pure {
        if (status > STATUS_DELISTED) revert InvalidStatus(status);
    }

    function _validateReasonHash(string calldata reasonHash) private pure {
        uint256 length = bytes(reasonHash).length;
        if (length == 0 || length > MAX_REASON_HASH_LENGTH) {
            revert InvalidReasonHashLength(length, MAX_REASON_HASH_LENGTH);
        }
    }

    function _validateOptionalReasonHash(string calldata reasonHash) private pure {
        uint256 length = bytes(reasonHash).length;
        if (length > MAX_REASON_HASH_LENGTH) {
            revert InvalidReasonHashLength(length, MAX_REASON_HASH_LENGTH);
        }
    }

    function _requirePage(uint256 cursor, uint256 limit, uint256 total) private pure {
        if (limit == 0 || limit > MAX_QUERY_PAGE_SIZE) {
            revert InvalidPageSize(limit, MAX_QUERY_PAGE_SIZE);
        }
        if (cursor > total) revert InvalidCursor(cursor, total);
    }

    function _pageTrail(TrailEntry[] storage source, uint256 cursor, uint256 limit)
        private
        view
        returns (uint256 nextCursor, bool hasMore, TrailEntry[] memory entries)
    {
        uint256 total = source.length;
        _requirePage(cursor, limit, total);
        uint256 end = cursor + limit;
        if (end > total) end = total;
        entries = new TrailEntry[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) entries[i - cursor] = source[i];
        nextCursor = end;
        hasMore = end < total;
    }

    function _now64() private view returns (uint64 timestamp) {
        if (block.timestamp > type(uint64).max) revert TimestampOverflow(block.timestamp);
        return uint64(block.timestamp);
    }
}
