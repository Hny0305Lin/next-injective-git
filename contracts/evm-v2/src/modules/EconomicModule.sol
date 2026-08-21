// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {
    IEconomicOwnershipHook,
    IModerationPolicy,
    IRecoveryOwnershipHook,
    IRepositoryCore,
    ISuiteDirectory,
    SuiteIds
} from "../suite/ISuite.sol";
import {SuiteModule} from "../suite/SuiteModule.sol";

contract EconomicModule is SuiteModule, IEconomicOwnershipHook {
    uint16 public constant MAX_PLATFORM_FEE_BPS = 500;
    uint256 public constant MAX_SPLIT_RECIPIENTS = 20;
    uint256 public constant MAX_MESSAGE_LENGTH = 256;
    uint256 public constant MAX_DENOM_LENGTH = 128;

    struct Split {
        address payable recipient;
        uint16 bps;
    }

    struct ImportSplits {
        bytes32 repoId;
        Split[] splits;
    }

    struct ImportTotal {
        bytes32 repoId;
        string denom;
        uint256 amount;
    }

    address public immutable admin;
    address payable public treasury;
    uint16 public platformFeeBps;
    uint256 private _settlementLock = 1;

    mapping(bytes32 repoId => Split[] splits) private _splits;
    mapping(bytes32 repoId => bool importedSplits) private _importedSplits;
    mapping(bytes32 repoId => mapping(bytes32 denomHash => uint256 total)) private _totals;
    mapping(bytes32 repoId => string[] denoms) private _denoms;
    mapping(bytes32 repoId => mapping(bytes32 denomHash => bool known)) private _knownDenom;

    error InvalidAdmin();
    error InvalidTreasury();
    error Unauthorized(address caller);
    error RepositoryNotFound(bytes32 repoId);
    error NoFunds();
    error InvalidMessageLength(uint256 length, uint256 maximum);
    error TooManySplitRecipients(uint256 count, uint256 maximum);
    error InvalidSplitRecipient(address recipient);
    error OwnerCannotReceiveRevenueSplit(address owner);
    error DuplicateSplitRecipient(address recipient);
    error InvalidSplitBps(address recipient, uint256 bps);
    error SplitTotalTooHigh(uint256 total, uint256 maximum);
    error PlatformFeeTooHigh(uint256 bps, uint256 maximum);
    error PayoutFailed(address recipient, uint256 amount);
    error ReentrantSettlement();
    error InvalidDenom(string denom);
    error InvalidImportKind(uint8 kind);
    error InvalidImportRecord();

    event RevenueSplitsUpdated(bytes32 indexed repoId, address indexed owner, uint256 totalBps);
    event SponsorSettled(
        bytes32 indexed repoId,
        address indexed sponsor,
        uint256 amount,
        uint256 platformFee,
        string message
    );
    event FeeConfigUpdated(address indexed treasury, uint16 platformFeeBps, address indexed updatedBy);

    constructor(
        address directory,
        address coordinator,
        address admin_,
        address payable treasury_,
        uint16 platformFeeBps_
    ) SuiteModule(directory, coordinator) {
        if (admin_ == address(0)) revert InvalidAdmin();
        if (treasury_ == address(0)) revert InvalidTreasury();
        if (platformFeeBps_ > MAX_PLATFORM_FEE_BPS) {
            revert PlatformFeeTooHigh(platformFeeBps_, MAX_PLATFORM_FEE_BPS);
        }
        admin = admin_;
        treasury = treasury_;
        platformFeeBps = platformFeeBps_;
    }

    function moduleId() public pure override returns (bytes32) {
        return SuiteIds.ECONOMIC;
    }

    function setRevenueSplits(bytes32 repoId, address payable[] calldata recipients, uint16[] calldata bps)
        external
        onlyActiveSuite
    {
        IRepositoryCore.Repository memory repository = _repository(repoId);
        if (msg.sender != repository.owner) revert Unauthorized(msg.sender);
        if (recipients.length != bps.length || recipients.length > MAX_SPLIT_RECIPIENTS) {
            revert TooManySplitRecipients(recipients.length, MAX_SPLIT_RECIPIENTS);
        }
        delete _splits[repoId];
        uint256 totalBps;
        for (uint256 i; i < recipients.length; ++i) {
            address payable recipient = recipients[i];
            if (recipient == address(0)) revert InvalidSplitRecipient(recipient);
            if (recipient == repository.owner) revert OwnerCannotReceiveRevenueSplit(repository.owner);
            if (bps[i] == 0) revert InvalidSplitBps(recipient, bps[i]);
            for (uint256 j; j < i; ++j) {
                if (recipients[j] == recipient) revert DuplicateSplitRecipient(recipient);
            }
            totalBps += bps[i];
            if (totalBps > 10_000) revert SplitTotalTooHigh(totalBps, 10_000);
            _splits[repoId].push(Split(recipient, bps[i]));
        }
        emit RevenueSplitsUpdated(repoId, repository.owner, totalBps);
    }

    function sponsor(bytes32 repoId, string calldata message) external payable onlyActiveSuite {
        if (_settlementLock != 1) revert ReentrantSettlement();
        if (msg.value == 0) revert NoFunds();
        if (bytes(message).length > MAX_MESSAGE_LENGTH) {
            revert InvalidMessageLength(bytes(message).length, MAX_MESSAGE_LENGTH);
        }
        IRepositoryCore.Repository memory repository = _repository(repoId);
        address moderation = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.MODERATION);
        if (moderation == address(0)) revert SuiteNotActive();
        IModerationPolicy(moderation).requireEconomicAction(repoId, msg.sender);

        _settlementLock = 2;
        _addTotal(repoId, "inj", msg.value);
        uint256 fee = (msg.value * platformFeeBps) / 10_000;
        uint256 distributable = msg.value - fee;
        uint256 distributed;
        if (fee != 0) _pay(treasury, fee);
        Split[] storage splits = _splits[repoId];
        for (uint256 i; i < splits.length; ++i) {
            uint256 amount = (distributable * splits[i].bps) / 10_000;
            distributed += amount;
            if (amount != 0) _pay(splits[i].recipient, amount);
        }
        _pay(payable(repository.owner), distributable - distributed);
        _settlementLock = 1;
        emit SponsorSettled(repoId, msg.sender, msg.value, fee, message);
    }

    function setFeeConfig(address payable newTreasury, uint16 newPlatformFeeBps) external onlyActiveSuite {
        if (msg.sender != admin) revert Unauthorized(msg.sender);
        if (newTreasury == address(0)) revert InvalidTreasury();
        if (newPlatformFeeBps > MAX_PLATFORM_FEE_BPS) {
            revert PlatformFeeTooHigh(newPlatformFeeBps, MAX_PLATFORM_FEE_BPS);
        }
        treasury = newTreasury;
        platformFeeBps = newPlatformFeeBps;
        emit FeeConfigUpdated(newTreasury, newPlatformFeeBps, msg.sender);
    }

    function clearRevenueSplitsOnOwnershipTransfer(bytes32 repoId)
        external
        override
        onlyActiveSuite
    {
        address core = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.CORE);
        if (core == address(0) || msg.sender != core) revert Unauthorized(msg.sender);
        _repository(repoId);
        delete _splits[repoId];
        address recovery = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.RECOVERY);
        if (recovery == address(0)) revert SuiteNotActive();
        IRecoveryOwnershipHook(recovery).cancelRecovery(repoId);
    }

    function revenueSplits(bytes32 repoId) external view returns (Split[] memory) {
        _repository(repoId);
        return _splits[repoId];
    }

    function sponsorTotal(bytes32 repoId, string calldata denom) external view returns (uint256) {
        _repository(repoId);
        return _totals[repoId][keccak256(bytes(denom))];
    }

    function sponsorDenoms(bytes32 repoId) external view returns (string[] memory) {
        _repository(repoId);
        return _denoms[repoId];
    }

    function _importPayload(bytes calldata payload) internal override returns (uint256 count) {
        (uint8 kind, bytes memory records) = abi.decode(payload, (uint8, bytes));
        if (kind == 0) {
            ImportSplits[] memory values = abi.decode(records, (ImportSplits[]));
            count = values.length;
            for (uint256 i; i < count; ++i) _importSplits(values[i]);
        } else if (kind == 1) {
            ImportTotal[] memory values = abi.decode(records, (ImportTotal[]));
            count = values.length;
            for (uint256 i; i < count; ++i) {
                _repository(values[i].repoId);
                if (values[i].amount == 0) revert InvalidImportRecord();
                _validateDenom(values[i].denom);
                bytes32 key = keccak256(bytes(values[i].denom));
                if (_totals[values[i].repoId][key] != 0) revert InvalidImportRecord();
                _addTotal(values[i].repoId, values[i].denom, values[i].amount);
            }
        } else {
            revert InvalidImportKind(kind);
        }
    }

    function _importSplits(ImportSplits memory value) private {
        IRepositoryCore.Repository memory repository = _repository(value.repoId);
        if (_importedSplits[value.repoId] || value.splits.length > MAX_SPLIT_RECIPIENTS) {
            revert InvalidImportRecord();
        }
        _importedSplits[value.repoId] = true;
        if (value.splits.length == 0) return;
        uint256 total;
        for (uint256 i; i < value.splits.length; ++i) {
            if (
                value.splits[i].recipient == address(0) || value.splits[i].recipient == repository.owner
                || value.splits[i].bps == 0
            ) revert InvalidImportRecord();
            for (uint256 j; j < i; ++j) {
                if (value.splits[j].recipient == value.splits[i].recipient) revert InvalidImportRecord();
            }
            total += value.splits[i].bps;
            if (total > 10_000) revert InvalidImportRecord();
            _splits[value.repoId].push(value.splits[i]);
        }
    }

    function _addTotal(bytes32 repoId, string memory denom, uint256 amount) private {
        bytes32 key = keccak256(bytes(denom));
        if (!_knownDenom[repoId][key]) {
            _knownDenom[repoId][key] = true;
            _denoms[repoId].push(denom);
        }
        _totals[repoId][key] += amount;
    }

    function _validateDenom(string memory denom) private pure {
        uint256 length = bytes(denom).length;
        if (length == 0 || length > MAX_DENOM_LENGTH) revert InvalidDenom(denom);
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

    function _pay(address payable recipient, uint256 amount) private {
        (bool ok,) = recipient.call{value: amount}("");
        if (!ok) revert PayoutFailed(recipient, amount);
    }
}
