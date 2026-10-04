// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {SuiteIds} from "./suite/ISuite.sol";
import {SuiteModule} from "./suite/SuiteModule.sol";

contract UsernameModule is SuiteModule {
    uint64 public constant ORIGINAL_OWNER_CLAIM_WINDOW = 90 days;
    uint256 public constant MAX_USERNAME_LENGTH = 32;
    uint256 public constant MAX_RESERVED_USERNAMES = 128;

    struct UsernameRecord {
        address owner;
        uint64 registeredAt;
    }

    struct ImportOriginalOwner {
        string name;
        address owner;
    }

    address public immutable policyAdmin;
    uint64 public originalOwnerClaimDeadline;
    uint256 public reservedUsernameCount;

    mapping(bytes32 nameHash => UsernameRecord record) private _records;
    mapping(bytes32 nameHash => string name) private _names;
    mapping(address owner => string name) private _reverse;
    mapping(bytes32 nameHash => bool reserved) private _reserved;
    mapping(bytes32 nameHash => address owner) private _snapshotOriginalOwner;

    error InvalidPolicyAdmin();
    error Unauthorized(address caller);
    error InvalidUsername(string name);
    error UsernameReserved(string name);
    error UsernameUnavailable(string name);
    error AddressAlreadyNamed(address owner);
    error UsernameNotFound(string name);
    error OriginalOwnerClaimExpired(uint64 deadline);
    error OriginalOwnerMismatch(address expected, address actual);
    error TimestampOverflow(uint256 timestamp);
    error InvalidImportKind(uint8 kind);
    error InvalidImportRecord();
    error TooManyReservedUsernames(uint256 count, uint256 maximum);

    event UsernameRegistered(string indexed name, address indexed owner, bool originalOwnerClaim);
    event UsernameReleased(string indexed name, address indexed owner);
    event UsernameReservationSet(string indexed name, bool reserved, address indexed updatedBy);
    event OriginalOwnerClaimWindowOpened(uint64 indexed deadline);

    constructor(address directory, address coordinator, address policyAdmin_)
        SuiteModule(directory, coordinator)
    {
        if (policyAdmin_ == address(0)) revert InvalidPolicyAdmin();
        policyAdmin = policyAdmin_;
    }

    function moduleId() public pure override returns (bytes32) {
        return SuiteIds.USERNAME;
    }

    function registerUsername(string calldata name) external onlyActiveSuite {
        bytes32 key = _validateAndHash(name);
        if (_reserved[key]) revert UsernameReserved(name);
        if (_records[key].owner != address(0)) revert UsernameUnavailable(name);
        if (bytes(_reverse[msg.sender]).length != 0) revert AddressAlreadyNamed(msg.sender);
        address original = _snapshotOriginalOwner[key];
        if (original != address(0) && block.timestamp <= originalOwnerClaimDeadline && original != msg.sender) {
            revert UsernameUnavailable(name);
        }
        _register(key, name, msg.sender, original == msg.sender);
    }

    function claimOriginalUsername(string calldata name) external onlyActiveSuite {
        bytes32 key = _validateAndHash(name);
        if (block.timestamp > originalOwnerClaimDeadline) {
            revert OriginalOwnerClaimExpired(originalOwnerClaimDeadline);
        }
        address expected = _snapshotOriginalOwner[key];
        if (expected == address(0) || expected != msg.sender) revert OriginalOwnerMismatch(expected, msg.sender);
        if (_reserved[key]) revert UsernameReserved(name);
        if (_records[key].owner != address(0)) revert UsernameUnavailable(name);
        if (bytes(_reverse[msg.sender]).length != 0) revert AddressAlreadyNamed(msg.sender);
        _register(key, name, msg.sender, true);
    }

    function releaseUsername() external onlyActiveSuite {
        string memory name = _reverse[msg.sender];
        if (bytes(name).length == 0) revert UsernameNotFound(name);
        bytes32 key = keccak256(bytes(name));
        delete _records[key];
        delete _names[key];
        delete _reverse[msg.sender];
        emit UsernameReleased(name, msg.sender);
    }

    function setReserved(string calldata name, bool reserved) external onlyActiveSuite {
        if (msg.sender != policyAdmin) revert Unauthorized(msg.sender);
        bytes32 key = _validateAndHash(name);
        if (reserved && _records[key].owner != address(0)) revert UsernameUnavailable(name);
        if (reserved != _reserved[key]) {
            if (reserved) {
                if (reservedUsernameCount >= MAX_RESERVED_USERNAMES) {
                    revert TooManyReservedUsernames(reservedUsernameCount + 1, MAX_RESERVED_USERNAMES);
                }
                ++reservedUsernameCount;
            } else {
                --reservedUsernameCount;
            }
        }
        _reserved[key] = reserved;
        emit UsernameReservationSet(name, reserved, msg.sender);
    }

    function resolveUsername(string calldata name) external view returns (UsernameRecord memory) {
        bytes32 key = keccak256(bytes(name));
        UsernameRecord memory record = _records[key];
        if (record.owner == address(0)) revert UsernameNotFound(name);
        return record;
    }

    function usernameOf(address owner) external view returns (string memory) {
        return _reverse[owner];
    }

    function originalOwnerOf(string calldata name) external view returns (address) {
        return _snapshotOriginalOwner[keccak256(bytes(name))];
    }

    function isReserved(string calldata name) external view returns (bool) {
        return _reserved[keccak256(bytes(name))];
    }

    function _importPayload(bytes calldata payload) internal override returns (uint256 count) {
        (uint8 kind, bytes memory records) = abi.decode(payload, (uint8, bytes));
        if (kind == 0) {
            ImportOriginalOwner[] memory owners = abi.decode(records, (ImportOriginalOwner[]));
            count = owners.length;
            for (uint256 i; i < count; ++i) {
                bytes32 key = _validateAndHash(owners[i].name);
                if (owners[i].owner == address(0) || _snapshotOriginalOwner[key] != address(0)) {
                    revert InvalidImportRecord();
                }
                _snapshotOriginalOwner[key] = owners[i].owner;
            }
        } else if (kind == 1) {
            string[] memory names = abi.decode(records, (string[]));
            count = names.length;
            for (uint256 i; i < count; ++i) {
                bytes32 key = _validateAndHash(names[i]);
                if (_reserved[key] || reservedUsernameCount >= MAX_RESERVED_USERNAMES) revert InvalidImportRecord();
                _reserved[key] = true;
                ++reservedUsernameCount;
            }
        } else {
            revert InvalidImportKind(kind);
        }
    }

    function _beforeBootstrapFinalized() internal override {
        uint256 deadline = block.timestamp + ORIGINAL_OWNER_CLAIM_WINDOW;
        if (deadline > type(uint64).max) revert TimestampOverflow(deadline);
        originalOwnerClaimDeadline = uint64(deadline);
        emit OriginalOwnerClaimWindowOpened(uint64(deadline));
    }

    function _register(bytes32 key, string memory name, address owner, bool originalClaim) private {
        uint64 now64 = _now64();
        _records[key] = UsernameRecord(owner, now64);
        _names[key] = name;
        _reverse[owner] = name;
        emit UsernameRegistered(name, owner, originalClaim);
    }

    function _validateAndHash(string memory name) private pure returns (bytes32) {
        bytes memory raw = bytes(name);
        if (
            raw.length < 3 || raw.length > MAX_USERNAME_LENGTH || raw[0] == "-" || raw[raw.length - 1] == "-"
            || (raw.length >= 4 && raw[0] == "i" && raw[1] == "n" && raw[2] == "j" && raw[3] == "1")
        ) {
            revert InvalidUsername(name);
        }
        for (uint256 i; i < raw.length; ++i) {
            bytes1 c = raw[i];
            if (!((c >= "a" && c <= "z") || (c >= "0" && c <= "9") || c == "-")) {
                revert InvalidUsername(name);
            }
        }
        return keccak256(raw);
    }

    function _now64() private view returns (uint64) {
        if (block.timestamp > type(uint64).max) revert TimestampOverflow(block.timestamp);
        return uint64(block.timestamp);
    }
}
