package chain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

var (
	// ErrEVMSignerUnavailable is returned before any transaction RPC when the
	// selected V2 backend has no secure signer implementation configured.
	ErrEVMSignerUnavailable = errors.New("EVM signer is not configured; install a secure EVM keystore signer")
	// ErrEVMUnsupportedFeature is used for V1-only operations absent from the
	// minimal V2 contract. Read-only compatibility queries use the legacy
	// adapter when the configured V1 contract is available.
	ErrEVMUnsupportedFeature = errors.New("operation is not available in EVM registry V2")
	// ErrLegacyWriteFallbackDisabled prevents an EVM-selected backend from
	// silently mutating a legacy repository after a V2 locator read miss.
	ErrLegacyWriteFallbackDisabled = errors.New("legacy repository writes are disabled while EVM V2 is selected")
)

// EVMTransaction is the signer boundary. The registry prepares chain ID,
// nonce, destination, calldata, and an estimated gas limit; a secure signer
// must produce the signed raw transaction. No private-key implementation lives
// in this package.
type EVMTransaction struct {
	ChainID  uint64
	Nonce    uint64
	To       string
	Data     string
	GasLimit uint64
	GasPrice string
	Value    string
}

// EVMSigner extends the user-facing signer contract with secure transaction
// signing. The current implementation uses an encrypted geth keystore; future
// implementations can use Windows Credential Manager, a system keyring, or a
// hardware wallet.
type EVMSigner interface {
	SignerBackend
	SignTransaction(ctx context.Context, tx EVMTransaction) (string, error)
}

// EVMRegistryV2 implements the core V2 registry operations over JSON-RPC.
// Reads are usable without a signer; writes fail explicitly until an
// EVMSigner is supplied through NewEVMRegistryV2WithDependencies.
type EVMRegistryV2 struct {
	cfg             config.Config
	rpc             *EVMRPC
	contractAddress string
	signer          EVMSigner
	nonceManager    *evmNonceManager
	legacy          *CosmWasmRegistryV1
}

// NewEVMRegistryV2 creates a V2 backend using EffectiveEVMRPC and
// EffectiveEVMContractAddress. Network profile resolution populates those
// values without changing this backend API.
func NewEVMRegistryV2(cfg config.Config) *EVMRegistryV2 {
	endpoint := cfg.EffectiveEVMRPC()
	return &EVMRegistryV2{
		cfg:             cfg,
		rpc:             NewEVMRPC(endpoint),
		contractAddress: cfg.EffectiveEVMContractAddress(),
		signer:          NewEVMKeystoreSigner(cfg),
		nonceManager:    newEVMNonceManager(),
		legacy:          legacyReadFallback(cfg),
	}
}

// NewEVMRegistryV2WithDependencies injects a transport and secure signer for
// tests or a future production signer implementation.
func NewEVMRegistryV2WithDependencies(cfg config.Config, rpc *EVMRPC, signer EVMSigner) *EVMRegistryV2 {
	if rpc == nil {
		endpoint := cfg.EffectiveEVMRPC()
		rpc = NewEVMRPC(endpoint)
	}
	return &EVMRegistryV2{
		cfg:             cfg,
		rpc:             rpc,
		contractAddress: cfg.EffectiveEVMContractAddress(),
		signer:          signer,
		nonceManager:    newEVMNonceManager(),
		legacy:          legacyReadFallback(cfg),
	}
}

// legacyReadFallback is intentionally enabled only for the automatic V2
// compatibility profile. An explicit evm/v2 selection is a pure EVM backend,
// matching doctor/setup behavior and preventing hidden LCD dependencies.
func legacyReadFallback(cfg config.Config) *CosmWasmRegistryV1 {
	if cfg.EffectiveContractBackend() != "auto" || cfg.EffectiveContractVersion() != "v2" {
		return nil
	}
	if _, err := decodeBech32Address(strings.TrimSpace(cfg.ContractAddress)); err != nil {
		return nil
	}
	return NewCosmWasmRegistryV1(cfg)
}

var _ RepoRegistryBackend = (*EVMRegistryV2)(nil)

func (e *EVMRegistryV2) contract() (string, error) {
	address := e.contractAddress
	if strings.TrimSpace(address) == "" {
		return "", fmt.Errorf("EVM V2 contract address is not configured")
	}
	return normalizeEVMAddress(address)
}

func (e *EVMRegistryV2) owner(owner string) (string, error) {
	return normalizeEVMAddress(owner)
}

// legacyOwner converts an already-normalized EVM owner back to the canonical
// Injective bech32 form required by the V1 LCD query surface. Keeping this at
// the fallback boundary prevents a caller's internal 0x address from leaking
// into a CosmWasm request.
func legacyOwner(owner, normalizedEVM string) string {
	if converted, err := userAddressFromEVM(normalizedEVM); err == nil {
		return converted
	}
	return owner
}

func (e *EVMRegistryV2) call(ctx context.Context, data []byte) ([]byte, error) {
	return e.callAt(ctx, data, "latest")
}

func (e *EVMRegistryV2) callAt(ctx context.Context, data []byte, blockTag string) ([]byte, error) {
	contract, err := e.contract()
	if err != nil {
		return nil, err
	}
	result, err := e.rpc.CallContractAt(ctx, contract, "0x"+hexEncode(data), blockTag)
	if err != nil {
		return nil, wrapEVMRPCError(err)
	}
	return result, nil
}

func (e *EVMRegistryV2) snapshotBlockTag(ctx context.Context) (string, error) {
	if _, err := e.contract(); err != nil {
		return "", err
	}
	blockNumber, err := e.rpc.BlockNumber(ctx)
	if err != nil {
		return "", wrapEVMRPCError(err)
	}
	return fmt.Sprintf("0x%x", blockNumber), nil
}

// ResolveRepo binds a user locator to the immutable V2 repo ID. Historical
// aliases remain readable but are marked non-canonical so callers can reject
// writes before packing, uploading, or signing anything.
func (e *EVMRegistryV2) ResolveRepo(owner, repo string) (*ResolvedRepo, error) {
	return e.resolveRepoAt(owner, repo, "latest")
}

func (e *EVMRegistryV2) resolveRepoAt(owner, repo, blockTag string) (*ResolvedRepo, error) {
	address, err := e.owner(owner)
	if err != nil {
		return nil, err
	}
	data, err := encodeABICall(
		"resolveRepo(address,string)",
		abiAddressValue(address),
		abiStringValue(repo),
	)
	if err != nil {
		return nil, err
	}
	result, err := e.callAt(context.Background(), data, blockTag)
	if err != nil {
		if isEVMLocatorNotFound(err) && e.legacy != nil {
			resolved, legacyErr := e.legacy.ResolveRepo(legacyOwner(owner, address), repo)
			if legacyErr != nil {
				return nil, legacyErr
			}
			resolved.WriteDisabled = true
			return resolved, nil
		}
		return nil, err
	}
	return decodeEVMResolvedRepo(result, owner, repo)
}

func (e *EVMRegistryV2) ownershipRepoID(repo *ResolvedRepo, requireCanonical bool) ([32]byte, error) {
	if repo == nil {
		return [32]byte{}, fmt.Errorf("resolved repository is nil")
	}
	if repo.Backend != BackendEVM {
		return [32]byte{}, fmt.Errorf("%w: %s", ErrLegacyWriteFallbackDisabled, repo.CanonicalURL())
	}
	if repo.WriteDisabled {
		return [32]byte{}, fmt.Errorf("%w: %s", ErrLegacyWriteFallbackDisabled, repo.CanonicalURL())
	}
	if requireCanonical && !repo.IsCanonical {
		return [32]byte{}, &RepoMovedError{
			RepoID:       repo.RepoID,
			CurrentOwner: repo.Canonical.Owner,
			Name:         repo.Canonical.Name,
		}
	}
	if repo.RepoID == ([32]byte{}) {
		return [32]byte{}, fmt.Errorf("resolved EVM repository has an empty repo ID")
	}
	return repo.RepoID, nil
}

func (e *EVMRegistryV2) BeginOwnershipTransfer(repo *ResolvedRepo, newOwner string) error {
	repoID, err := e.ownershipRepoID(repo, true)
	if err != nil {
		return err
	}
	owner, err := e.owner(newOwner)
	if err != nil {
		return fmt.Errorf("invalid ownership transfer target: %w", err)
	}
	data, err := encodeABICall(
		"beginOwnershipTransfer(bytes32,address)",
		abiBytes32Value(repoID), abiAddressValue(owner),
	)
	if err != nil {
		return err
	}
	return e.send(context.Background(), data)
}

func (e *EVMRegistryV2) CancelOwnershipTransfer(repo *ResolvedRepo) error {
	return e.sendOwnershipTransferAction(repo, "cancelOwnershipTransfer(bytes32)")
}

func (e *EVMRegistryV2) RejectOwnershipTransfer(repo *ResolvedRepo) error {
	return e.sendOwnershipTransferAction(repo, "rejectOwnershipTransfer(bytes32)")
}

func (e *EVMRegistryV2) ExpireOwnershipTransfer(repo *ResolvedRepo) error {
	return e.sendOwnershipTransferAction(repo, "expireOwnershipTransfer(bytes32)")
}

func (e *EVMRegistryV2) AcceptOwnership(repo *ResolvedRepo) error {
	return e.sendOwnershipTransferAction(repo, "acceptOwnership(bytes32)")
}

func (e *EVMRegistryV2) sendOwnershipTransferAction(repo *ResolvedRepo, signature string) error {
	repoID, err := e.ownershipRepoID(repo, true)
	if err != nil {
		return err
	}
	data, err := encodeABICall(signature, abiBytes32Value(repoID))
	if err != nil {
		return err
	}
	return e.send(context.Background(), data)
}

func (e *EVMRegistryV2) PendingOwnershipTransfer(repo *ResolvedRepo) (*OwnershipTransferInfo, error) {
	if repo == nil {
		return nil, fmt.Errorf("resolved repository is nil")
	}
	if repo.Backend == BackendCosmWasm {
		if e.legacy == nil {
			return nil, fmt.Errorf("%w: legacy repository reads are not configured", ErrEVMUnsupportedFeature)
		}
		return e.legacy.PendingOwnershipTransfer(repo)
	}
	if repo.Backend != BackendEVM {
		return nil, fmt.Errorf("resolved repository belongs to unknown backend %q", repo.Backend)
	}
	repoID, err := e.ownershipRepoID(repo, false)
	if err != nil {
		return nil, err
	}
	data, err := encodeABICall("pendingOwnershipTransfer(bytes32)", abiBytes32Value(repoID))
	if err != nil {
		return nil, err
	}
	result, err := e.call(context.Background(), data)
	if err != nil {
		return nil, err
	}
	return decodeEVMPendingOwnershipTransfer(result)
}

const evmQueryPageSize uint64 = 64

// ListRepos drains bounded owner-index pages from V2. Repository enumeration
// has no locator miss to classify, so a configured V2 transport/ABI failure is
// returned directly instead of being hidden by a legacy list query.
func (e *EVMRegistryV2) ListRepos(owner string) ([]RepoInfo, error) {
	address, err := e.owner(owner)
	if err != nil {
		return nil, err
	}
	blockTag, err := e.snapshotBlockTag(context.Background())
	if err != nil {
		return nil, err
	}
	var repositories []RepoInfo
	var cursor uint64
	for {
		data, err := encodeABICall(
			"listReposPage(address,uint256,uint256)",
			abiAddressValue(address),
			abiUintValue(cursor),
			abiUintValue(evmQueryPageSize),
		)
		if err != nil {
			return nil, err
		}
		result, err := e.callAt(context.Background(), data, blockTag)
		if err != nil {
			return nil, err
		}
		nextCursor, hasMore, page, err := decodeEVMRepoPage(result)
		if err != nil {
			return nil, err
		}
		repositories = append(repositories, page...)
		if !hasMore {
			return repositories, nil
		}
		if nextCursor <= cursor {
			return nil, fmt.Errorf("EVM repository page cursor did not advance from %d", cursor)
		}
		cursor = nextCursor
	}
}

// ListRefs drains bounded V2 pages after resolving the requested locator once.
// A LocatorNotFound can select the legacy read adapter during migration; every
// subsequent V2 failure stays on V2 so stale V1 state cannot leak in.
func (e *EVMRegistryV2) ListRefs(owner, repo string) ([]RefInfo, error) {
	blockTag, err := e.snapshotBlockTag(context.Background())
	if err != nil {
		return nil, err
	}
	resolved, err := e.resolveRepoAt(owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	if resolved.Backend == BackendCosmWasm {
		return e.legacy.ListRefs(resolved.Canonical.Owner, resolved.Canonical.Name)
	}

	refs := make([]RefInfo, 0)
	cursor := uint64(0)
	for {
		nextCursor, hasMore, page, err := e.listRefsPageByID(resolved.RepoID, cursor, evmQueryPageSize, blockTag)
		if err != nil {
			return nil, err
		}
		refs = append(refs, page...)
		if !hasMore {
			return refs, nil
		}
		if nextCursor <= cursor {
			return nil, fmt.Errorf("EVM listRefs page cursor did not advance: %d", cursor)
		}
		cursor = nextCursor
	}
}

func (e *EVMRegistryV2) listRefsPageByID(
	repoID [32]byte,
	cursor, limit uint64,
	blockTag string,
) (uint64, bool, []RefInfo, error) {
	data, err := encodeABICall(
		"listRefsPageById(bytes32,uint256,uint256)",
		abiBytes32Value(repoID),
		abiUintValue(cursor),
		abiUintValue(limit),
	)
	if err != nil {
		return 0, false, nil, err
	}
	result, err := e.callAt(context.Background(), data, blockTag)
	if err != nil {
		return 0, false, nil, err
	}
	return decodeEVMRefPage(result)
}

// RepoInfo fetches the V2 repository tuple and translates EVM owner addresses
// back to the canonical inj1... display form.
func (e *EVMRegistryV2) RepoInfo(owner, repo string) (*RepoInfo, error) {
	address, err := e.owner(owner)
	if err != nil {
		return nil, err
	}
	data, err := encodeABICall("getRepo(address,string)", abiAddressValue(address), abiStringValue(repo))
	if err != nil {
		return nil, err
	}
	result, err := e.call(context.Background(), data)
	if err != nil {
		if isEVMLocatorNotFound(err) && e.legacy != nil {
			return e.legacy.RepoInfo(legacyOwner(owner, address), repo)
		}
		return nil, err
	}
	return decodeEVMRepoInfo(result)
}

// repoInfoByIDAt resolves module-owned stable identities through the core
// registry at the same block used for paginated module reads. This keeps
// owner/name presentation canonical across ownership transfers.
func (e *EVMRegistryV2) repoInfoByIDAt(repoID [32]byte, blockTag string) (*RepoInfo, error) {
	data, err := encodeABICall("getRepoById(bytes32)", abiBytes32Value(repoID))
	if err != nil {
		return nil, err
	}
	result, err := e.callAt(context.Background(), data, blockTag)
	if err != nil {
		return nil, err
	}
	return decodeEVMRepoInfo(result)
}

// ResolveRef returns a V2 ref's commit SHA and pack URIs.
func (e *EVMRegistryV2) ResolveRef(owner, repo, refName string) (string, []string, error) {
	address, err := e.owner(owner)
	if err != nil {
		return "", nil, err
	}
	data, err := encodeABICall("resolveRef(address,string,string)", abiAddressValue(address), abiStringValue(repo), abiStringValue(refName))
	if err != nil {
		return "", nil, err
	}
	result, err := e.call(context.Background(), data)
	if err != nil {
		// A missing ref is not a missing repository. Only fall back for the
		// repository-level error so a V2 repo cannot silently resolve stale V1
		// state for one ref.
		if isEVMLocatorNotFound(err) && e.legacy != nil {
			return e.legacy.ResolveRef(legacyOwner(owner, address), repo, refName)
		}
		return "", nil, err
	}
	ref, err := decodeEVMResolveRef(result)
	if err != nil {
		return "", nil, err
	}
	return ref.CommitSha, ref.PackURIs, nil
}

func (e *EVMRegistryV2) CreateRepo(name, description, defaultBranch string) error {
	data, err := encodeABICall("createRepo(string,string,string)", abiStringValue(name), abiStringValue(description), abiStringValue(defaultBranch))
	if err != nil {
		return err
	}
	return e.send(context.Background(), data)
}

// UpdateRepoInfo patches metadata in the signing account's repository
// namespace. Explicit flags preserve the V1 distinction between an omitted
// field and setting that field to an empty string.
func (e *EVMRegistryV2) UpdateRepoInfo(repo string, description, defaultBranch *string) error {
	updateDescription := description != nil
	descriptionValue := ""
	if description != nil {
		descriptionValue = *description
	}
	updateDefaultBranch := defaultBranch != nil
	defaultBranchValue := ""
	if defaultBranch != nil {
		defaultBranchValue = *defaultBranch
	}
	data, err := encodeABICall(
		"updateRepoInfo(string,bool,string,bool,string)",
		abiStringValue(repo), abiBoolValue(updateDescription), abiStringValue(descriptionValue),
		abiBoolValue(updateDefaultBranch), abiStringValue(defaultBranchValue),
	)
	if err != nil {
		return err
	}
	return e.send(context.Background(), data)
}

func (e *EVMRegistryV2) UpdateRef(owner, repo, refName, commitSHA string, packURIs []string, expectedSHA string, force bool) error {
	address, err := e.owner(owner)
	if err != nil {
		return err
	}
	data, err := encodeABICall(
		"updateRef(address,string,string,string,string[],string,bool)",
		abiAddressValue(address), abiStringValue(repo), abiStringValue(refName),
		abiStringValue(commitSHA), abiStringArrayValue(packURIs), abiStringValue(expectedSHA), abiBoolValue(force),
	)
	if err != nil {
		return err
	}
	return e.send(context.Background(), data)
}

func (e *EVMRegistryV2) DeleteRef(owner, repo, refName string) error {
	address, err := e.owner(owner)
	if err != nil {
		return err
	}
	data, err := encodeABICall("deleteRef(address,string,string)", abiAddressValue(address), abiStringValue(repo), abiStringValue(refName))
	if err != nil {
		return err
	}
	return e.send(context.Background(), data)
}

// SetCollaborator updates or removes a V2 collaborator. The Solidity enum is
// represented in the ABI as uint8: 0=None, 1=Maintainer, 2=Reader.
func (e *EVMRegistryV2) SetCollaborator(owner, repo, collaborator, role string) error {
	ownerAddress, err := e.owner(owner)
	if err != nil {
		return err
	}
	collaboratorAddress, err := e.owner(collaborator)
	if err != nil {
		return fmt.Errorf("invalid collaborator address: %w", err)
	}
	roleValue, err := collaboratorRoleValue(role)
	if err != nil {
		return err
	}
	data, err := encodeABICall(
		"setCollaborator(address,string,address,uint8)",
		abiAddressValue(ownerAddress), abiStringValue(repo),
		abiAddressValue(collaboratorAddress), abiUintValue(roleValue),
	)
	if err != nil {
		return err
	}
	return e.send(context.Background(), data)
}

// ListCollaborators drains bounded V2 pages in the resolved repository.
func (e *EVMRegistryV2) ListCollaborators(owner, repo string) ([]CollaboratorInfo, error) {
	blockTag, err := e.snapshotBlockTag(context.Background())
	if err != nil {
		return nil, err
	}
	resolved, err := e.resolveRepoAt(owner, repo, blockTag)
	if err != nil {
		return nil, err
	}
	if resolved.Backend == BackendCosmWasm {
		return e.legacy.ListCollaborators(resolved.Canonical.Owner, resolved.Canonical.Name)
	}

	collaborators := make([]CollaboratorInfo, 0)
	cursor := uint64(0)
	for {
		nextCursor, hasMore, page, err := e.listCollaboratorsPageByID(
			resolved.RepoID,
			cursor,
			evmQueryPageSize,
			blockTag,
		)
		if err != nil {
			return nil, err
		}
		collaborators = append(collaborators, page...)
		if !hasMore {
			return collaborators, nil
		}
		if nextCursor <= cursor {
			return nil, fmt.Errorf("EVM listCollaborators page cursor did not advance: %d", cursor)
		}
		cursor = nextCursor
	}
}

func (e *EVMRegistryV2) listCollaboratorsPageByID(
	repoID [32]byte,
	cursor, limit uint64,
	blockTag string,
) (uint64, bool, []CollaboratorInfo, error) {
	data, err := encodeABICall(
		"listCollaboratorsPageById(bytes32,uint256,uint256)",
		abiBytes32Value(repoID),
		abiUintValue(cursor),
		abiUintValue(limit),
	)
	if err != nil {
		return 0, false, nil, err
	}
	result, err := e.callAt(context.Background(), data, blockTag)
	if err != nil {
		return 0, false, nil, err
	}
	return decodeEVMCollaboratorPage(result)
}

func collaboratorRoleValue(role string) (uint64, error) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "none":
		return 0, nil
	case "maintainer":
		return 1, nil
	case "reader":
		return 2, nil
	default:
		return 0, fmt.Errorf("invalid collaborator role %q (maintainer|reader|none)", role)
	}
}

func collaboratorRoleName(role uint64) (string, error) {
	switch role {
	case 1:
		return "maintainer", nil
	case 2:
		return "reader", nil
	default:
		return "", fmt.Errorf("invalid collaborator role value %d", role)
	}
}

func (e *EVMRegistryV2) ResolveUsername(name string) (string, error) {
	if e.legacy == nil {
		return "", ErrEVMUnsupportedFeature
	}
	return e.legacy.ResolveUsername(name)
}

var _ OwnershipRegistryBackend = (*EVMRegistryV2)(nil)

func isEVMLocatorNotFound(err error) bool {
	var notFound *LocatorNotFoundError
	return errors.As(err, &notFound)
}

func (e *EVMRegistryV2) send(ctx context.Context, data []byte) error {
	contract, err := e.contract()
	if err != nil {
		return err
	}
	return e.sendTo(ctx, contract, data)
}

// sendTo signs and broadcasts one call to an explicitly reviewed module
// address while preserving the registry's nonce, gas, chain ID, and receipt
// semantics.
func (e *EVMRegistryV2) sendTo(ctx context.Context, contract string, data []byte) error {
	return e.sendToWithValue(ctx, contract, data, "")
}

// sendToWithValue is the shared EVM transaction pipeline for payable modules.
// value is a canonical 0x-prefixed wei quantity; an empty value preserves the
// zero-value behavior used by registry and badge writes.
func (e *EVMRegistryV2) sendToWithValue(ctx context.Context, contract string, data []byte, value string) error {
	if e.signer == nil {
		return ErrEVMSignerUnavailable
	}
	contract, err := normalizeEVMAddress(contract)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, defaultEVMRPCTimeout)
	defer cancel()
	owner, err := e.signer.OwnerAddress()
	if err != nil {
		return fmt.Errorf("resolve EVM signer address: %w", err)
	}
	from, err := normalizeEVMAddress(owner)
	if err != nil {
		return fmt.Errorf("resolve EVM signer address: %w", err)
	}
	chainID, err := e.rpc.ChainID(ctx)
	if err != nil {
		return err
	}
	if expected := e.cfg.EffectiveEVMChainID(); expected != 0 && expected != chainID {
		return fmt.Errorf("EVM chain ID mismatch: RPC reported %d, profile requires %d", chainID, expected)
	}
	nonceManager := e.nonceManager
	if nonceManager == nil {
		nonceManager = newEVMNonceManager()
	}
	nonce, reservation, err := nonceManager.reserve(ctx, e.rpc, chainID, from)
	if err != nil {
		return err
	}
	completed := false
	defer func() {
		if !completed {
			reservation.Release()
		}
	}()
	call := EVMCall{From: from, To: contract, Data: "0x" + hexEncode(data), Value: value}
	gas, err := e.rpc.EstimateGas(ctx, call)
	if err != nil {
		return wrapEVMRPCError(err)
	}
	const gasHeadroom uint64 = 10_000
	gasLimit, err := AdjustEVMGasLimit(gas, gasHeadroom)
	if err != nil {
		return err
	}
	gasPrice, err := e.rpc.GasPrice(ctx)
	if err != nil {
		return err
	}
	rawTx, err := e.signer.SignTransaction(ctx, EVMTransaction{
		ChainID:  chainID,
		Nonce:    nonce,
		To:       contract,
		Data:     "0x" + hexEncode(data),
		GasLimit: gasLimit,
		GasPrice: gasPrice,
		Value:    value,
	})
	if err != nil {
		return fmt.Errorf("sign EVM transaction: %w", err)
	}
	if _, err := decodeHexBytes(rawTx); err != nil {
		return fmt.Errorf("signer returned invalid raw EVM transaction: %w", err)
	}
	hash, err := e.rpc.SendRawTransaction(ctx, rawTx)
	if err != nil {
		return wrapEVMRPCError(err)
	}
	_, err = e.rpc.WaitReceipt(ctx, hash)
	if err != nil {
		return err
	}
	reservation.Commit()
	completed = true
	return nil
}

func wrapEVMRPCError(err error) error {
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Data == "" {
		return err
	}
	decoded := decodeContractRevert(rpcErr.Data)
	if decoded == nil {
		return err
	}
	return &EVMContractRevertError{RPC: rpcErr, Revert: decoded}
}

// EVMContractRevertError preserves both the JSON-RPC context and the typed
// Solidity error. errors.As can therefore classify RepoMoved and locator
// misses without brittle string matching.
type EVMContractRevertError struct {
	RPC    *RPCError
	Revert error
}

func (e *EVMContractRevertError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v: %v", e.RPC, e.Revert)
}

func (e *EVMContractRevertError) Unwrap() []error {
	if e == nil {
		return nil
	}
	return []error{e.RPC, e.Revert}
}

func hexEncode(data []byte) string {
	const digits = "0123456789abcdef"
	encoded := make([]byte, len(data)*2)
	for i, value := range data {
		encoded[i*2] = digits[value>>4]
		encoded[i*2+1] = digits[value&15]
	}
	return string(encoded)
}

// EVMTransfer keeps the TransferBackend boundary explicit while generic
// Cosmos messages remain unsupported. Typed registry operations use
// EVMRegistryV2.send directly and therefore still get receipt handling.
type EVMTransfer struct {
	registry *EVMRegistryV2
}

func NewEVMTransfer(cfg config.Config) *EVMTransfer {
	return &EVMTransfer{registry: NewEVMRegistryV2(cfg)}
}

func (t *EVMTransfer) Execute(any) error {
	return fmt.Errorf("generic transfer messages are not supported by EVM V2; use a typed registry operation")
}

func (t *EVMTransfer) ExecuteWithFunds(any, string) error {
	return fmt.Errorf("generic funded messages are not supported by EVM V2; use a typed registry operation")
}

var _ TransferBackend = (*EVMTransfer)(nil)
