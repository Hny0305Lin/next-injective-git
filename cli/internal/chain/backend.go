package chain

import (
	"errors"
	"fmt"

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
	Info        RepoInfo
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

// RecoveryBackend owns guardian configuration and delayed ownership recovery.
// It is deliberately separate from Core's normal ownership-transfer surface:
// RepositoryCore accepts recovered ownership only from this module capability.
type RecoveryBackend interface {
	SetGuardians(repo string, guardians []string, threshold uint8) error
	ProposeRecovery(owner, repo, newOwner string) error
	ApproveRecovery(owner, repo string) error
	CancelRecovery(repo string) error
	AcceptRecovery(owner, repo string) error
	OwnershipSecurity(owner, repo string) (*OwnershipSecurityInfo, error)
}

// UsernameBackend exposes the suite's liability-free username registry. V1
// deposit refunding belongs to the archive/cutover workflow, not this surface.
type UsernameBackend interface {
	RegisterUsername(name string) error
	ClaimOriginalUsername(name string) error
	ReleaseUsername() error
	ResolveUsername(name string) (string, error)
	AddressUsername(address string) (string, error)
}

// ReleaseBackend registers and reads immutable release artifact checksums.
type ReleaseBackend interface {
	RegisterRelease(version string, artifacts []ReleaseArtifact) error
	ReleaseArtifacts(version string) ([]ReleaseArtifact, error)
}

// ForkBackend creates a bounded fork using Core's stable repository identity.
type ForkBackend interface {
	ForkRepo(owner, repo, newName string) error
}

// SignerBackend resolves the configured account address. The address returned
// here is the canonical user-facing address (currently inj1...); an EVM
// implementation can perform its internal 20-byte conversion behind this
// boundary.
type SignerBackend interface {
	OwnerAddress() (string, error)
	CreateKey(name string) error
}

// KeyImporter is implemented by signers that can encrypt an existing private
// key into their local keystore. It is separate from SignerBackend so remote
// and migration test signers do not need to accept key material.
type KeyImporter interface {
	ImportKey(name string) error
}

// BackendKind identifies a registry protocol without exposing it in normal
// user-facing command output.
type BackendKind string

const (
	BackendEVM BackendKind = "evm"
)

// UsesEVMBackend reports the effective protocol selected by configuration.
// Keeping this decision in the chain package prevents command frontends from
// accidentally validating or mutating a legacy CosmWasm contract when the
// user selected V2.
func UsesEVMBackend(_ config.Config) bool {
	return true
}

// SelectRegistryBackend always selects the immutable EVM suite. Archived V1
// access lives in internal/archivev1 and is not linked through this selector.
func SelectRegistryBackend(cfg config.Config) (RepoRegistryBackend, error) {
	return NewEVMSuiteRegistry(cfg), nil
}

// NewRegistryBackend is the short constructor used by transport callers.
func NewRegistryBackend(cfg config.Config) (RepoRegistryBackend, error) {
	return SelectRegistryBackend(cfg)
}

// NewBadgeBackend returns the badge implementation paired with the selected
// registry backend. Explicit V2 selection never falls back to the V1 writer.
func NewBadgeBackend(cfg config.Config) (BadgeBackend, error) {
	return NewEVMSuiteRegistry(cfg), nil
}

// NewEconomicBackend returns the sponsorship module paired with the selected
// registry backend. Explicit V2 selection never falls back to a V1 writer.
func NewEconomicBackend(cfg config.Config) (EconomicBackend, error) {
	return NewEVMSuiteRegistry(cfg), nil
}

// NewModerationBackend returns the selected moderation implementation.
// Explicit V2 selection never constructs or falls back to a V1 write. Unlike
// locator reads, report-ID queries are also backend-local and never fallback.
func NewModerationBackend(cfg config.Config) (ModerationBackend, error) {
	return NewEVMSuiteRegistry(cfg), nil
}

func NewRecoveryBackend(cfg config.Config) (RecoveryBackend, error) {
	return NewEVMSuiteRegistry(cfg), nil
}

func NewUsernameBackend(cfg config.Config) (UsernameBackend, error) {
	return NewEVMSuiteRegistry(cfg), nil
}

func NewReleaseBackend(cfg config.Config) (ReleaseBackend, error) {
	return NewEVMSuiteRegistry(cfg), nil
}

func NewForkBackend(cfg config.Config) (ForkBackend, error) {
	return NewEVMSuiteRegistry(cfg), nil
}

// NewSignerBackend returns the configured signer abstraction. V1 keeps using
// the injectived keyring; V2 uses an encrypted local EVM keystore.
func NewSignerBackend(cfg config.Config) (SignerBackend, error) {
	return NewEVMKeystoreSigner(cfg), nil
}
