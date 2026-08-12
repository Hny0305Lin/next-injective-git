package chain

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

// RepoRegistryBackend is the chain-neutral surface used by the Git remote
// helper and repository commands. Implementations own the protocol details
// (CosmWasm messages, EVM calldata, query transport, and address conversion).
//
// The methods deliberately use the existing repository domain types so callers
// do not need to know which contract version produced a result.
type RepoRegistryBackend interface {
	ResolveRepo(owner, repo string) (*ResolvedRepo, error)
	ListRepos(owner string) ([]RepoInfo, error)
	ListRefs(owner, repo string) ([]RefInfo, error)
	RepoInfo(owner, repo string) (*RepoInfo, error)
	ResolveRef(owner, repo, refName string) (string, []string, error)
	ListCollaborators(owner, repo string) ([]CollaboratorInfo, error)
	CreateRepo(name, description, defaultBranch string) error
	UpdateRepoInfo(repo string, description, defaultBranch *string) error
	UpdateRef(owner, repo, refName, commitSHA string, packURIs []string, expectedSHA string, force bool) error
	DeleteRef(owner, repo, refName string) error
	SetCollaborator(owner, repo, collaborator, role string) error
	ResolveUsername(name string) (string, error)
}

type RepoLocator struct {
	Owner string
	Name  string
}

type ResolvedRepo struct {
	RepoID      [32]byte
	Backend     BackendKind
	Requested   RepoLocator
	Canonical   RepoLocator
	IsCanonical bool
	// WriteDisabled is true when an EVM-selected backend resolved this
	// repository through the legacy V1 read adapter. Callers must reject writes
	// before packing, uploading, signing, or broadcasting.
	WriteDisabled bool
	Info          RepoInfo
}

func (r *ResolvedRepo) CanonicalURL() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("igit://%s/%s", r.Canonical.Owner, r.Canonical.Name)
}

var ErrOwnershipOperationUnsupported = errors.New("ownership operation is not supported by the selected registry backend")

// OwnershipRegistryBackend is the chain-neutral ownership-transfer surface.
// URL-based callers resolve a repository once, then pass the bound identity to
// every operation. EVM implementations use RepoID; the V1 adapter uses the
// canonical locator retained in the same value.
type OwnershipRegistryBackend interface {
	BeginOwnershipTransfer(repo *ResolvedRepo, newOwner string) error
	CancelOwnershipTransfer(repo *ResolvedRepo) error
	RejectOwnershipTransfer(repo *ResolvedRepo) error
	ExpireOwnershipTransfer(repo *ResolvedRepo) error
	AcceptOwnership(repo *ResolvedRepo) error
	PendingOwnershipTransfer(repo *ResolvedRepo) (*OwnershipTransferInfo, error)
}

// BadgeBackend is the chain-neutral contribution badge surface. V2 stores
// badges in a separate module keyed by stable repo ID; V1 adapts its legacy
// owner/name indexes behind the same methods.
type BadgeBackend interface {
	AwardBadge(owner, repo, recipient, reason string) error
	BadgesByRecipient(recipient string) ([]Badge, error)
	BadgesByRepo(owner, repo string) ([]Badge, error)
}

// EconomicBackend is the chain-neutral sponsorship and revenue-split surface.
// V2 stores state by stable repo ID in its independent economic module; V1
// adapts the legacy owner/name messages behind this boundary.
type EconomicBackend interface {
	Sponsor(owner, repo, message, amount string) error
	SetRevenueSplits(owner, repo string, splits []SplitEntry) error
	RevenueSplits(owner, repo string) ([]SplitEntry, error)
	SponsorTotals(owner, repo string) ([]Coin, error)
}

// ModerationBackend is the chain-neutral report, appeal, and decision surface.
// Report IDs are backend-local, so ID-only methods always query the selected
// profile and never guess or fall back across V1 and V2.
type ModerationBackend interface {
	SetModerationStatus(owner, repo, status, reasonHash string) error
	SubmitModerationReport(owner, repo, reasonHash string) error
	ResolveModerationReport(id uint64, status, reasonHash string) error
	AppealModerationReport(id uint64, reasonHash string) error
	ResolveModerationAppeal(id uint64, status, reasonHash string) error
	ModerationReport(id uint64) (*ModerationReportInfo, error)
}

// SignerBackend resolves the configured account address. The address returned
// here is the canonical user-facing address (currently inj1...); an EVM
// implementation can perform its internal 20-byte conversion behind this
// boundary.
type SignerBackend interface {
	OwnerAddress() (string, error)
	CreateKey(name string) error
}

// TransferBackend is the transaction transport used by registry backends.
// Message construction remains owned by RepoRegistryBackend implementations;
// this interface only describes signing/broadcasting and receipt-level errors.
type TransferBackend interface {
	Execute(execMsg any) error
	ExecuteWithFunds(execMsg any, amount string) error
}

// CosmWasmRegistryV1 is the legacy registry adapter. Embedding Client keeps
// every existing V1 command available while making the selected backend
// explicit to new callers.
type CosmWasmRegistryV1 struct {
	*Client
}

// NewCosmWasmRegistryV1 creates the read/write legacy adapter.
func NewCosmWasmRegistryV1(cfg config.Config) *CosmWasmRegistryV1 {
	return &CosmWasmRegistryV1{Client: New(cfg)}
}

func (c *CosmWasmRegistryV1) ResolveRepo(owner, repo string) (*ResolvedRepo, error) {
	info, err := c.RepoInfo(owner, repo)
	if err != nil {
		return nil, err
	}
	return &ResolvedRepo{
		Backend:     BackendCosmWasm,
		Requested:   RepoLocator{Owner: owner, Name: repo},
		Canonical:   RepoLocator{Owner: info.Owner, Name: info.Name},
		IsCanonical: true,
		Info:        *info,
	}, nil
}

func legacyOwnershipLocator(repo *ResolvedRepo) (RepoLocator, error) {
	if repo == nil {
		return RepoLocator{}, fmt.Errorf("resolved repository is nil")
	}
	if repo.Backend != BackendCosmWasm {
		return RepoLocator{}, fmt.Errorf("resolved repository belongs to %s, not CosmWasm V1", repo.Backend)
	}
	if !repo.IsCanonical {
		return RepoLocator{}, &RepoMovedError{
			RepoID:       repo.RepoID,
			CurrentOwner: repo.Canonical.Owner,
			Name:         repo.Canonical.Name,
		}
	}
	if strings.TrimSpace(repo.Canonical.Owner) == "" || strings.TrimSpace(repo.Canonical.Name) == "" {
		return RepoLocator{}, fmt.Errorf("resolved CosmWasm repository has an empty canonical locator")
	}
	return repo.Canonical, nil
}

func (c *CosmWasmRegistryV1) BeginOwnershipTransfer(repo *ResolvedRepo, newOwner string) error {
	locator, err := legacyOwnershipLocator(repo)
	if err != nil {
		return err
	}
	return c.Client.TransferOwnership(locator.Name, newOwner)
}

func (c *CosmWasmRegistryV1) CancelOwnershipTransfer(repo *ResolvedRepo) error {
	locator, err := legacyOwnershipLocator(repo)
	if err != nil {
		return err
	}
	return c.Client.CancelOwnershipTransfer(locator.Name)
}

func (c *CosmWasmRegistryV1) RejectOwnershipTransfer(*ResolvedRepo) error {
	return fmt.Errorf("%w: CosmWasm V1 has no target rejection action", ErrOwnershipOperationUnsupported)
}

func (c *CosmWasmRegistryV1) ExpireOwnershipTransfer(*ResolvedRepo) error {
	return fmt.Errorf("%w: CosmWasm V1 has no permissionless expiry action", ErrOwnershipOperationUnsupported)
}

func (c *CosmWasmRegistryV1) AcceptOwnership(repo *ResolvedRepo) error {
	locator, err := legacyOwnershipLocator(repo)
	if err != nil {
		return err
	}
	return c.Client.AcceptOwnership(locator.Owner, locator.Name)
}

func (c *CosmWasmRegistryV1) PendingOwnershipTransfer(repo *ResolvedRepo) (*OwnershipTransferInfo, error) {
	locator, err := legacyOwnershipLocator(repo)
	if err != nil {
		return nil, err
	}
	security, err := c.Client.OwnershipSecurity(locator.Owner, locator.Name)
	if err != nil {
		return nil, err
	}
	return security.Transfer, nil
}

// SetCollaborator adapts the V1 sender-owned message to the unified backend
// contract. V1 derives the repository owner from the signing key, so the
// explicit owner argument is intentionally ignored; the CLI still supplies it
// to keep the V1 and V2 command surface identical.
func (c *CosmWasmRegistryV1) SetCollaborator(_owner, repo, collaborator, role string) error {
	if strings.EqualFold(strings.TrimSpace(role), "none") {
		role = ""
	}
	return c.Client.SetCollaborator(repo, collaborator, role)
}

func (c *CosmWasmRegistryV1) AwardBadge(_owner, repo, recipient, reason string) error {
	return c.Client.AwardBadge(repo, recipient, reason)
}

func (c *CosmWasmRegistryV1) Sponsor(owner, repo, message, amount string) error {
	return c.Client.Sponsor(owner, repo, message, amount)
}

func (c *CosmWasmRegistryV1) SetRevenueSplits(_owner, repo string, splits []SplitEntry) error {
	return c.Client.SetRevenueSplits(repo, splits)
}

func (c *CosmWasmRegistryV1) RevenueSplits(owner, repo string) ([]SplitEntry, error) {
	return c.Client.RevenueSplits(owner, repo)
}

func (c *CosmWasmRegistryV1) SponsorTotals(owner, repo string) ([]Coin, error) {
	return c.Client.SponsorTotals(owner, repo)
}

// CosmosSigner is the legacy Cosmos keyring signer. It is intentionally a
// small adapter around Client so key handling stays behind SignerBackend.
type CosmosSigner struct {
	cfg    config.Config
	client *Client
}

// NewCosmosSigner creates a signer backed by the configured injectived keyring.
func NewCosmosSigner(cfg config.Config) *CosmosSigner {
	return &CosmosSigner{cfg: cfg, client: New(cfg)}
}

func (s *CosmosSigner) OwnerAddress() (string, error) {
	return s.client.OwnerAddress()
}

// CreateKey delegates interactive key creation to the legacy keyring. The
// mnemonic is streamed directly between injectived and the terminal and never
// enters igit's config or logs.
func (s *CosmosSigner) CreateKey(name string) error {
	bin := s.cfg.InjectivedBin
	if bin == "" {
		bin = "injectived"
	}
	cmd := exec.Command(bin, "keys", "add", name, "--keyring-backend", s.cfg.KeyringBackend)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CosmosTransfer is the legacy injectived transaction transport.
type CosmosTransfer struct {
	client *Client
}

// NewCosmosTransfer creates a transfer transport for the configured Cosmos
// network and keyring.
func NewCosmosTransfer(cfg config.Config) *CosmosTransfer {
	return &CosmosTransfer{client: New(cfg)}
}

func (t *CosmosTransfer) Execute(execMsg any) error {
	return t.client.Execute(execMsg)
}

func (t *CosmosTransfer) ExecuteWithFunds(execMsg any, amount string) error {
	return t.client.ExecuteWithFunds(execMsg, amount)
}

var (
	_ RepoRegistryBackend      = (*CosmWasmRegistryV1)(nil)
	_ OwnershipRegistryBackend = (*CosmWasmRegistryV1)(nil)
	_ BadgeBackend             = (*CosmWasmRegistryV1)(nil)
	_ EconomicBackend          = (*CosmWasmRegistryV1)(nil)
	_ ModerationBackend        = (*CosmWasmRegistryV1)(nil)
	_ TransferBackend          = (*CosmWasmRegistryV1)(nil)
	_ SignerBackend            = (*CosmosSigner)(nil)
	_ TransferBackend          = (*CosmosTransfer)(nil)
)

// BackendKind identifies a registry protocol without exposing it in normal
// user-facing command output.
type BackendKind string

const (
	BackendAuto     BackendKind = "auto"
	BackendCosmWasm BackendKind = "cosmwasm"
	BackendEVM      BackendKind = "evm"
)

// UsesEVMBackend reports the effective protocol selected by configuration.
// Keeping this decision in the chain package prevents command frontends from
// accidentally validating or mutating a legacy CosmWasm contract when the
// user selected V2.
func UsesEVMBackend(cfg config.Config) bool {
	backend := strings.ToLower(strings.TrimSpace(cfg.EffectiveContractBackend()))
	return backend == string(BackendEVM) || backend == "v2" ||
		(backend == string(BackendAuto) && cfg.EffectiveContractVersion() == "v2")
}

// SelectRegistryBackend selects the chain implementation from config. The
// selector is intentionally conservative during the migration: legacy config
// files and "auto" + v1 continue to use V1. Explicit EVM/V2 requests select
// the JSON-RPC backend; writes still fail safely until a secure EVMSigner is
// supplied.
func SelectRegistryBackend(cfg config.Config) (RepoRegistryBackend, error) {
	kind := BackendKind(strings.ToLower(strings.TrimSpace(cfg.EffectiveContractBackend())))
	if kind == "" || kind == BackendAuto {
		switch strings.ToLower(strings.TrimSpace(cfg.EffectiveContractVersion())) {
		case "", "v1":
			kind = BackendCosmWasm
		case "v2":
			kind = BackendEVM
		default:
			return nil, fmt.Errorf("unknown contract version %q (expected v1 or v2)", cfg.EffectiveContractVersion())
		}
	}
	switch kind {
	case BackendCosmWasm, "v1":
		return NewCosmWasmRegistryV1(cfg), nil
	case BackendEVM, "v2":
		return NewEVMRegistryV2(cfg), nil
	default:
		return nil, fmt.Errorf("unknown contract backend %q (expected auto, cosmwasm, or evm)", kind)
	}
}

// NewRegistryBackend is the short constructor used by transport callers.
func NewRegistryBackend(cfg config.Config) (RepoRegistryBackend, error) {
	return SelectRegistryBackend(cfg)
}

// NewBadgeBackend returns the badge implementation paired with the selected
// registry backend. Explicit V2 selection never falls back to the V1 writer.
func NewBadgeBackend(cfg config.Config) (BadgeBackend, error) {
	backend, err := SelectRegistryBackend(cfg)
	if err != nil {
		return nil, err
	}
	switch selected := backend.(type) {
	case *CosmWasmRegistryV1:
		return selected, nil
	case *EVMRegistryV2:
		return NewEVMBadgeModuleWithDependencies(cfg, selected, selected.rpc, selected.signer), nil
	default:
		return nil, fmt.Errorf("selected registry backend %T does not support badges", backend)
	}
}

// NewEconomicBackend returns the sponsorship module paired with the selected
// registry backend. Explicit V2 selection never falls back to a V1 writer.
func NewEconomicBackend(cfg config.Config) (EconomicBackend, error) {
	backend, err := SelectRegistryBackend(cfg)
	if err != nil {
		return nil, err
	}
	switch selected := backend.(type) {
	case *CosmWasmRegistryV1:
		return selected, nil
	case *EVMRegistryV2:
		return NewEVMEconomicModuleWithDependencies(cfg, selected, selected.rpc, selected.signer), nil
	default:
		return nil, fmt.Errorf("selected registry backend %T does not support economic operations", backend)
	}
}

// NewModerationBackend returns the selected moderation implementation.
// Explicit V2 selection never constructs or falls back to a V1 write. Unlike
// locator reads, report-ID queries are also backend-local and never fallback.
func NewModerationBackend(cfg config.Config) (ModerationBackend, error) {
	backend, err := SelectRegistryBackend(cfg)
	if err != nil {
		return nil, err
	}
	switch selected := backend.(type) {
	case *CosmWasmRegistryV1:
		return selected, nil
	case *EVMRegistryV2:
		return NewEVMModerationModuleWithDependencies(cfg, selected, selected.rpc, selected.signer), nil
	default:
		return nil, fmt.Errorf("selected registry backend %T does not support moderation", backend)
	}
}

// NewSignerBackend returns the configured signer abstraction. V1 keeps using
// the injectived keyring; V2 uses an encrypted local EVM keystore.
func NewSignerBackend(cfg config.Config) (SignerBackend, error) {
	kind := BackendKind(strings.ToLower(strings.TrimSpace(cfg.EffectiveContractBackend())))
	if kind == "" || kind == BackendAuto {
		kind = BackendKind(strings.ToLower(strings.TrimSpace(cfg.EffectiveContractVersion())))
	}
	switch kind {
	case BackendCosmWasm, "v1", "":
		return NewCosmosSigner(cfg), nil
	case BackendEVM, "v2":
		return NewEVMKeystoreSigner(cfg), nil
	default:
		return nil, fmt.Errorf("unknown signer backend %q", kind)
	}
}

// NewTransferBackend returns the selected transaction transport. Keeping this
// constructor separate lets future EVM code provide a JSON-RPC transfer path
// without teaching the remote helper about signing details.
func NewTransferBackend(cfg config.Config) (TransferBackend, error) {
	kind := BackendKind(strings.ToLower(strings.TrimSpace(cfg.EffectiveContractBackend())))
	if kind == "" || kind == BackendAuto {
		kind = BackendKind(strings.ToLower(strings.TrimSpace(cfg.EffectiveContractVersion())))
	}
	switch kind {
	case BackendCosmWasm, "v1", "":
		return NewCosmosTransfer(cfg), nil
	case BackendEVM, "v2":
		return NewEVMTransfer(cfg), nil
	default:
		return nil, fmt.Errorf("unknown transfer backend %q", kind)
	}
}
