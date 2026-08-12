package chain

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

// EVMEconomicModule implements native-INJ sponsorship and revenue splits in
// the independently deployable module keyed by stable core registry repo IDs.
type EVMEconomicModule struct {
	cfg      config.Config
	registry *EVMRegistryV2
	rpc      *EVMRPC
	signer   EVMSigner
	address  string
}

var _ EconomicBackend = (*EVMEconomicModule)(nil)

func NewEVMEconomicModule(cfg config.Config) *EVMEconomicModule {
	registry := NewEVMRegistryV2(cfg)
	return NewEVMEconomicModuleWithDependencies(cfg, registry, registry.rpc, registry.signer)
}

func NewEVMEconomicModuleWithDependencies(
	cfg config.Config,
	registry *EVMRegistryV2,
	rpc *EVMRPC,
	signer EVMSigner,
) *EVMEconomicModule {
	if registry == nil {
		registry = NewEVMRegistryV2WithDependencies(cfg, rpc, signer)
	}
	if rpc == nil {
		rpc = registry.rpc
	}
	if signer == nil {
		signer = registry.signer
	}
	return &EVMEconomicModule{
		cfg: cfg, registry: registry, rpc: rpc, signer: signer,
		address: cfg.EffectiveEVMEconomicModuleAddress(),
	}
}

func (m *EVMEconomicModule) contract() (string, error) {
	if strings.TrimSpace(m.address) == "" {
		return "", fmt.Errorf("EVM V2 economic module address is not configured")
	}
	return normalizeEVMAddress(m.address)
}

func (m *EVMEconomicModule) resolve(owner, repo string) (*ResolvedRepo, error) {
	resolved, err := m.registry.ResolveRepo(owner, repo)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

func (m *EVMEconomicModule) Sponsor(owner, repo, message, amount string) error {
	resolved, err := m.resolve(owner, repo)
	if err != nil {
		return err
	}
	if resolved.Backend != BackendEVM || resolved.WriteDisabled {
		return fmt.Errorf("%w: %s", ErrLegacyWriteFallbackDisabled, resolved.CanonicalURL())
	}
	value, err := evmINJValue(amount)
	if err != nil {
		return err
	}
	if len([]byte(message)) > 256 {
		return fmt.Errorf("sponsor message must contain at most 256 UTF-8 bytes")
	}
	data, err := encodeABICall(
		"sponsor(bytes32,string)",
		abiBytes32Value(resolved.RepoID), abiStringValue(message),
	)
	if err != nil {
		return err
	}
	contract, err := m.contract()
	if err != nil {
		return err
	}
	return m.registry.sendToWithValue(context.Background(), contract, data, value)
}

func (m *EVMEconomicModule) SetRevenueSplits(owner, repo string, splits []SplitEntry) error {
	resolved, err := m.resolve(owner, repo)
	if err != nil {
		return err
	}
	if resolved.Backend != BackendEVM || resolved.WriteDisabled {
		return fmt.Errorf("%w: %s", ErrLegacyWriteFallbackDisabled, resolved.CanonicalURL())
	}
	if !resolved.IsCanonical {
		return &RepoMovedError{RepoID: resolved.RepoID, CurrentOwner: resolved.Canonical.Owner, Name: resolved.Canonical.Name}
	}
	if len(splits) > 20 {
		return fmt.Errorf("revenue splits support at most 20 recipients")
	}
	recipients := make([]string, 0, len(splits))
	basisPoints := make([]uint64, 0, len(splits))
	seen := make(map[string]struct{}, len(splits))
	var total uint64
	for i, split := range splits {
		address, err := normalizeEVMAddress(split.Address)
		if err != nil {
			return fmt.Errorf("revenue split %d recipient: %w", i, err)
		}
		key := strings.ToLower(address)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate revenue split recipient %s", split.Address)
		}
		seen[key] = struct{}{}
		if split.Bps == 0 {
			return fmt.Errorf("revenue split %d has zero basis points", i)
		}
		total += uint64(split.Bps)
		if total > 10_000 {
			return fmt.Errorf("revenue split total %d bps exceeds 10000", total)
		}
		recipients = append(recipients, address)
		basisPoints = append(basisPoints, uint64(split.Bps))
	}
	data, err := encodeABICall(
		"setRevenueSplits(bytes32,address[],uint16[])",
		abiBytes32Value(resolved.RepoID),
		abiAddressArrayValue(recipients),
		abiUintArrayValue(basisPoints),
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

func (m *EVMEconomicModule) RevenueSplits(owner, repo string) ([]SplitEntry, error) {
	blockTag, err := m.registry.snapshotBlockTag(context.Background())
	if err != nil {
		return nil, err
	}
	resolved, err := m.registry.resolveRepoAt(owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	if resolved.Backend == BackendCosmWasm {
		return m.registry.legacy.RevenueSplits(resolved.Canonical.Owner, resolved.Canonical.Name)
	}
	data, err := encodeABICall("revenueSplits(bytes32)", abiBytes32Value(resolved.RepoID))
	if err != nil {
		return nil, err
	}
	result, err := m.callAt(data, blockTag)
	if err != nil {
		return nil, err
	}
	return decodeEVMRevenueSplits(result)
}

func (m *EVMEconomicModule) SponsorTotals(owner, repo string) ([]Coin, error) {
	blockTag, err := m.registry.snapshotBlockTag(context.Background())
	if err != nil {
		return nil, err
	}
	resolved, err := m.registry.resolveRepoAt(owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	if resolved.Backend == BackendCosmWasm {
		return m.registry.legacy.SponsorTotals(resolved.Canonical.Owner, resolved.Canonical.Name)
	}
	data, err := encodeABICall("sponsorTotal(bytes32)", abiBytes32Value(resolved.RepoID))
	if err != nil {
		return nil, err
	}
	result, err := m.callAt(data, blockTag)
	if err != nil {
		return nil, err
	}
	word, err := readABIWord(result, 0)
	if err != nil {
		return nil, err
	}
	amount := new(big.Int).SetBytes(word)
	if amount.Sign() == 0 {
		return nil, nil
	}
	return []Coin{{Denom: "inj", Amount: amount.String()}}, nil
}

func (m *EVMEconomicModule) callAt(data []byte, blockTag string) ([]byte, error) {
	contract, err := m.contract()
	if err != nil {
		return nil, err
	}
	result, err := m.rpc.CallContractAt(context.Background(), contract, "0x"+hexEncode(data), blockTag)
	if err != nil {
		return nil, wrapEVMRPCError(err)
	}
	return result, nil
}

func evmINJValue(amount string) (string, error) {
	value := strings.TrimSpace(amount)
	if !strings.HasSuffix(value, "inj") {
		return "", fmt.Errorf("EVM sponsorship amount must use inj base units")
	}
	base := strings.TrimSuffix(value, "inj")
	integer, ok := new(big.Int).SetString(base, 10)
	if !ok || integer.Sign() <= 0 {
		return "", fmt.Errorf("invalid positive INJ base-unit amount %q", amount)
	}
	return "0x" + integer.Text(16), nil
}

func decodeEVMRevenueSplits(data []byte) ([]SplitEntry, error) {
	offset, err := readABIOffset(data, 0, 0)
	if err != nil {
		return nil, err
	}
	count, err := readABIUint(data, offset)
	if err != nil {
		return nil, err
	}
	if count > uint64((len(data)-offset-32)/64) {
		return nil, fmt.Errorf("EVM revenue split length %d exceeds result length", count)
	}
	result := make([]SplitEntry, 0, count)
	for i := uint64(0); i < count; i++ {
		base := offset + 32 + int(i)*64
		word, err := readABIWord(data, base)
		if err != nil {
			return nil, err
		}
		for _, high := range word[:12] {
			if high != 0 {
				return nil, fmt.Errorf("EVM revenue split %d address has non-zero padding", i)
			}
		}
		address, err := userAddressFromEVM("0x" + hex.EncodeToString(word[12:]))
		if err != nil {
			return nil, err
		}
		bps, err := readABIUint(data, base+32)
		if err != nil {
			return nil, err
		}
		if bps == 0 || bps > 10_000 {
			return nil, fmt.Errorf("EVM revenue split %d returned invalid bps %d", i, bps)
		}
		result = append(result, SplitEntry{Address: address, Bps: uint16(bps)})
	}
	return result, nil
}
