package chain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const suiteQueryPageSize uint64 = 64

// EVMSuiteRegistry is the ordinary runtime client for the immutable EVM suite.
// Its only configured contract address is SuiteDirectory; module addresses are
// accepted only after VerifySuite validates the complete binding and code-hash
// chain at one block.
type EVMSuiteRegistry struct {
	cfg        config.Config
	rpc        *EVMRPC
	signer     EVMSigner
	transactor *EVMTransactor
	directory  string

	verifyMu sync.Mutex
	info     *SuiteInfo
}

// NewEVMSuiteRegistry creates a lazy suite client. Network verification runs
// before the first contract read or write, which keeps construction useful in
// command wiring while preventing an unverified address from serving data.
func NewEVMSuiteRegistry(cfg config.Config) *EVMSuiteRegistry {
	rpc := NewEVMRPC(cfg.EffectiveEVMRPC())
	signer := NewEVMKeystoreSigner(cfg)
	return NewEVMSuiteRegistryWithDependencies(cfg, rpc, signer)
}

// NewEVMSuiteRegistryWithDependencies supplies test or hardware-backed
// transports without weakening suite verification.
func NewEVMSuiteRegistryWithDependencies(cfg config.Config, rpc *EVMRPC, signer EVMSigner) *EVMSuiteRegistry {
	if rpc == nil {
		rpc = NewEVMRPC(cfg.EffectiveEVMRPC())
	}
	nonces := newEVMNonceManager()
	return &EVMSuiteRegistry{
		cfg: cfg, rpc: rpc, signer: signer,
		transactor: newEVMTransactor(cfg, rpc, signer, nonces),
		directory:  cfg.EffectiveEVMSuiteDirectoryAddress(),
	}
}

// NewVerifiedEVMSuiteRegistry reuses already verified immutable bindings. It
// is primarily useful to suite info/verify commands and fixed-block exporters.
func NewVerifiedEVMSuiteRegistry(cfg config.Config, rpc *EVMRPC, signer EVMSigner, info *SuiteInfo) (*EVMSuiteRegistry, error) {
	registry := NewEVMSuiteRegistryWithDependencies(cfg, rpc, signer)
	if info == nil {
		return nil, errors.New("verified suite info is nil")
	}
	if !strings.EqualFold(info.Directory, registry.directory) {
		return nil, fmt.Errorf("verified directory %s does not match profile %s", info.Directory, registry.directory)
	}
	registry.info = info
	return registry, nil
}

var (
	_ RepoRegistryBackend      = (*EVMSuiteRegistry)(nil)
	_ OwnershipRegistryBackend = (*EVMSuiteRegistry)(nil)
	_ RecoveryBackend          = (*EVMSuiteRegistry)(nil)
	_ BadgeBackend             = (*EVMSuiteRegistry)(nil)
	_ EconomicBackend          = (*EVMSuiteRegistry)(nil)
	_ ModerationBackend        = (*EVMSuiteRegistry)(nil)
	_ UsernameBackend          = (*EVMSuiteRegistry)(nil)
	_ ReleaseBackend           = (*EVMSuiteRegistry)(nil)
)

func (s *EVMSuiteRegistry) ensureVerified(ctx context.Context) (*SuiteInfo, error) {
	if s == nil {
		return nil, errors.New("EVM suite registry is nil")
	}
	s.verifyMu.Lock()
	defer s.verifyMu.Unlock()
	if s.info != nil {
		return s.info, nil
	}
	if strings.TrimSpace(s.directory) == "" {
		return nil, errors.New("EVM SuiteDirectory address is not configured")
	}
	info, err := VerifySuite(ctx, s.rpc, s.directory, s.cfg.EffectiveEVMChainID())
	if err != nil {
		return nil, err
	}
	s.info = info
	return info, nil
}

// SuiteInfo returns a defensive copy of the verified directory view.
func (s *EVMSuiteRegistry) SuiteInfo(ctx context.Context) (*SuiteInfo, error) {
	info, err := s.ensureVerified(ctx)
	if err != nil {
		return nil, err
	}
	copyInfo := *info
	copyInfo.Modules = append([]SuiteModuleInfo(nil), info.Modules...)
	return &copyInfo, nil
}

func (s *EVMSuiteRegistry) module(ctx context.Context, name string) (string, gethabi.ABI, error) {
	info, err := s.ensureVerified(ctx)
	if err != nil {
		return "", gethabi.ABI{}, err
	}
	address := info.ModuleAddress(name)
	if address == "" {
		return "", gethabi.ABI{}, fmt.Errorf("verified suite has no %s module", name)
	}
	for _, required := range RequiredSuiteModules {
		if required.Name == name {
			contractABI, err := loadSuiteABI(required.ABI)
			return address, contractABI, err
		}
	}
	return "", gethabi.ABI{}, fmt.Errorf("unknown suite module %q", name)
}

func (s *EVMSuiteRegistry) snapshotBlockTag(ctx context.Context) (string, error) {
	if _, err := s.ensureVerified(ctx); err != nil {
		return "", err
	}
	block, err := s.rpc.BlockNumber(ctx)
	if err != nil {
		return "", wrapEVMRPCError(err)
	}
	return fmt.Sprintf("0x%x", block), nil
}

func (s *EVMSuiteRegistry) callAt(
	ctx context.Context,
	module, blockTag, method string,
	arguments ...any,
) ([]any, error) {
	address, contractABI, err := s.module(ctx, module)
	if err != nil {
		return nil, err
	}
	return suiteCall(ctx, s.rpc, address, blockTag, contractABI, method, arguments...)
}

func (s *EVMSuiteRegistry) send(ctx context.Context, module, method, value string, arguments ...any) error {
	address, contractABI, err := s.module(ctx, module)
	if err != nil {
		return err
	}
	data, err := contractABI.Pack(method, arguments...)
	if err != nil {
		return fmt.Errorf("encode %s.%s: %w", module, method, err)
	}
	_, err = s.transactor.Send(ctx, address, data, value)
	return err
}

type suiteRepositoryABI struct {
	Id            [32]byte
	Owner         common.Address
	Name          string
	Description   string
	DefaultBranch string
	ForkedFrom    [32]byte
	CreatedAt     uint64
	UpdatedAt     uint64
	Exists        bool
}

type suiteRefABI struct {
	CommitSha string
	PackUris  []string
	UpdatedAt uint64
	UpdatedBy common.Address
	Exists    bool
}

type suitePendingTransferABI struct {
	NewOwner     common.Address
	ExecuteAfter uint64
	ExpiresAt    uint64
}

type suiteGuardianConfigABI struct {
	ConfiguredBy common.Address
	Threshold    uint8
	Guardians    []common.Address
}

type suiteRecoveryProposalABI struct {
	ProposedBy   common.Address
	NewOwner     common.Address
	ExecuteAfter uint64
	ExpiresAt    uint64
	Nonce        uint64
	Approvals    uint8
}

type suiteSplitABI struct {
	Recipient common.Address
	Bps       uint16
}

type suiteUsernameABI struct {
	Owner        common.Address
	RegisteredAt uint64
}

type suiteBadgeABI struct {
	Id        *big.Int
	RepoId    [32]byte
	Recipient common.Address
	AwardedBy common.Address
	Reason    string
	AwardedAt uint64
	Exists    bool
}

type suiteReleaseABI struct {
	Version      string
	Platform     string
	Sha256       [32]byte
	RegisteredBy common.Address
	RegisteredAt uint64
	Exists       bool
}

type suiteReportABI struct {
	Id         *big.Int
	RepoId     [32]byte
	Reporter   common.Address
	Status     uint8
	Resolution uint8
	ReasonHash string
	CreatedAt  uint64
	UpdatedAt  uint64
	Exists     bool
}

type suiteTrailABI struct {
	Action     uint8
	Actor      common.Address
	Status     uint8
	ReasonHash string
	Timestamp  uint64
}

func suiteTuple[T any](value any) (T, error) {
	var zero T
	converted := gethabi.ConvertType(value, new(T))
	pointer, ok := converted.(*T)
	if !ok || pointer == nil {
		return zero, fmt.Errorf("ABI tuple has type %T, want %T", value, zero)
	}
	return *pointer, nil
}

func suiteTupleSlice[T any](value any) ([]T, error) {
	converted := gethabi.ConvertType(value, new([]T))
	pointer, ok := converted.(*[]T)
	if !ok || pointer == nil {
		return nil, fmt.Errorf("ABI tuple array has type %T", value)
	}
	return *pointer, nil
}

func suiteBigUint64(value any, label string) (uint64, error) {
	number, ok := value.(*big.Int)
	if !ok || number == nil || !number.IsUint64() {
		return 0, fmt.Errorf("%s returned %T outside uint64", label, value)
	}
	return number.Uint64(), nil
}

func suiteRepoID(value [32]byte) [32]byte { return value }

func (s *EVMSuiteRegistry) moderationStatusAt(ctx context.Context, repoID [32]byte, blockTag string) (string, error) {
	values, err := s.callAt(ctx, "moderation", blockTag, "effectiveStatus", repoID)
	if err != nil {
		return "", err
	}
	if len(values) != 1 {
		return "", fmt.Errorf("effectiveStatus returned %d values", len(values))
	}
	status, ok := values[0].(uint8)
	if !ok {
		return "", fmt.Errorf("effectiveStatus returned %T", values[0])
	}
	return moderationStatusName(uint64(status))
}

func suiteUserAddress(address common.Address) (string, error) {
	return userAddressFromEVM(address.Hex())
}

func (s *EVMSuiteRegistry) repoInfoAt(ctx context.Context, repository suiteRepositoryABI, blockTag string) (RepoInfo, error) {
	owner, err := suiteUserAddress(repository.Owner)
	if err != nil {
		return RepoInfo{}, err
	}
	status, err := s.moderationStatusAt(ctx, repository.Id, blockTag)
	if err != nil {
		return RepoInfo{}, err
	}
	return RepoInfo{
		Owner: owner, Name: repository.Name, Description: repository.Description,
		DefaultBranch: repository.DefaultBranch, CreatedAt: repository.CreatedAt,
		UpdatedAt: repository.UpdatedAt, ModerationStatus: status,
	}, nil
}

func (s *EVMSuiteRegistry) resolveRepoAt(ctx context.Context, owner, name, blockTag string) (*ResolvedRepo, error) {
	ownerAddress, err := normalizeEVMAddress(owner)
	if err != nil {
		return nil, err
	}
	values, err := s.callAt(ctx, "core", blockTag, "resolveRepository", common.HexToAddress(ownerAddress), name)
	if err != nil {
		return nil, err
	}
	if len(values) != 2 {
		return nil, fmt.Errorf("resolveRepository returned %d values", len(values))
	}
	repository, err := suiteTuple[suiteRepositoryABI](values[0])
	if err != nil {
		return nil, err
	}
	canonical, ok := values[1].(bool)
	if !ok || !repository.Exists {
		return nil, fmt.Errorf("resolveRepository returned invalid repository state")
	}
	info, err := s.repoInfoAt(ctx, repository, blockTag)
	if err != nil {
		return nil, err
	}
	return &ResolvedRepo{
		RepoID: suiteRepoID(repository.Id), Backend: BackendEVM,
		Requested:   RepoLocator{Owner: owner, Name: name},
		Canonical:   RepoLocator{Owner: info.Owner, Name: repository.Name},
		IsCanonical: canonical, Info: info,
	}, nil
}

func (s *EVMSuiteRegistry) ResolveRepo(owner, repo string) (*ResolvedRepo, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	return s.resolveRepoAt(ctx, owner, repo, blockTag)
}

func (s *EVMSuiteRegistry) repoForWrite(ctx context.Context, owner, repo string) (*ResolvedRepo, error) {
	resolved, err := s.resolveRepoAt(ctx, owner, repo, "latest")
	if err != nil {
		return nil, err
	}
	if !resolved.IsCanonical {
		return nil, &RepoMovedError{
			RepoID: resolved.RepoID, CurrentOwner: resolved.Canonical.Owner, Name: resolved.Canonical.Name,
		}
	}
	return resolved, nil
}

func (s *EVMSuiteRegistry) signerRepo(ctx context.Context, repo string) (*ResolvedRepo, error) {
	if s.signer == nil {
		return nil, ErrEVMSignerUnavailable
	}
	owner, err := s.signer.OwnerAddress()
	if err != nil {
		return nil, err
	}
	return s.repoForWrite(ctx, owner, repo)
}

func (s *EVMSuiteRegistry) CreateRepo(name, description, defaultBranch string) error {
	return s.send(context.Background(), "core", "createRepository", "", name, description, defaultBranch)
}

func (s *EVMSuiteRegistry) ForkRepo(owner, repo, newName string) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	if strings.TrimSpace(newName) == "" {
		newName = resolved.Canonical.Name
	}
	return s.send(ctx, "core", "forkRepository", "", resolved.RepoID, newName)
}

func (s *EVMSuiteRegistry) UpdateRepoInfo(repo string, description, defaultBranch *string) error {
	ctx := context.Background()
	resolved, err := s.signerRepo(ctx, repo)
	if err != nil {
		return err
	}
	descriptionValue, branchValue := "", ""
	if description != nil {
		descriptionValue = *description
	}
	if defaultBranch != nil {
		branchValue = *defaultBranch
	}
	return s.send(ctx, "core", "updateMetadata", "", resolved.RepoID,
		description != nil, descriptionValue, defaultBranch != nil, branchValue)
}

func (s *EVMSuiteRegistry) UpdateRef(owner, repo, refName, commitSHA string, packURIs []string, expectedSHA string, force bool) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	return s.send(ctx, "core", "updateRef", "", resolved.RepoID, refName, commitSHA, packURIs, expectedSHA, force)
}

func (s *EVMSuiteRegistry) DeleteRef(owner, repo, refName string) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	return s.send(ctx, "core", "deleteRef", "", resolved.RepoID, refName)
}

func (s *EVMSuiteRegistry) SetCollaborator(owner, repo, collaborator, role string) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	account, err := normalizeEVMAddress(collaborator)
	if err != nil {
		return err
	}
	roleValue, err := collaboratorRoleValue(role)
	if err != nil {
		return err
	}
	return s.send(ctx, "core", "setCollaborator", "", resolved.RepoID, common.HexToAddress(account), uint8(roleValue))
}

func (s *EVMSuiteRegistry) ListRepos(owner string) ([]RepoInfo, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	address, err := normalizeEVMAddress(owner)
	if err != nil {
		return nil, err
	}
	var result []RepoInfo
	var cursor uint64
	for {
		values, err := s.callAt(ctx, "core", blockTag, "listRepositoriesPage",
			common.HexToAddress(address), new(big.Int).SetUint64(cursor), new(big.Int).SetUint64(suiteQueryPageSize))
		if err != nil {
			return nil, err
		}
		if len(values) != 2 {
			return nil, fmt.Errorf("listRepositoriesPage returned %d values", len(values))
		}
		page, err := suiteTupleSlice[suiteRepositoryABI](values[0])
		if err != nil {
			return nil, err
		}
		next, err := suiteBigUint64(values[1], "listRepositoriesPage next cursor")
		if err != nil {
			return nil, err
		}
		for _, repository := range page {
			info, err := s.repoInfoAt(ctx, repository, blockTag)
			if err != nil {
				return nil, err
			}
			result = append(result, info)
		}
		if len(page) == 0 || len(page) < int(suiteQueryPageSize) {
			return result, nil
		}
		if next <= cursor {
			return nil, fmt.Errorf("repository page cursor did not advance from %d", cursor)
		}
		cursor = next
	}
}

func (s *EVMSuiteRegistry) RepoInfo(owner, repo string) (*RepoInfo, error) {
	resolved, err := s.ResolveRepo(owner, repo)
	if err != nil {
		return nil, err
	}
	info := resolved.Info
	return &info, nil
}

func suiteRefInfo(name string, reference suiteRefABI) (RefInfo, error) {
	updatedBy, err := suiteUserAddress(reference.UpdatedBy)
	if err != nil {
		return RefInfo{}, err
	}
	return RefInfo{
		RefName: name, CommitSha: reference.CommitSha,
		PackURIs:  append([]string(nil), reference.PackUris...),
		UpdatedAt: reference.UpdatedAt, UpdatedBy: updatedBy,
	}, nil
}

func (s *EVMSuiteRegistry) ListRefs(owner, repo string) ([]RefInfo, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveRepoAt(ctx, owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	var result []RefInfo
	var cursor uint64
	for {
		values, err := s.callAt(ctx, "core", blockTag, "listRefsPage", resolved.RepoID,
			new(big.Int).SetUint64(cursor), new(big.Int).SetUint64(suiteQueryPageSize))
		if err != nil {
			return nil, err
		}
		if len(values) != 3 {
			return nil, fmt.Errorf("listRefsPage returned %d values", len(values))
		}
		names, ok := values[0].([]string)
		if !ok {
			return nil, fmt.Errorf("listRefsPage names returned %T", values[0])
		}
		refs, err := suiteTupleSlice[suiteRefABI](values[1])
		if err != nil {
			return nil, err
		}
		if len(names) != len(refs) {
			return nil, fmt.Errorf("listRefsPage returned %d names and %d refs", len(names), len(refs))
		}
		next, err := suiteBigUint64(values[2], "listRefsPage next cursor")
		if err != nil {
			return nil, err
		}
		for index := range refs {
			info, err := suiteRefInfo(names[index], refs[index])
			if err != nil {
				return nil, err
			}
			result = append(result, info)
		}
		if len(refs) == 0 || len(refs) < int(suiteQueryPageSize) {
			return result, nil
		}
		if next <= cursor {
			return nil, fmt.Errorf("ref page cursor did not advance from %d", cursor)
		}
		cursor = next
	}
}

func (s *EVMSuiteRegistry) ResolveRef(owner, repo, refName string) (string, []string, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return "", nil, err
	}
	resolved, err := s.resolveRepoAt(ctx, owner, repo, blockTag)
	if err != nil {
		return "", nil, err
	}
	values, err := s.callAt(ctx, "core", blockTag, "getRef", resolved.RepoID, refName)
	if err != nil {
		return "", nil, err
	}
	if len(values) != 1 {
		return "", nil, fmt.Errorf("getRef returned %d values", len(values))
	}
	reference, err := suiteTuple[suiteRefABI](values[0])
	if err != nil {
		return "", nil, fmt.Errorf("decode getRef: %w", err)
	}
	if !reference.Exists {
		return "", nil, fmt.Errorf("ref %q does not exist", refName)
	}
	return reference.CommitSha, append([]string(nil), reference.PackUris...), nil
}

func (s *EVMSuiteRegistry) ListCollaborators(owner, repo string) ([]CollaboratorInfo, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveRepoAt(ctx, owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	var result []CollaboratorInfo
	var cursor uint64
	for {
		values, err := s.callAt(ctx, "core", blockTag, "listCollaboratorsPage", resolved.RepoID,
			new(big.Int).SetUint64(cursor), new(big.Int).SetUint64(suiteQueryPageSize))
		if err != nil {
			return nil, err
		}
		if len(values) != 3 {
			return nil, fmt.Errorf("listCollaboratorsPage returned %d values", len(values))
		}
		accounts, ok := values[0].([]common.Address)
		if !ok {
			return nil, fmt.Errorf("listCollaboratorsPage accounts returned %T", values[0])
		}
		roles, ok := values[1].([]uint8)
		if !ok || len(accounts) != len(roles) {
			return nil, fmt.Errorf("listCollaboratorsPage returned mismatched accounts and roles")
		}
		next, err := suiteBigUint64(values[2], "listCollaboratorsPage next cursor")
		if err != nil {
			return nil, err
		}
		for index, account := range accounts {
			address, err := suiteUserAddress(account)
			if err != nil {
				return nil, err
			}
			role, err := collaboratorRoleName(uint64(roles[index]))
			if err != nil {
				return nil, err
			}
			result = append(result, CollaboratorInfo{Address: address, Role: role})
		}
		if len(accounts) == 0 || len(accounts) < int(suiteQueryPageSize) {
			return result, nil
		}
		if next <= cursor {
			return nil, fmt.Errorf("collaborator page cursor did not advance from %d", cursor)
		}
		cursor = next
	}
}

func (s *EVMSuiteRegistry) ResolveUsername(name string) (string, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return "", err
	}
	values, err := s.callAt(ctx, "username", blockTag, "resolveUsername", name)
	if err != nil {
		return "", err
	}
	if len(values) != 1 {
		return "", fmt.Errorf("resolveUsername returned %d values", len(values))
	}
	record, err := suiteTuple[suiteUsernameABI](values[0])
	if err != nil {
		return "", err
	}
	return suiteUserAddress(record.Owner)
}

func (s *EVMSuiteRegistry) AddressUsername(address string) (string, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return "", err
	}
	normalized, err := normalizeEVMAddress(address)
	if err != nil {
		return "", err
	}
	values, err := s.callAt(ctx, "username", blockTag, "usernameOf", common.HexToAddress(normalized))
	if err != nil {
		return "", err
	}
	if len(values) != 1 {
		return "", fmt.Errorf("usernameOf returned %d values", len(values))
	}
	name, ok := values[0].(string)
	if !ok {
		return "", fmt.Errorf("usernameOf returned %T", values[0])
	}
	return name, nil
}

func (s *EVMSuiteRegistry) RegisterUsername(name string) error {
	return s.send(context.Background(), "username", "registerUsername", "", name)
}

func (s *EVMSuiteRegistry) ClaimOriginalUsername(name string) error {
	return s.send(context.Background(), "username", "claimOriginalUsername", "", name)
}

func (s *EVMSuiteRegistry) ReleaseUsername() error {
	return s.send(context.Background(), "username", "releaseUsername", "")
}

func (s *EVMSuiteRegistry) BeginOwnershipTransfer(repo *ResolvedRepo, newOwner string) error {
	if err := requireSuiteResolvedRepo(repo, true); err != nil {
		return err
	}
	address, err := normalizeEVMAddress(newOwner)
	if err != nil {
		return err
	}
	return s.send(context.Background(), "core", "beginOwnershipTransfer", "", repo.RepoID, common.HexToAddress(address))
}

func (s *EVMSuiteRegistry) CancelOwnershipTransfer(repo *ResolvedRepo) error {
	if err := requireSuiteResolvedRepo(repo, true); err != nil {
		return err
	}
	return s.send(context.Background(), "core", "cancelOwnershipTransfer", "", repo.RepoID)
}

func (s *EVMSuiteRegistry) RejectOwnershipTransfer(repo *ResolvedRepo) error {
	return s.CancelOwnershipTransfer(repo)
}

func (s *EVMSuiteRegistry) ExpireOwnershipTransfer(repo *ResolvedRepo) error {
	if err := requireSuiteResolvedRepo(repo, false); err != nil {
		return err
	}
	return s.send(context.Background(), "core", "expireOwnershipTransfer", "", repo.RepoID)
}

func (s *EVMSuiteRegistry) AcceptOwnership(repo *ResolvedRepo) error {
	if err := requireSuiteResolvedRepo(repo, false); err != nil {
		return err
	}
	return s.send(context.Background(), "core", "acceptOwnershipTransfer", "", repo.RepoID)
}

func requireSuiteResolvedRepo(repo *ResolvedRepo, canonical bool) error {
	if repo == nil {
		return errors.New("resolved repository is nil")
	}
	if repo.Backend != BackendEVM {
		return errors.New("resolved repository is not part of the active EVM suite")
	}
	if canonical && !repo.IsCanonical {
		return &RepoMovedError{RepoID: repo.RepoID, CurrentOwner: repo.Canonical.Owner, Name: repo.Canonical.Name}
	}
	return nil
}

func (s *EVMSuiteRegistry) pendingOwnershipTransferAt(ctx context.Context, repoID [32]byte, blockTag string) (*OwnershipTransferInfo, error) {
	values, err := s.callAt(ctx, "core", blockTag, "pendingOwnershipTransfer", repoID)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("pendingOwnershipTransfer returned %d values", len(values))
	}
	pending, err := suiteTuple[suitePendingTransferABI](values[0])
	if err != nil {
		return nil, err
	}
	if pending.NewOwner == (common.Address{}) {
		return nil, nil
	}
	newOwner, err := suiteUserAddress(pending.NewOwner)
	if err != nil {
		return nil, err
	}
	proposedAt := uint64(0)
	if pending.ExecuteAfter >= 7*24*60*60 {
		proposedAt = pending.ExecuteAfter - 7*24*60*60
	}
	return &OwnershipTransferInfo{
		NewOwner: newOwner, ProposedAt: proposedAt,
		ExecuteAfter: pending.ExecuteAfter, ExpiresAt: pending.ExpiresAt,
	}, nil
}

func (s *EVMSuiteRegistry) PendingOwnershipTransfer(repo *ResolvedRepo) (*OwnershipTransferInfo, error) {
	if err := requireSuiteResolvedRepo(repo, false); err != nil {
		return nil, err
	}
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	return s.pendingOwnershipTransferAt(ctx, repo.RepoID, blockTag)
}

func (s *EVMSuiteRegistry) SetGuardians(repo string, guardians []string, threshold uint8) error {
	ctx := context.Background()
	resolved, err := s.signerRepo(ctx, repo)
	if err != nil {
		return err
	}
	addresses := make([]common.Address, len(guardians))
	for index, guardian := range guardians {
		normalized, err := normalizeEVMAddress(guardian)
		if err != nil {
			return fmt.Errorf("guardian %d: %w", index, err)
		}
		addresses[index] = common.HexToAddress(normalized)
	}
	return s.send(ctx, "recovery", "setGuardians", "", resolved.RepoID, addresses, threshold)
}

func (s *EVMSuiteRegistry) ProposeRecovery(owner, repo, newOwner string) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	address, err := normalizeEVMAddress(newOwner)
	if err != nil {
		return err
	}
	return s.send(ctx, "recovery", "proposeRecovery", "", resolved.RepoID, common.HexToAddress(address))
}

func (s *EVMSuiteRegistry) ApproveRecovery(owner, repo string) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	return s.send(ctx, "recovery", "approveRecovery", "", resolved.RepoID)
}

func (s *EVMSuiteRegistry) CancelRecovery(repo string) error {
	ctx := context.Background()
	resolved, err := s.signerRepo(ctx, repo)
	if err != nil {
		return err
	}
	return s.send(ctx, "recovery", "cancelRecovery", "", resolved.RepoID)
}

func (s *EVMSuiteRegistry) AcceptRecovery(owner, repo string) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	return s.send(ctx, "recovery", "executeRecovery", "", resolved.RepoID)
}

func (s *EVMSuiteRegistry) OwnershipSecurity(owner, repo string) (*OwnershipSecurityInfo, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveRepoAt(ctx, owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	transfer, err := s.pendingOwnershipTransferAt(ctx, resolved.RepoID, blockTag)
	if err != nil {
		return nil, err
	}
	configValues, err := s.callAt(ctx, "recovery", blockTag, "guardianConfig", resolved.RepoID)
	if err != nil {
		return nil, err
	}
	proposalValues, err := s.callAt(ctx, "recovery", blockTag, "recoveryProposal", resolved.RepoID)
	if err != nil {
		return nil, err
	}
	if len(configValues) != 1 || len(proposalValues) != 1 {
		return nil, errors.New("recovery module returned malformed security state")
	}
	guardianConfig, err := suiteTuple[suiteGuardianConfigABI](configValues[0])
	if err != nil {
		return nil, err
	}
	proposal, err := suiteTuple[suiteRecoveryProposalABI](proposalValues[0])
	if err != nil {
		return nil, err
	}
	guardians := make([]string, len(guardianConfig.Guardians))
	for index, guardian := range guardianConfig.Guardians {
		guardians[index], err = suiteUserAddress(guardian)
		if err != nil {
			return nil, err
		}
	}
	var recovery *RecoveryProposalInfo
	if proposal.NewOwner != (common.Address{}) {
		newOwner, err := suiteUserAddress(proposal.NewOwner)
		if err != nil {
			return nil, err
		}
		approvals := make([]string, 0, proposal.Approvals)
		for index, guardian := range guardianConfig.Guardians {
			approvedValues, err := s.callAt(ctx, "recovery", blockTag, "hasApproved", resolved.RepoID, proposal.Nonce, guardian)
			if err != nil {
				return nil, err
			}
			if len(approvedValues) == 1 {
				approved, _ := approvedValues[0].(bool)
				if approved {
					approvals = append(approvals, guardians[index])
				}
			}
		}
		proposedAt := uint64(0)
		if proposal.ExecuteAfter >= 7*24*60*60 {
			proposedAt = proposal.ExecuteAfter - 7*24*60*60
		}
		recovery = &RecoveryProposalInfo{
			NewOwner: newOwner, ProposedAt: proposedAt,
			ExecuteAfter: proposal.ExecuteAfter, Approvals: approvals,
		}
	}
	return &OwnershipSecurityInfo{
		Transfer: transfer, Recovery: recovery, Guardians: guardians,
		GuardianThreshold: guardianConfig.Threshold,
	}, nil
}

func (s *EVMSuiteRegistry) SetModerationStatus(owner, repo, status, reasonHash string) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	code, err := moderationStatusCode(status)
	if err != nil {
		return err
	}
	return s.send(ctx, "moderation", "setRepositoryStatus", "", resolved.RepoID, uint8(code), reasonHash)
}

func (s *EVMSuiteRegistry) SubmitModerationReport(owner, repo, reasonHash string) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	return s.send(ctx, "moderation", "submitReport", "", resolved.RepoID, reasonHash)
}

func (s *EVMSuiteRegistry) ResolveModerationReport(id uint64, status, reasonHash string) error {
	return s.sendModerationDecision("resolveReport", id, status, reasonHash)
}

func (s *EVMSuiteRegistry) ResolveModerationAppeal(id uint64, status, reasonHash string) error {
	return s.sendModerationDecision("resolveAppeal", id, status, reasonHash)
}

func (s *EVMSuiteRegistry) sendModerationDecision(method string, id uint64, status, reasonHash string) error {
	code, err := moderationStatusCode(status)
	if err != nil {
		return err
	}
	return s.send(context.Background(), "moderation", method, "", new(big.Int).SetUint64(id), uint8(code), reasonHash)
}

func (s *EVMSuiteRegistry) AppealModerationReport(id uint64, reasonHash string) error {
	return s.send(context.Background(), "moderation", "appealReport", "", new(big.Int).SetUint64(id), reasonHash)
}

func (s *EVMSuiteRegistry) ModerationReport(id uint64) (*ModerationReportInfo, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	values, err := s.callAt(ctx, "moderation", blockTag, "getReport", new(big.Int).SetUint64(id))
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("getReport returned %d values", len(values))
	}
	report, err := suiteTuple[suiteReportABI](values[0])
	if err != nil || !report.Exists || report.Id == nil || !report.Id.IsUint64() {
		return nil, fmt.Errorf("decode moderation report: %w", err)
	}
	repository, err := s.repositoryByIDAt(ctx, report.RepoId, blockTag)
	if err != nil {
		return nil, err
	}
	owner, err := suiteUserAddress(repository.Owner)
	if err != nil {
		return nil, err
	}
	reporter, err := suiteUserAddress(report.Reporter)
	if err != nil {
		return nil, err
	}
	status, err := reportStatusName(uint64(report.Status))
	if err != nil {
		return nil, err
	}
	resolution := ""
	if report.Status != 0 {
		resolution, err = moderationStatusName(uint64(report.Resolution))
		if err != nil {
			return nil, err
		}
	}
	resolutionHash, appealHash, err := s.reportReasonTrailAt(ctx, report.Id.Uint64(), blockTag)
	if err != nil {
		return nil, err
	}
	return &ModerationReportInfo{
		ID: report.Id.Uint64(), Owner: owner, Repo: repository.Name,
		Reporter: reporter, ReasonHash: report.ReasonHash, Status: status,
		Resolution: resolution, ResolutionHash: resolutionHash, AppealHash: appealHash,
		CreatedAt: report.CreatedAt, UpdatedAt: report.UpdatedAt,
	}, nil
}

func (s *EVMSuiteRegistry) reportReasonTrailAt(ctx context.Context, id uint64, blockTag string) (*string, *string, error) {
	var cursor uint64
	var resolutionHash, appealHash *string
	for {
		values, err := s.callAt(ctx, "moderation", blockTag, "listReportTrailPage",
			new(big.Int).SetUint64(id), new(big.Int).SetUint64(cursor), new(big.Int).SetUint64(suiteQueryPageSize))
		if err != nil {
			return nil, nil, err
		}
		if len(values) != 2 {
			return nil, nil, errors.New("listReportTrailPage returned malformed values")
		}
		page, err := suiteTupleSlice[suiteTrailABI](values[0])
		if err != nil {
			return nil, nil, err
		}
		next, err := suiteBigUint64(values[1], "listReportTrailPage next cursor")
		if err != nil {
			return nil, nil, err
		}
		for _, entry := range page {
			value := entry.ReasonHash
			switch entry.Action {
			case 1, 3:
				resolutionHash = &value
			case 2:
				appealHash = &value
			}
		}
		if len(page) == 0 || len(page) < int(suiteQueryPageSize) {
			return resolutionHash, appealHash, nil
		}
		if next <= cursor {
			return nil, nil, fmt.Errorf("report trail cursor did not advance from %d", cursor)
		}
		cursor = next
	}
}

func (s *EVMSuiteRegistry) Sponsor(owner, repo, message, amount string) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	value, err := evmINJValue(amount)
	if err != nil {
		return err
	}
	return s.send(ctx, "economic", "sponsor", value, resolved.RepoID, message)
}

func (s *EVMSuiteRegistry) SetRevenueSplits(owner, repo string, splits []SplitEntry) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	recipients := make([]common.Address, len(splits))
	bps := make([]uint16, len(splits))
	for index, split := range splits {
		address, err := normalizeEVMAddress(split.Address)
		if err != nil {
			return fmt.Errorf("split %d: %w", index, err)
		}
		recipients[index] = common.HexToAddress(address)
		bps[index] = split.Bps
	}
	return s.send(ctx, "economic", "setRevenueSplits", "", resolved.RepoID, recipients, bps)
}

func (s *EVMSuiteRegistry) RevenueSplits(owner, repo string) ([]SplitEntry, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveRepoAt(ctx, owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	values, err := s.callAt(ctx, "economic", blockTag, "revenueSplits", resolved.RepoID)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("revenueSplits returned %d values", len(values))
	}
	splits, err := suiteTupleSlice[suiteSplitABI](values[0])
	if err != nil {
		return nil, err
	}
	result := make([]SplitEntry, len(splits))
	for index, split := range splits {
		address, err := suiteUserAddress(split.Recipient)
		if err != nil {
			return nil, err
		}
		result[index] = SplitEntry{Address: address, Bps: split.Bps}
	}
	return result, nil
}

func (s *EVMSuiteRegistry) SponsorTotals(owner, repo string) ([]Coin, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveRepoAt(ctx, owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	values, err := s.callAt(ctx, "economic", blockTag, "sponsorDenoms", resolved.RepoID)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, errors.New("sponsorDenoms returned malformed values")
	}
	denoms, ok := values[0].([]string)
	if !ok {
		return nil, fmt.Errorf("sponsorDenoms returned %T", values[0])
	}
	result := make([]Coin, 0, len(denoms))
	for _, denom := range denoms {
		totalValues, err := s.callAt(ctx, "economic", blockTag, "sponsorTotal", resolved.RepoID, denom)
		if err != nil {
			return nil, err
		}
		if len(totalValues) != 1 {
			return nil, errors.New("sponsorTotal returned malformed values")
		}
		total, ok := totalValues[0].(*big.Int)
		if !ok || total == nil {
			return nil, fmt.Errorf("sponsorTotal returned %T", totalValues[0])
		}
		result = append(result, Coin{Denom: denom, Amount: total.String()})
	}
	return result, nil
}

func (s *EVMSuiteRegistry) AwardBadge(owner, repo, recipient, reason string) error {
	ctx := context.Background()
	resolved, err := s.repoForWrite(ctx, owner, repo)
	if err != nil {
		return err
	}
	address, err := normalizeEVMAddress(recipient)
	if err != nil {
		return err
	}
	return s.send(ctx, "badge", "awardBadge", "", resolved.RepoID, common.HexToAddress(address), reason)
}

func (s *EVMSuiteRegistry) BadgesByRecipient(recipient string) ([]Badge, error) {
	address, err := normalizeEVMAddress(recipient)
	if err != nil {
		return nil, err
	}
	return s.badgesPage("listBadgesByRecipientPage", common.HexToAddress(address))
}

func (s *EVMSuiteRegistry) BadgesByRepo(owner, repo string) ([]Badge, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveRepoAt(ctx, owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	return s.badgesPageAt(ctx, blockTag, "listBadgesByRepositoryPage", resolved.RepoID)
}

func (s *EVMSuiteRegistry) badgesPage(method string, key any) ([]Badge, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	return s.badgesPageAt(ctx, blockTag, method, key)
}

func (s *EVMSuiteRegistry) badgesPageAt(ctx context.Context, blockTag, method string, key any) ([]Badge, error) {
	var result []Badge
	var cursor uint64
	repositories := make(map[[32]byte]suiteRepositoryABI)
	for {
		values, err := s.callAt(ctx, "badge", blockTag, method, key,
			new(big.Int).SetUint64(cursor), new(big.Int).SetUint64(suiteQueryPageSize))
		if err != nil {
			return nil, err
		}
		if len(values) != 2 {
			return nil, fmt.Errorf("%s returned %d values", method, len(values))
		}
		page, err := suiteTupleSlice[suiteBadgeABI](values[0])
		if err != nil {
			return nil, err
		}
		next, err := suiteBigUint64(values[1], method+" next cursor")
		if err != nil {
			return nil, err
		}
		for _, badge := range page {
			if badge.Id == nil || !badge.Id.IsUint64() || !badge.Exists {
				return nil, errors.New("badge module returned an invalid badge")
			}
			repository, ok := repositories[badge.RepoId]
			if !ok {
				repository, err = s.repositoryByIDAt(ctx, badge.RepoId, blockTag)
				if err != nil {
					return nil, err
				}
				repositories[badge.RepoId] = repository
			}
			owner, err := suiteUserAddress(repository.Owner)
			if err != nil {
				return nil, err
			}
			recipient, err := suiteUserAddress(badge.Recipient)
			if err != nil {
				return nil, err
			}
			awardedBy, err := suiteUserAddress(badge.AwardedBy)
			if err != nil {
				return nil, err
			}
			result = append(result, Badge{
				ID: badge.Id.Uint64(), RepoID: common.BytesToHash(badge.RepoId[:]).Hex(),
				RepoOwner: owner, RepoName: repository.Name,
				Recipient: recipient, Reason: badge.Reason, AwardedBy: awardedBy, AwardedAt: badge.AwardedAt,
			})
		}
		if len(page) == 0 || len(page) < int(suiteQueryPageSize) {
			return result, nil
		}
		if next <= cursor {
			return nil, fmt.Errorf("badge page cursor did not advance from %d", cursor)
		}
		cursor = next
	}
}

func (s *EVMSuiteRegistry) repositoryByIDAt(ctx context.Context, repoID [32]byte, blockTag string) (suiteRepositoryABI, error) {
	values, err := s.callAt(ctx, "core", blockTag, "getRepository", repoID)
	if err != nil {
		return suiteRepositoryABI{}, err
	}
	if len(values) != 1 {
		return suiteRepositoryABI{}, fmt.Errorf("getRepository returned %d values", len(values))
	}
	return suiteTuple[suiteRepositoryABI](values[0])
}

func (s *EVMSuiteRegistry) RegisterRelease(version string, artifacts []ReleaseArtifact) error {
	for _, artifact := range artifacts {
		digest, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(artifact.SHA256)), "0x"))
		if err != nil || len(digest) != sha256.Size {
			return fmt.Errorf("invalid SHA-256 for %s", artifact.Platform)
		}
		var fixed [32]byte
		copy(fixed[:], digest)
		if err := s.send(context.Background(), "release", "registerArtifact", "", version, artifact.Platform, fixed); err != nil {
			return err
		}
	}
	return nil
}

func (s *EVMSuiteRegistry) ReleaseArtifacts(version string) ([]ReleaseArtifact, error) {
	ctx := context.Background()
	blockTag, err := s.snapshotBlockTag(ctx)
	if err != nil {
		return nil, err
	}
	var result []ReleaseArtifact
	var cursor uint64
	for {
		values, err := s.callAt(ctx, "release", blockTag, "listArtifactsPage", version,
			new(big.Int).SetUint64(cursor), new(big.Int).SetUint64(suiteQueryPageSize))
		if err != nil {
			return nil, err
		}
		if len(values) != 2 {
			return nil, errors.New("listArtifactsPage returned malformed values")
		}
		page, err := suiteTupleSlice[suiteReleaseABI](values[0])
		if err != nil {
			return nil, err
		}
		next, err := suiteBigUint64(values[1], "listArtifactsPage next cursor")
		if err != nil {
			return nil, err
		}
		for _, artifact := range page {
			if !artifact.Exists {
				return nil, errors.New("release module returned a missing artifact")
			}
			registeredBy, err := suiteUserAddress(artifact.RegisteredBy)
			if err != nil {
				return nil, err
			}
			result = append(result, ReleaseArtifact{
				Version: artifact.Version, Platform: artifact.Platform,
				SHA256:       hex.EncodeToString(artifact.Sha256[:]),
				RegisteredBy: registeredBy, RegisteredAt: artifact.RegisteredAt,
			})
		}
		if len(page) == 0 || len(page) < int(suiteQueryPageSize) {
			return result, nil
		}
		if next <= cursor {
			return nil, fmt.Errorf("release page cursor did not advance from %d", cursor)
		}
		cursor = next
	}
}

// FixedBlockRepository returns the complete Core tuple without converting or
// consulting moving state. Migration verification uses it after suite import.
func (s *EVMSuiteRegistry) FixedBlockRepository(ctx context.Context, repoID [32]byte, blockTag string) (suiteRepositoryABI, error) {
	if err := requireFixedEVMBlockTag(blockTag); err != nil {
		return suiteRepositoryABI{}, err
	}
	return s.repositoryByIDAt(ctx, repoID, blockTag)
}

func checkedUint64(value *big.Int, label string) (uint64, error) {
	if value == nil || value.Sign() < 0 || value.BitLen() > 64 {
		return 0, fmt.Errorf("%s exceeds uint64", label)
	}
	return value.Uint64(), nil
}

func checkedUintFromInt(value int, label string) (uint64, error) {
	if value < 0 || uint64(value) > math.MaxUint64 {
		return 0, fmt.Errorf("%s exceeds uint64", label)
	}
	return uint64(value), nil
}
