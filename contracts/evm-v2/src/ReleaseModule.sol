// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {SuiteIds} from "./suite/ISuite.sol";
import {SuiteModule} from "./suite/SuiteModule.sol";

contract ReleaseModule is SuiteModule {
    uint256 public constant MAX_VERSION_LENGTH = 64;
    uint256 public constant MAX_PLATFORM_LENGTH = 64;
    uint256 public constant MAX_QUERY_PAGE_SIZE = 64;

    struct Artifact {
        string version;
        string platform;
        bytes32 sha256;
        address registeredBy;
        uint64 registeredAt;
        bool exists;
    }

    address public immutable releaseAuthority;
    mapping(bytes32 key => Artifact artifact) private _artifacts;
    mapping(bytes32 versionHash => string[] platforms) private _platformsByVersion;

    error InvalidReleaseAuthority();
    error Unauthorized(address caller);
    error InvalidVersion(string version);
    error InvalidPlatform(string platform);
    error InvalidSHA256();
    error ArtifactAlreadyRegistered(string version, string platform);
    error ArtifactNotFound(string version, string platform);
    error InvalidPageSize(uint256 requested, uint256 maximum);
    error InvalidCursor(uint256 cursor, uint256 total);
    error TimestampOverflow(uint256 timestamp);
    error InvalidImportRecord();

    event ReleaseArtifactRegistered(
        string indexed version,
        string indexed platform,
        bytes32 indexed sha256,
        address registeredBy,
        uint64 registeredAt
    );

    constructor(address directory, address coordinator, address releaseAuthority_)
        SuiteModule(directory, coordinator)
    {
        if (releaseAuthority_ == address(0)) revert InvalidReleaseAuthority();
        releaseAuthority = releaseAuthority_;
    }

    function moduleId() public pure override returns (bytes32) {
        return SuiteIds.RELEASE;
    }

    function registerArtifact(string calldata version, string calldata platform, bytes32 digest)
        external
        onlyActiveSuite
    {
        if (msg.sender != releaseAuthority) revert Unauthorized(msg.sender);
        _validateToken(version, MAX_VERSION_LENGTH, true);
        _validateToken(platform, MAX_PLATFORM_LENGTH, false);
        if (digest == bytes32(0)) revert InvalidSHA256();
        uint64 now64 = _now64();
        _storeArtifact(Artifact(version, platform, digest, msg.sender, now64, true));
        emit ReleaseArtifactRegistered(version, platform, digest, msg.sender, now64);
    }

    function getArtifact(string calldata version, string calldata platform) external view returns (Artifact memory) {
        Artifact memory artifact = _artifacts[_key(version, platform)];
        if (!artifact.exists) revert ArtifactNotFound(version, platform);
        return artifact;
    }

    function listArtifactsPage(string calldata version, uint256 cursor, uint256 limit)
        external
        view
        returns (Artifact[] memory page, uint256 nextCursor)
    {
        string[] storage platforms = _platformsByVersion[keccak256(bytes(version))];
        if (limit == 0 || limit > MAX_QUERY_PAGE_SIZE) revert InvalidPageSize(limit, MAX_QUERY_PAGE_SIZE);
        if (cursor > platforms.length) revert InvalidCursor(cursor, platforms.length);
        uint256 end = cursor + limit;
        if (end > platforms.length) end = platforms.length;
        page = new Artifact[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) {
            page[i - cursor] = _artifacts[_key(version, platforms[i])];
        }
        return (page, end);
    }

    function _importPayload(bytes calldata payload) internal override returns (uint256 count) {
        Artifact[] memory artifacts = abi.decode(payload, (Artifact[]));
        count = artifacts.length;
        for (uint256 i; i < count; ++i) {
            Artifact memory artifact = artifacts[i];
            if (!artifact.exists || artifact.sha256 == bytes32(0) || artifact.registeredBy == address(0) || artifact.registeredAt == 0) {
                revert InvalidImportRecord();
            }
            _validateToken(artifact.version, MAX_VERSION_LENGTH, true);
            _validateToken(artifact.platform, MAX_PLATFORM_LENGTH, false);
            _storeArtifact(artifact);
        }
    }

    function _storeArtifact(Artifact memory artifact) private {
        bytes32 key = _key(artifact.version, artifact.platform);
        if (_artifacts[key].exists) revert ArtifactAlreadyRegistered(artifact.version, artifact.platform);
        _artifacts[key] = artifact;
        _platformsByVersion[keccak256(bytes(artifact.version))].push(artifact.platform);
    }

    function _key(string memory version, string memory platform) private pure returns (bytes32) {
        return keccak256(abi.encode(version, platform));
    }

    function _validateToken(string memory value, uint256 maximum, bool version) private pure {
        bytes memory raw = bytes(value);
        if (raw.length == 0 || raw.length > maximum) {
            if (version) revert InvalidVersion(value);
            revert InvalidPlatform(value);
        }
        for (uint256 i; i < raw.length; ++i) {
            bytes1 c = raw[i];
            bool valid = (c >= "a" && c <= "z") || (c >= "A" && c <= "Z") || (c >= "0" && c <= "9")
                || c == "." || c == "-" || c == "_";
            if (!valid) {
                if (version) revert InvalidVersion(value);
                revert InvalidPlatform(value);
            }
        }
    }

    function _now64() private view returns (uint64) {
        if (block.timestamp > type(uint64).max) revert TimestampOverflow(block.timestamp);
        return uint64(block.timestamp);
    }
}
