package chain

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

// EVMBadgeModule implements contribution badges in the independently
// deployable module keyed by the core registry's stable repository IDs.
type EVMBadgeModule struct {
	cfg      config.Config
	registry *EVMRegistryV2
	rpc      *EVMRPC
	signer   EVMSigner
	address  string
}

var _ BadgeBackend = (*EVMBadgeModule)(nil)

func NewEVMBadgeModule(cfg config.Config) *EVMBadgeModule {
	registry := NewEVMRegistryV2(cfg)
	return NewEVMBadgeModuleWithDependencies(cfg, registry, registry.rpc, registry.signer)
}

func NewEVMBadgeModuleWithDependencies(
	cfg config.Config,
	registry *EVMRegistryV2,
	rpc *EVMRPC,
	signer EVMSigner,
) *EVMBadgeModule {
	if registry == nil {
		registry = NewEVMRegistryV2WithDependencies(cfg, rpc, signer)
	}
	if rpc == nil {
		rpc = registry.rpc
	}
	if signer == nil {
		signer = registry.signer
	}
	return &EVMBadgeModule{
		cfg: cfg, registry: registry, rpc: rpc, signer: signer,
		address: cfg.EffectiveEVMBadgeModuleAddress(),
	}
}

func (m *EVMBadgeModule) contract() (string, error) {
	if strings.TrimSpace(m.address) == "" {
		return "", fmt.Errorf("EVM V2 badge module address is not configured")
	}
	return normalizeEVMAddress(m.address)
}

func (m *EVMBadgeModule) callAt(ctx context.Context, data []byte, blockTag string) ([]byte, error) {
	contract, err := m.contract()
	if err != nil {
		return nil, err
	}
	result, err := m.rpc.CallContractAt(ctx, contract, "0x"+hexEncode(data), blockTag)
	if err != nil {
		return nil, wrapEVMRPCError(err)
	}
	return result, nil
}

func (m *EVMBadgeModule) AwardBadge(owner, repo, recipient, reason string) error {
	resolved, err := m.registry.ResolveRepo(owner, repo)
	if err != nil {
		return err
	}
	if resolved.Backend != BackendEVM || resolved.WriteDisabled {
		return fmt.Errorf("%w: %s", ErrLegacyWriteFallbackDisabled, resolved.CanonicalURL())
	}
	if !resolved.IsCanonical {
		return &RepoMovedError{RepoID: resolved.RepoID, CurrentOwner: resolved.Canonical.Owner, Name: resolved.Canonical.Name}
	}
	recipientAddress, err := normalizeEVMAddress(recipient)
	if err != nil {
		return fmt.Errorf("invalid badge recipient: %w", err)
	}
	data, err := encodeABICall(
		"awardBadge(bytes32,address,string)",
		abiBytes32Value(resolved.RepoID), abiAddressValue(recipientAddress), abiStringValue(reason),
	)
	if err != nil {
		return err
	}
	contract, err := m.contract()
	if err != nil {
		return err
	}
	return m.registry.sendTo(context.Background(), contract, data)
}

func (m *EVMBadgeModule) BadgesByRecipient(recipient string) ([]Badge, error) {
	address, err := normalizeEVMAddress(recipient)
	if err != nil {
		return nil, err
	}
	blockTag, err := m.snapshotBlockTag(context.Background())
	if err != nil {
		return nil, err
	}
	badges, err := m.drainBadges(
		"listBadgesByRecipientPage(address,uint256,uint256)",
		abiAddressValue(address),
		blockTag,
	)
	if err != nil {
		return nil, err
	}
	if err := m.attachCanonicalRepos(badges, blockTag); err != nil {
		return nil, err
	}

	// Recipient queries have no repository locator whose V2 miss could select
	// the legacy adapter. In auto/v2 compatibility mode, append V1 records
	// after a successful V2 read so old trophy walls remain visible. Explicit
	// evm/v2 mode has no legacy adapter and therefore remains EVM-only.
	if m.registry.legacy != nil {
		legacyRecipient := legacyOwner(recipient, address)
		legacy, legacyErr := m.registry.legacy.BadgesByRecipient(legacyRecipient)
		if legacyErr != nil {
			return nil, legacyErr
		}
		badges = append(badges, legacy...)
	}
	return badges, nil
}

func (m *EVMBadgeModule) BadgesByRepo(owner, repo string) ([]Badge, error) {
	blockTag, err := m.snapshotBlockTag(context.Background())
	if err != nil {
		return nil, err
	}
	resolved, err := m.registry.resolveRepoAt(owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	if resolved.Backend == BackendCosmWasm {
		return m.registry.legacy.BadgesByRepo(resolved.Canonical.Owner, resolved.Canonical.Name)
	}
	badges, err := m.drainBadges(
		"listBadgesByRepoPage(bytes32,uint256,uint256)",
		abiBytes32Value(resolved.RepoID),
		blockTag,
	)
	if err != nil {
		return nil, err
	}
	for i := range badges {
		badges[i].RepoOwner = resolved.Canonical.Owner
		badges[i].RepoName = resolved.Canonical.Name
	}
	return badges, nil
}

func (m *EVMBadgeModule) snapshotBlockTag(ctx context.Context) (string, error) {
	if _, err := m.contract(); err != nil {
		return "", err
	}
	if _, err := m.registry.contract(); err != nil {
		return "", err
	}
	blockNumber, err := m.rpc.BlockNumber(ctx)
	if err != nil {
		return "", wrapEVMRPCError(err)
	}
	return fmt.Sprintf("0x%x", blockNumber), nil
}

func (m *EVMBadgeModule) attachCanonicalRepos(badges []Badge, blockTag string) error {
	repositories := make(map[[32]byte]*RepoInfo)
	for i := range badges {
		repoID, err := badgeRepoID(badges[i])
		if err != nil {
			return fmt.Errorf("EVM badge %d: %w", badges[i].ID, err)
		}
		info, ok := repositories[repoID]
		if !ok {
			info, err = m.registry.repoInfoByIDAt(repoID, blockTag)
			if err != nil {
				return fmt.Errorf("resolve repository for EVM badge %d: %w", badges[i].ID, err)
			}
			repositories[repoID] = info
		}
		badges[i].RepoOwner = info.Owner
		badges[i].RepoName = info.Name
	}
	return nil
}

func badgeRepoID(badge Badge) ([32]byte, error) {
	raw, err := decodeHexBytes(badge.RepoID)
	if err != nil {
		return [32]byte{}, fmt.Errorf("invalid repo ID: %w", err)
	}
	if len(raw) != 32 {
		return [32]byte{}, fmt.Errorf("invalid repo ID length %d", len(raw))
	}
	var repoID [32]byte
	copy(repoID[:], raw)
	return repoID, nil
}

func (m *EVMBadgeModule) drainBadges(signature string, subject evmABIValue, blockTag string) ([]Badge, error) {
	badges := make([]Badge, 0)
	var cursor uint64
	for {
		data, err := encodeABICall(signature, subject, abiUintValue(cursor), abiUintValue(evmQueryPageSize))
		if err != nil {
			return nil, err
		}
		result, err := m.callAt(context.Background(), data, blockTag)
		if err != nil {
			return nil, err
		}
		next, more, page, err := decodeEVMBadgePage(result)
		if err != nil {
			return nil, err
		}
		badges = append(badges, page...)
		if !more {
			return badges, nil
		}
		if next <= cursor {
			return nil, fmt.Errorf("EVM badge page cursor did not advance: %d", cursor)
		}
		cursor = next
	}
}

func decodeEVMBadgePage(data []byte) (uint64, bool, []Badge, error) {
	next, err := readABIUint(data, 0)
	if err != nil {
		return 0, false, nil, err
	}
	moreWord, err := readABIWord(data, 32)
	if err != nil {
		return 0, false, nil, err
	}
	for _, value := range moreWord[:31] {
		if value != 0 {
			return 0, false, nil, fmt.Errorf("EVM badge page returned malformed hasMore flag")
		}
	}
	if moreWord[31] > 1 {
		return 0, false, nil, fmt.Errorf("EVM badge page returned invalid hasMore flag %d", moreWord[31])
	}
	valuesOffset, err := readABIOffset(data, 64, 0)
	if err != nil {
		return 0, false, nil, err
	}
	count, err := readABIUint(data, valuesOffset)
	if err != nil {
		return 0, false, nil, err
	}
	if count > uint64((len(data)-valuesOffset-32)/32) {
		return 0, false, nil, fmt.Errorf("EVM badge page length %d exceeds result length", count)
	}
	badges := make([]Badge, 0, count)
	for i := uint64(0); i < count; i++ {
		base, err := readABIOffset(data, valuesOffset+32+int(i)*32, valuesOffset+32)
		if err != nil {
			return 0, false, nil, err
		}
		badge, err := decodeEVMBadgeAt(data, base)
		if err != nil {
			return 0, false, nil, err
		}
		badges = append(badges, badge)
	}
	return next, moreWord[31] == 1, badges, nil
}

func decodeEVMBadgeAt(data []byte, base int) (Badge, error) {
	id, err := readABIUint(data, base)
	if err != nil || id == 0 {
		if err == nil {
			err = fmt.Errorf("EVM badge returned zero ID")
		}
		return Badge{}, err
	}
	repoID, err := readABIBytes32(data, base+32)
	if err != nil {
		return Badge{}, err
	}
	recipient, err := readABIAddress(data, base+64)
	if err != nil {
		return Badge{}, err
	}
	reasonOffset, err := readABIOffset(data, base+96, base)
	if err != nil {
		return Badge{}, err
	}
	reason, err := readABIString(data, reasonOffset)
	if err != nil {
		return Badge{}, err
	}
	awardedBy, err := readABIAddress(data, base+128)
	if err != nil {
		return Badge{}, err
	}
	awardedAt, err := readABIUint(data, base+160)
	if err != nil {
		return Badge{}, err
	}
	return Badge{
		ID: id, RepoID: "0x" + hex.EncodeToString(repoID[:]), Recipient: recipient,
		Reason: reason, AwardedBy: awardedBy, AwardedAt: awardedAt,
	}, nil
}
