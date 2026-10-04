package successor

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// ErrRefNotFound mirrors the contract RefNotFound(repoId, refId) revert.
var ErrRefNotFound = errors.New("successor: ref not found")

// CommitmentMismatchError decodes CommitmentMismatch(expectedRevision,
// expectedDigest, actualRevision, actualDigest) reverts so callers can
// distinguish a lost CAS race from transport failures and retry with the
// fresh on-chain state.
type CommitmentMismatchError struct {
	ExpectedRevision uint64
	ExpectedDigest   [32]byte
	ActualRevision   uint64
	ActualDigest     [32]byte
}

func (e *CommitmentMismatchError) Error() string {
	return fmt.Sprintf(
		"successor: commitment CAS mismatch: expected revision %d digest %x, chain has revision %d digest %x",
		e.ExpectedRevision, e.ExpectedDigest, e.ActualRevision, e.ActualDigest,
	)
}

// Commitment is the client shape of the on-chain ref commitment. ManifestSize
// is uint96 on-chain but structurally bounded by the contract to
// MANIFEST_MAX_SIZE (64 KiB), so uint64 holds every legal value.
type Commitment struct {
	ManifestDigest   [32]byte
	ManifestSize     uint64
	BootstrapLocator string
}

// RefState is the decoded getRef view. CommitSha is intentionally absent: the
// commit OID is bound by the committed manifest and carried by RefUpdated
// events, not by storage.
type RefState struct {
	Commitment Commitment
	Revision   uint64
	UpdatedAt  uint64
	UpdatedBy  common.Address
}

// Valid performs the same structural checks the contract enforces before any
// upload or ref write, so clients can fail closed before touching the cloud.
func (c Commitment) Valid() error {
	if c.ManifestDigest == [32]byte{} {
		return errors.New("successor: manifest digest is zero")
	}
	if c.ManifestSize == 0 || c.ManifestSize > MaxManifestSize {
		return fmt.Errorf("successor: manifest size %d outside 1..%d", c.ManifestSize, MaxManifestSize)
	}
	if err := validLocator(c.BootstrapLocator); err != nil {
		return err
	}
	return nil
}

// MaxManifestSize mirrors RepositoryCore.MANIFEST_MAX_SIZE and the frozen
// PackManifest schema-1 receiver limit.
const MaxManifestSize uint64 = 65_536

// MaxLocatorLength mirrors RepositoryCore.MAX_LOCATOR_LENGTH.
const MaxLocatorLength = 512

func validLocator(locator string) error {
	if len(locator) < 8 || len(locator) > MaxLocatorLength {
		return fmt.Errorf("successor: bootstrap locator length %d outside 8..%d", len(locator), MaxLocatorLength)
	}
	if locator[:8] != "https://" {
		return errors.New("successor: bootstrap locator must use https")
	}
	return nil
}

// RepositoryView is the successor repository metadata needed by Git flows.
type RepositoryView struct {
	RepoID        [32]byte
	OwnerHex      string
	Name          string
	DefaultBranch string
}