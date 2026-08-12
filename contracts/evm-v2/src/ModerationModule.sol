// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {IModerationPolicy, IRepositoryCore, ISuiteDirectory, SuiteIds} from "./suite/ISuite.sol";
import {SuiteModule} from "./suite/SuiteModule.sol";

contract ModerationModule is SuiteModule, IModerationPolicy {
    uint256 public constant MAX_REASON_LENGTH = 128;
    uint256 public constant MAX_QUERY_PAGE_SIZE = 64;

    enum RepoStatus {
        Active,
        Frozen,
        Delisted
    }

    enum ReportStatus {
        Open,
        Resolved,
        Appealed,
        AppealResolved
    }

    enum TrailAction {
        Submitted,
        Resolved,
        Appealed,
        AppealResolved,
        StatusSet
    }

    struct Report {
        uint256 id;
        bytes32 repoId;
        address reporter;
        ReportStatus status;
        RepoStatus resolution;
        string reasonHash;
        uint64 createdAt;
        uint64 updatedAt;
        bool exists;
    }

    struct TrailEntry {
        TrailAction action;
        address actor;
        RepoStatus status;
        string reasonHash;
        uint64 timestamp;
    }

    struct ImportStatus {
        bytes32 repoId;
        RepoStatus status;
    }

    struct ImportReport {
        Report report;
        TrailEntry[] trail;
    }

    struct StatusTrailEntry {
        RepoStatus status;
        address actor;
        string reasonHash;
        uint64 timestamp;
        uint256 reportId;
    }

    struct ImportStatusTrail {
        bytes32 repoId;
        StatusTrailEntry[] trail;
    }

    address public immutable admin;
    address public committee;
    uint256 public nextReportId = 1;

    mapping(bytes32 repoId => RepoStatus status) private _statuses;
    mapping(uint256 id => Report report) private _reports;
    mapping(uint256 id => TrailEntry[] trail) private _trails;
    mapping(bytes32 repoId => uint256[] reportIds) private _repoReportIds;
    mapping(bytes32 repoId => StatusTrailEntry[] trail) private _statusTrails;

    error InvalidAdmin();
    error InvalidCommittee();
    error Unauthorized(address caller);
    error RepositoryNotFound(bytes32 repoId);
    error RepositoryFrozen(bytes32 repoId);
    error InvalidReasonHash(string reasonHash);
    error ReportNotFound(uint256 reportId);
    error InvalidReportState(uint256 reportId, uint8 expected, uint8 actual);
    error InvalidPageSize(uint256 requested, uint256 maximum);
    error InvalidCursor(uint256 cursor, uint256 total);
    error TimestampOverflow(uint256 timestamp);
    error InvalidImportKind(uint8 kind);
    error InvalidImportRecord();

    event CommitteeUpdated(address indexed committee, address indexed updatedBy);
    event RepositoryStatusSet(bytes32 indexed repoId, RepoStatus status, address indexed updatedBy, string reasonHash);
    event ReportSubmitted(uint256 indexed reportId, bytes32 indexed repoId, address indexed reporter, string reasonHash);
    event ReportResolved(uint256 indexed reportId, RepoStatus status, address indexed resolvedBy, string reasonHash);
    event ReportAppealed(uint256 indexed reportId, address indexed owner, string reasonHash);
    event AppealResolved(uint256 indexed reportId, RepoStatus status, address indexed resolvedBy, string reasonHash);

    constructor(address directory, address coordinator, address admin_, address committee_)
        SuiteModule(directory, coordinator)
    {
        if (admin_ == address(0)) revert InvalidAdmin();
        if (committee_ == address(0)) revert InvalidCommittee();
        admin = admin_;
        committee = committee_;
    }

    modifier onlyCommittee() {
        if (msg.sender != committee) revert Unauthorized(msg.sender);
        _;
    }

    function moduleId() public pure override returns (bytes32) {
        return SuiteIds.MODERATION;
    }

    function setCommittee(address newCommittee) external onlyActiveSuite {
        if (msg.sender != admin) revert Unauthorized(msg.sender);
        if (newCommittee == address(0)) revert InvalidCommittee();
        committee = newCommittee;
        emit CommitteeUpdated(newCommittee, msg.sender);
    }

    function setRepositoryStatus(bytes32 repoId, RepoStatus status, string calldata reasonHash)
        external
        onlyActiveSuite
        onlyCommittee
    {
        _repository(repoId);
        _validateOptionalReason(reasonHash);
        _statuses[repoId] = status;
        _statusTrails[repoId].push(StatusTrailEntry(status, msg.sender, reasonHash, _now64(), 0));
        emit RepositoryStatusSet(repoId, status, msg.sender, reasonHash);
    }

    function submitReport(bytes32 repoId, string calldata reasonHash)
        external
        onlyActiveSuite
        returns (uint256 reportId)
    {
        _repository(repoId);
        _validateReason(reasonHash);
        reportId = nextReportId++;
        uint64 now64 = _now64();
        _reports[reportId] = Report(
            reportId, repoId, msg.sender, ReportStatus.Open, RepoStatus.Active, reasonHash, now64, now64, true
        );
        _repoReportIds[repoId].push(reportId);
        _trails[reportId].push(TrailEntry(TrailAction.Submitted, msg.sender, RepoStatus.Active, reasonHash, now64));
        emit ReportSubmitted(reportId, repoId, msg.sender, reasonHash);
    }

    function resolveReport(uint256 reportId, RepoStatus status, string calldata reasonHash)
        external
        onlyActiveSuite
        onlyCommittee
    {
        Report storage report = _report(reportId);
        if (report.status != ReportStatus.Open) {
            revert InvalidReportState(reportId, uint8(ReportStatus.Open), uint8(report.status));
        }
        _validateReason(reasonHash);
        report.status = ReportStatus.Resolved;
        report.resolution = status;
        report.reasonHash = reasonHash;
        report.updatedAt = _now64();
        _statuses[report.repoId] = status;
        _trails[reportId].push(TrailEntry(TrailAction.Resolved, msg.sender, status, reasonHash, report.updatedAt));
        _statusTrails[report.repoId].push(StatusTrailEntry(status, msg.sender, reasonHash, report.updatedAt, reportId));
        emit ReportResolved(reportId, status, msg.sender, reasonHash);
    }

    function appealReport(uint256 reportId, string calldata reasonHash) external onlyActiveSuite {
        Report storage report = _report(reportId);
        if (report.status != ReportStatus.Resolved) {
            revert InvalidReportState(reportId, uint8(ReportStatus.Resolved), uint8(report.status));
        }
        if (msg.sender != _repository(report.repoId).owner) revert Unauthorized(msg.sender);
        _validateReason(reasonHash);
        report.status = ReportStatus.Appealed;
        report.reasonHash = reasonHash;
        report.updatedAt = _now64();
        _trails[reportId].push(
            TrailEntry(TrailAction.Appealed, msg.sender, report.resolution, reasonHash, report.updatedAt)
        );
        emit ReportAppealed(reportId, msg.sender, reasonHash);
    }

    function resolveAppeal(uint256 reportId, RepoStatus status, string calldata reasonHash)
        external
        onlyActiveSuite
        onlyCommittee
    {
        Report storage report = _report(reportId);
        if (report.status != ReportStatus.Appealed) {
            revert InvalidReportState(reportId, uint8(ReportStatus.Appealed), uint8(report.status));
        }
        _validateReason(reasonHash);
        report.status = ReportStatus.AppealResolved;
        report.resolution = status;
        report.reasonHash = reasonHash;
        report.updatedAt = _now64();
        _statuses[report.repoId] = status;
        _trails[reportId].push(
            TrailEntry(TrailAction.AppealResolved, msg.sender, status, reasonHash, report.updatedAt)
        );
        _statusTrails[report.repoId].push(StatusTrailEntry(status, msg.sender, reasonHash, report.updatedAt, reportId));
        emit AppealResolved(reportId, status, msg.sender, reasonHash);
    }

    function requireRefMutation(bytes32 repoId, address) external view override {
        _repository(repoId);
        if (_statuses[repoId] == RepoStatus.Frozen) revert RepositoryFrozen(repoId);
    }

    function requireEconomicAction(bytes32 repoId, address) external view override {
        _repository(repoId);
        if (_statuses[repoId] == RepoStatus.Frozen) revert RepositoryFrozen(repoId);
    }

    function effectiveStatus(bytes32 repoId) external view returns (RepoStatus) {
        _repository(repoId);
        return _statuses[repoId];
    }

    function getReport(uint256 reportId) external view returns (Report memory) {
        return _report(reportId);
    }

    function listReportTrailPage(uint256 reportId, uint256 cursor, uint256 limit)
        external
        view
        returns (TrailEntry[] memory page, uint256 nextCursor)
    {
        _report(reportId);
        TrailEntry[] storage source = _trails[reportId];
        (uint256 start, uint256 end) = _page(cursor, limit, source.length);
        page = new TrailEntry[](end - start);
        for (uint256 i = start; i < end; ++i) page[i - start] = source[i];
        return (page, end);
    }

    function listReportsByRepositoryPage(bytes32 repoId, uint256 cursor, uint256 limit)
        external
        view
        returns (Report[] memory page, uint256 nextCursor)
    {
        _repository(repoId);
        uint256[] storage ids = _repoReportIds[repoId];
        (uint256 start, uint256 end) = _page(cursor, limit, ids.length);
        page = new Report[](end - start);
        for (uint256 i = start; i < end; ++i) page[i - start] = _reports[ids[i]];
        return (page, end);
    }

    function listRepositoryStatusTrailPage(bytes32 repoId, uint256 cursor, uint256 limit)
        external
        view
        returns (StatusTrailEntry[] memory page, uint256 nextCursor)
    {
        _repository(repoId);
        StatusTrailEntry[] storage source = _statusTrails[repoId];
        (uint256 start, uint256 end) = _page(cursor, limit, source.length);
        page = new StatusTrailEntry[](end - start);
        for (uint256 i = start; i < end; ++i) page[i - start] = source[i];
        return (page, end);
    }

    function _importPayload(bytes calldata payload) internal override returns (uint256 count) {
        (uint8 kind, bytes memory records) = abi.decode(payload, (uint8, bytes));
        if (kind == 0) {
            ImportStatus[] memory statuses = abi.decode(records, (ImportStatus[]));
            count = statuses.length;
            for (uint256 i; i < count; ++i) {
                _repository(statuses[i].repoId);
                _statuses[statuses[i].repoId] = statuses[i].status;
            }
        } else if (kind == 1) {
            ImportReport[] memory reports = abi.decode(records, (ImportReport[]));
            count = reports.length;
            for (uint256 i; i < count; ++i) _storeImportedReport(reports[i]);
        } else if (kind == 2) {
            ImportStatusTrail[] memory trails = abi.decode(records, (ImportStatusTrail[]));
            count = trails.length;
            for (uint256 i; i < count; ++i) _storeImportedStatusTrail(trails[i]);
        } else {
            revert InvalidImportKind(kind);
        }
    }

    function _storeImportedReport(ImportReport memory imported) private {
        Report memory report = imported.report;
        if (
            report.id == 0 || report.id >= type(uint64).max || !report.exists || _reports[report.id].exists
                || report.createdAt == 0 || report.updatedAt < report.createdAt || imported.trail.length == 0
        ) revert InvalidImportRecord();
        _repository(report.repoId);
        _reports[report.id] = report;
        _repoReportIds[report.repoId].push(report.id);
        for (uint256 i; i < imported.trail.length; ++i) {
            if (imported.trail[i].timestamp == 0) revert InvalidImportRecord();
            _trails[report.id].push(imported.trail[i]);
        }
        if (report.id >= nextReportId) nextReportId = report.id + 1;
    }

    function _storeImportedStatusTrail(ImportStatusTrail memory imported) private {
        _repository(imported.repoId);
        if (imported.trail.length == 0 || _statusTrails[imported.repoId].length != 0) {
            revert InvalidImportRecord();
        }
        uint64 previousTimestamp;
        for (uint256 i; i < imported.trail.length; ++i) {
            StatusTrailEntry memory entry = imported.trail[i];
            if (entry.actor == address(0) || entry.timestamp == 0 || entry.timestamp < previousTimestamp) {
                revert InvalidImportRecord();
            }
            _validateOptionalReason(entry.reasonHash);
            if (entry.reportId != 0) {
                Report memory report = _reports[entry.reportId];
                if (!report.exists || report.repoId != imported.repoId) revert InvalidImportRecord();
            }
            _statusTrails[imported.repoId].push(entry);
            previousTimestamp = entry.timestamp;
        }
        if (imported.trail[imported.trail.length - 1].status != _statuses[imported.repoId]) {
            revert InvalidImportRecord();
        }
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

    function _report(uint256 reportId) private view returns (Report storage report) {
        report = _reports[reportId];
        if (!report.exists) revert ReportNotFound(reportId);
    }

    function _validateReason(string memory reasonHash) private pure {
        if (bytes(reasonHash).length == 0 || bytes(reasonHash).length > MAX_REASON_LENGTH) {
            revert InvalidReasonHash(reasonHash);
        }
    }

    function _validateOptionalReason(string memory reasonHash) private pure {
        if (bytes(reasonHash).length > MAX_REASON_LENGTH) revert InvalidReasonHash(reasonHash);
    }

    function _page(uint256 cursor, uint256 limit, uint256 total) private pure returns (uint256, uint256) {
        if (limit == 0 || limit > MAX_QUERY_PAGE_SIZE) revert InvalidPageSize(limit, MAX_QUERY_PAGE_SIZE);
        if (cursor > total) revert InvalidCursor(cursor, total);
        uint256 end = cursor + limit;
        return (cursor, end > total ? total : end);
    }

    function _now64() private view returns (uint64) {
        if (block.timestamp > type(uint64).max) revert TimestampOverflow(block.timestamp);
        return uint64(block.timestamp);
    }
}
