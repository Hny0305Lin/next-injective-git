// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

import {RepoRegistryV2} from "../src/RepoRegistryV2.sol";
import {RepoRegistryV2ModerationModule} from "../src/RepoRegistryV2ModerationModule.sol";

interface ModerationVm {
    function prank(address sender) external;
    function expectRevert(bytes calldata revertData) external;
    function expectEmit(bool checkTopic1, bool checkTopic2, bool checkTopic3, bool checkData) external;
    function warp(uint256 newTimestamp) external;
}

abstract contract RepoRegistryV2ModerationModuleTestBase {
    ModerationVm internal constant vm = ModerationVm(address(uint160(uint256(keccak256("hevm cheat code")))));
    RepoRegistryV2 internal registry;
    RepoRegistryV2ModerationModule internal moderation;
    address internal owner = address(0xA11CE);
    address internal reporter = address(0xB0B);
    address internal moderator = address(0xCA701);
    address internal replacementOwner = address(0xD00D);

    event ReportSubmitted(
        uint256 indexed reportId,
        bytes32 indexed repoId,
        address indexed reporter,
        address ownerAtReport,
        string repoNameAtReport,
        string reasonHash,
        uint64 timestamp
    );

    function setUp() public {
        registry = new RepoRegistryV2(address(this));
        moderation = new RepoRegistryV2ModerationModule(address(registry), address(this), moderator);
        vm.prank(owner);
        registry.createRepo("reported", "A reported repository", "main");
    }
}

contract RepoRegistryV2ModerationModuleTrailTest is RepoRegistryV2ModerationModuleTestBase {

    function testReportResolutionAppealAndAppendOnlyTrail() public {
        bytes32 repoId = registry.computeRepoId(owner, "reported");
        vm.warp(100);
        vm.expectEmit(true, true, true, true);
        emit ReportSubmitted(1, repoId, reporter, owner, "reported", "report-hash", 100);
        vm.prank(reporter);
        uint256 reportId = moderation.submitReport(repoId, "report-hash");
        require(reportId == 1, "report id");

        vm.warp(200);
        uint8 frozen = moderation.STATUS_FROZEN();
        vm.prank(moderator);
        moderation.resolveReport(reportId, frozen, "decision-hash");
        (uint8 status, bool overridden) = moderation.effectiveStatus(repoId);
        require(status == moderation.STATUS_FROZEN() && overridden, "frozen override");
        require(registry.getRepoById(repoId).moderationStatus == 0, "core must remain unchanged");
        string[] memory packUris = new string[](1);
        packUris[0] = "ipfs://bafy-moderation-boundary";
        vm.prank(owner);
        registry.updateRef(
            owner,
            "reported",
            "refs/heads/main",
            "1111111111111111111111111111111111111111",
            packUris,
            "",
            false
        );
        require(registry.resolveRef(owner, "reported", "refs/heads/main").exists, "core write boundary");

        vm.warp(300);
        vm.prank(owner);
        moderation.appealReport(reportId, "appeal-hash");

        vm.warp(400);
        uint8 active = moderation.STATUS_ACTIVE();
        vm.prank(moderator);
        moderation.resolveAppeal(reportId, active, "appeal-decision");
        (RepoRegistryV2ModerationModule.Report memory report) = moderation.getReport(reportId);
        require(report.status == moderation.REPORT_APPEAL_RESOLVED(), "appeal resolved");
        require(report.hasResolution && report.hasAppeal, "audit fields");
        require(keccak256(bytes(report.appealHash)) == keccak256(bytes("appeal-hash")), "appeal hash");

        (uint256 next, bool more, RepoRegistryV2ModerationModule.TrailEntry[] memory first) = moderation
            .getReportTrailPage(reportId, 0, 2);
        require(next == 2 && more && first.length == 2, "first trail page");
        require(first[0].action == moderation.TRAIL_REPORT_SUBMITTED(), "submission trail");
        require(first[1].action == moderation.TRAIL_REPORT_RESOLVED(), "resolution trail");
        (next, more, first) = moderation.getReportTrailPage(reportId, next, 2);
        require(next == 4 && !more && first.length == 2, "second trail page");
        require(first[0].action == moderation.TRAIL_APPEALED(), "appeal trail");
        require(first[1].action == moderation.TRAIL_APPEAL_RESOLVED(), "final trail");
    }

}

contract RepoRegistryV2ModerationModulePolicyTest is RepoRegistryV2ModerationModuleTestBase {
    function testCommitteeAdminDirectStatusAndReportPermissions() public {
        bytes32 repoId = registry.computeRepoId(owner, "reported");
        uint8 delisted = moderation.STATUS_DELISTED();
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ModerationModule.Unauthorized.selector, reporter));
        vm.prank(reporter);
        moderation.setModerationStatus(repoId, delisted, "not-moderator");

        vm.prank(moderator);
        moderation.setModerationStatus(repoId, delisted, "committee-decision");
        (uint8 status, bool overridden) = moderation.effectiveStatus(repoId);
        require(status == moderation.STATUS_DELISTED() && overridden, "admin status");

        uint8 active = moderation.STATUS_ACTIVE();
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ModerationModule.Unauthorized.selector, reporter));
        vm.prank(reporter);
        moderation.resolveReport(99, active, "missing");
    }

    function testAppealPreservesV1RecordedOwnerAuthorityAfterStableRepoTransfer() public {
        bytes32 repoId = registry.computeRepoId(owner, "reported");
        vm.prank(reporter);
        uint256 reportId = moderation.submitReport(repoId, "report-hash");
        uint8 frozen = moderation.STATUS_FROZEN();
        vm.prank(moderator);
        moderation.resolveReport(reportId, frozen, "decision-hash");

        vm.prank(owner);
        registry.beginOwnershipTransfer(repoId, replacementOwner);
        RepoRegistryV2.PendingOwnershipTransfer memory pending = registry.pendingOwnershipTransfer(repoId);
        vm.warp(pending.executeAfter);
        vm.prank(replacementOwner);
        registry.acceptOwnership(repoId);

        vm.expectRevert(abi.encodeWithSelector(
            RepoRegistryV2ModerationModule.AppealUnauthorized.selector,
            replacementOwner,
            owner
        ));
        vm.prank(replacementOwner);
        moderation.appealReport(reportId, "current-owner-appeal");
        vm.prank(owner);
        moderation.appealReport(reportId, "recorded-owner-appeal");
        (RepoRegistryV2ModerationModule.Report memory report) = moderation.getReport(reportId);
        require(report.ownerAtReport == owner, "historical owner snapshot");
        require(report.status == moderation.REPORT_APPEALED(), "appealed by recorded owner");
    }

    function testCommitteeConfigurationMatchesV1FallbackSemantics() public {
        bytes32 repoId = registry.computeRepoId(owner, "reported");
        uint8 frozen = moderation.STATUS_FROZEN();
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ModerationModule.Unauthorized.selector, address(this)));
        moderation.setModerationStatus(repoId, frozen, "admin-while-committee");

        moderation.setModerationCommittee(address(0));
        moderation.setModerationStatus(repoId, frozen, "");
        (uint8 status, bool overridden) = moderation.effectiveStatus(repoId);
        require(status == frozen && overridden, "admin fallback status");

        uint8 active = moderation.STATUS_ACTIVE();
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ModerationModule.Unauthorized.selector, moderator));
        vm.prank(moderator);
        moderation.setModerationStatus(repoId, active, "old committee");
    }

    function testRejectsInvalidStatusReasonAndPages() public {
        bytes32 repoId = registry.computeRepoId(owner, "reported");
        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ModerationModule.InvalidStatus.selector, 3));
        vm.prank(moderator);
        moderation.setModerationStatus(repoId, 3, "bad-status");

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ModerationModule.InvalidReasonHashLength.selector, 0, 128));
        vm.prank(reporter);
        moderation.submitReport(repoId, "");

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ModerationModule.InvalidPageSize.selector, 0, 64));
        moderation.listReportsByRepoPage(repoId, 0, 0);

        vm.expectRevert(abi.encodeWithSelector(RepoRegistryV2ModerationModule.ReportNotFound.selector, 7));
        moderation.getReport(7);
    }

    function testRepoPagesAndReportCountAreBounded() public {
        bytes32 repoId = registry.computeRepoId(owner, "reported");
        vm.prank(reporter);
        moderation.submitReport(repoId, "first");
        vm.prank(reporter);
        moderation.submitReport(repoId, "second");
        require(moderation.reportCountByRepo(repoId) == 2, "report count");

        (uint256 next, bool more, RepoRegistryV2ModerationModule.Report[] memory page) = moderation
            .listReportsByRepoPage(repoId, 0, 1);
        require(next == 1 && more && page.length == 1, "first report page");
        require(page[0].reporter == reporter, "reporter");
        (next, more, page) = moderation.listReportsByRepoPage(repoId, next, 1);
        require(next == 2 && !more && page.length == 1, "second report page");
    }
}
