package chain

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain/successor"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// SuccessorRegistry is the runtime client for a verified storage-neutral
// successor suite (suiteVersion 4). It reuses the pinned VerifySuite trust
// chain, the checked-in successor ABI and the EVMTransactor semantics; the
// legacy v3 EVMSuiteRegistry is never loaded for these calls.
type SuccessorRegistry struct {
	rpc        *EVMRPC
	transactor *EVMTransactor
	info       *SuiteInfo
	client     *successor.Client
	core       string
	chainID    uint64
}

// NewSuccessorRegistryReadOnly verifies a successor suite and returns a
// read-only registry (clone/fetch/list need no signer).
func NewSuccessorRegistryReadOnly(cfg config.Config) (*SuccessorRegistry, error) {
	return newSuccessorRegistry(cfg, nil)
}

// NewSuccessorRegistry verifies a successor suite and returns a registry that
// can also broadcast ref transactions through the standard EVMTransactor path.
func NewSuccessorRegistry(cfg config.Config) (*SuccessorRegistry, error) {
	rpc := NewEVMRPC(cfg.EffectiveEVMRPC())
	signer := NewEVMKeystoreSigner(cfg)
	return newSuccessorRegistryWithRPC(cfg, rpc, signer)
}

func newSuccessorRegistry(cfg config.Config, rpc *EVMRPC) (*SuccessorRegistry, error) {
	if rpc == nil {
		rpc = NewEVMRPC(cfg.EffectiveEVMRPC())
	}
	return newSuccessorRegistryWithRPC(cfg, rpc, nil)
}

// NewSuccessorRegistryWithDependencies is the test seam: rpc and signer are
// injected, exactly like NewEVMSuiteRegistryWithDependencies.
func NewSuccessorRegistryWithDependencies(cfg config.Config, rpc *EVMRPC, signer EVMSigner) (*SuccessorRegistry, error) {
	if rpc == nil {
		rpc = NewEVMRPC(cfg.EffectiveEVMRPC())
	}
	return newSuccessorRegistryWithRPC(cfg, rpc, signer)
}

func newSuccessorRegistryWithRPC(cfg config.Config, rpc *EVMRPC, signer EVMSigner) (*SuccessorRegistry, error) {
	directory := strings.TrimSpace(cfg.EffectiveEVMSuiteDirectoryAddress())
	if directory == "" {
		return nil, errors.New("EVM SuiteDirectory address is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := VerifySuccessorSuite(ctx, rpc, directory, cfg.EffectiveEVMChainID())
	if err != nil {
		return nil, err
	}
	core := info.ModuleAddress("core")
	if core == "" {
		return nil, errors.New("verified successor suite has no core module")
	}
	registry := &SuccessorRegistry{rpc: rpc, info: info, core: core, chainID: info.ChainID}
	if signer != nil {
		registry.transactor = NewEVMTransactor(cfg, rpc, signer)
	}
	backend := &successorEVMBackend{rpc: rpc, transactor: registry.transactor, core: core}
	client, err := successor.NewClient(backend)
	if err != nil {
		return nil, err
	}
	registry.client = client
	return registry, nil
}

// timeoutDuration keeps the timeout literal in nanoseconds readable above.

// SuiteInfo exposes the verified successor binding view.
func (s *SuccessorRegistry) SuiteInfo() *SuiteInfo { return s.info }

// ChainIDDecimal returns the verified chain ID as a decimal string (manifest
// context form).
func (s *SuccessorRegistry) ChainIDDecimal() string { return fmt.Sprintf("%d", s.chainID) }

// DirectoryHex returns the verified SuiteDirectory in manifest context form.
func (s *SuccessorRegistry) DirectoryHex() string {
	return strings.ToLower(s.info.Directory)
}

// GetRef reads the full commitment state of one ref through the successor ABI.
func (s *SuccessorRegistry) GetRef(ctx context.Context, repoID [32]byte, refName string) (successor.RefState, error) {
	return s.client.GetRef(ctx, repoID, refName)
}

// UpdateRef broadcasts a CAS commitment publish and returns the tx hash.
func (s *SuccessorRegistry) UpdateRef(
	ctx context.Context,
	repoID [32]byte,
	refName, commitSHA string,
	commitment successor.Commitment,
	expectedRevision uint64,
	expectedDigest [32]byte,
	force bool,
) (string, error) {
	if s.transactor == nil {
		return "", ErrEVMSignerUnavailable
	}
	return s.client.UpdateRef(ctx, repoID, refName, commitSHA, commitment, expectedRevision, expectedDigest, force)
}

// DeleteRef broadcasts the tombstoning delete of one ref.
func (s *SuccessorRegistry) DeleteRef(ctx context.Context, repoID [32]byte, refName string) (string, error) {
	if s.transactor == nil {
		return "", ErrEVMSignerUnavailable
	}
	return s.client.DeleteRef(ctx, repoID, refName)
}

// ResolveRepository mirrors the v3 resolveRepository flow over the successor
// ABI (repository metadata is structurally unchanged in the successor core).
func (s *SuccessorRegistry) ResolveRepository(ctx context.Context, owner, repo string) (successor.RepositoryView, bool, error) {
	ownerAddress, err := normalizeEVMAddress(owner)
	if err != nil {
		return successor.RepositoryView{}, false, err
	}
	abi, err := successor.CoreABI()
	if err != nil {
		return successor.RepositoryView{}, false, err
	}
	data, err := abi.Pack("resolveRepository", common.HexToAddress(ownerAddress), repo)
	if err != nil {
		return successor.RepositoryView{}, false, fmt.Errorf("encode resolveRepository: %w", err)
	}
	result, err := s.rpc.CallContractAt(ctx, s.core, "0x"+hex.EncodeToString(data), "latest")
	if err != nil {
		return successor.RepositoryView{}, false, wrapEVMRPCError(err)
	}
	values, err := abi.Unpack("resolveRepository", result)
	if err != nil || len(values) != 2 {
		return successor.RepositoryView{}, false, fmt.Errorf("decode resolveRepository: %w", err)
	}
	resolved, err := suiteTuple[suiteRepositoryABI](values[0])
	if err != nil {
		return successor.RepositoryView{}, false, err
	}
	canonical, _ := values[1].(bool)
	if !resolved.Exists {
		return successor.RepositoryView{}, false, fmt.Errorf("repository %s/%s not found", owner, repo)
	}
	return successor.RepositoryView{
		RepoID:        resolved.Id,
		OwnerHex:      resolved.Owner.Hex(),
		Name:          resolved.Name,
		DefaultBranch: resolved.DefaultBranch,
	}, canonical, nil
}

// ListRefNames pages listRefsPage and returns the live ref names (tombstoned
// refs are absent by construction). Commitments are read per name via GetRef.
func (s *SuccessorRegistry) ListRefNames(ctx context.Context, repoID [32]byte) ([]string, error) {
	abi, err := successor.CoreABI()
	if err != nil {
		return nil, err
	}
	var names []string
	var cursor uint64
	for {
		data, err := abi.Pack("listRefsPage", repoID, new(big.Int).SetUint64(cursor), new(big.Int).SetUint64(suiteQueryPageSize))
		if err != nil {
			return nil, err
		}
		result, err := s.rpc.CallContractAt(ctx, s.core, "0x"+hex.EncodeToString(data), "latest")
		if err != nil {
			return nil, wrapEVMRPCError(err)
		}
		values, err := abi.Unpack("listRefsPage", result)
		if err != nil || len(values) != 3 {
			return nil, fmt.Errorf("decode listRefsPage: %w", err)
		}
		page, ok := values[0].([]string)
		if !ok {
			return nil, fmt.Errorf("listRefsPage names returned %T", values[0])
		}
		next, err := suiteBigUint64(values[2], "listRefsPage next cursor")
		if err != nil {
			return nil, err
		}
		names = append(names, page...)
		if len(page) == 0 || len(page) < int(suiteQueryPageSize) {
			return names, nil
		}
		if next <= cursor {
			return nil, fmt.Errorf("ref page cursor did not advance from %d", cursor)
		}
		cursor = next
	}
}

// successorEVMBackend adapts the successor client onto the production RPC and
// transactor. Reads go through eth_call at latest after the pinned
// construction-time suite verification; writes follow the single
// EVMTransactor nonce/signing/receipt semantics.
type successorEVMBackend struct {
	rpc        *EVMRPC
	transactor *EVMTransactor
	core       string
}

func (b *successorEVMBackend) Call(ctx context.Context, calldata []byte) ([]byte, error) {
	result, err := b.rpc.CallContractAt(ctx, b.core, "0x"+hex.EncodeToString(calldata), "latest")
	if err != nil {
		if typed := decodeSuccessorRevert(err); typed != nil {
			return nil, typed
		}
		return nil, wrapEVMRPCError(err)
	}
	return result, nil
}

// decodeSuccessorRevert maps an RPC revert payload carrying successor ABI
// error selectors to typed client errors (RefNotFound, CommitmentMismatch...).
func decodeSuccessorRevert(err error) error {
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Data == "" {
		return nil
	}
	raw, e := hex.DecodeString(strings.TrimPrefix(rpcErr.Data, "0x"))
	if e != nil || len(raw) < 4 {
		return nil
	}
	return successor.DecodeRevert(raw)
}

func (b *successorEVMBackend) Send(ctx context.Context, calldata []byte) (string, error) {
	if b.transactor == nil {
		return "", ErrEVMSignerUnavailable
	}
	result, err := b.transactor.Send(ctx, b.core, calldata, "")
	if err != nil {
		if typed := decodeSuccessorRevert(err); typed != nil {
			return "", typed
		}
		return "", err
	}
	return result.Hash, nil
}

// ensure the successor ABI exposes the repository methods this file packs
var _ = gethabi.ABI{}
