package successor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// Backend is the transport seam of the successor client. Production wiring
// goes through the existing EVMTransactor semantics (nonce ownership, legacy
// signing, bounded receipt polling, uncertain receipts returning the tx hash);
// tests inject the fake chain. Call executes a read, Send broadcasts a write
// and returns the transaction hash.
type Backend interface {
	Call(ctx context.Context, calldata []byte) ([]byte, error)
	Send(ctx context.Context, calldata []byte) (txHash string, err error)
}

// Client encodes and decodes successor RepositoryCore calls through the
// checked-in ABI only. No selectors or word layouts are hand-coded.
type Client struct {
	backend Backend
}

func NewClient(backend Backend) (*Client, error) {
	if backend == nil {
		return nil, errors.New("successor: backend is nil")
	}
	return &Client{backend: backend}, nil
}

// GetRef reads the full commitment state of one ref.
func (c *Client) GetRef(ctx context.Context, repoID [32]byte, refName string) (RefState, error) {
	abi, err := CoreABI()
	if err != nil {
		return RefState{}, err
	}
	data, err := abi.Pack("getRef", repoID, refName)
	if err != nil {
		return RefState{}, fmt.Errorf("encode getRef: %w", err)
	}
	output, err := c.backend.Call(ctx, data)
	if err != nil {
		return RefState{}, err
	}
	values, err := abi.Unpack("getRef", output)
	if err != nil {
		return RefState{}, fmt.Errorf("decode getRef: %w", err)
	}
	raw, err := tuple[gitRefABI](values[0])
	if err != nil {
		return RefState{}, err
	}
	return raw.state()
}

// UpdateRef broadcasts a commitment publish under revision CAS. It returns the
// transaction hash; receipt handling stays with the backend/transactor. force
// is forwarded for API symmetry and never waives the on-chain CAS.
func (c *Client) UpdateRef(
	ctx context.Context,
	repoID [32]byte,
	refName string,
	commitSHA string,
	commitment Commitment,
	expectedRevision uint64,
	expectedDigest [32]byte,
	force bool,
) (string, error) {
	if err := commitment.Valid(); err != nil {
		return "", err
	}
	abi, err := CoreABI()
	if err != nil {
		return "", err
	}
	data, err := abi.Pack(
		"updateRef",
		repoID,
		refName,
		commitSHA,
		commitment.ManifestDigest,
		new(big.Int).SetUint64(commitment.ManifestSize),
		commitment.BootstrapLocator,
		expectedRevision,
		expectedDigest,
		force,
	)
	if err != nil {
		return "", fmt.Errorf("encode updateRef: %w", err)
	}
	return c.backend.Send(ctx, data)
}

// DeleteRef broadcasts the tombstoning delete of one ref.
func (c *Client) DeleteRef(ctx context.Context, repoID [32]byte, refName string) (string, error) {
	abi, err := CoreABI()
	if err != nil {
		return "", err
	}
	data, err := abi.Pack("deleteRef", repoID, refName)
	if err != nil {
		return "", fmt.Errorf("encode deleteRef: %w", err)
	}
	return c.backend.Send(ctx, data)
}

// gitRefABI mirrors the compiled struct layout of RepositoryCore.GitRef.
type gitRefABI struct {
	ManifestDigest   [32]byte
	ManifestSize     *big.Int
	BootstrapLocator string
	Revision         uint64
	UpdatedAt        uint64
	UpdatedBy        common.Address
	Exists           bool
}

func (r gitRefABI) state() (RefState, error) {
	if !r.Exists {
		return RefState{}, ErrRefNotFound
	}
	size, err := boundedSize(r.ManifestSize)
	if err != nil {
		return RefState{}, err
	}
	return RefState{
		Commitment: Commitment{
			ManifestDigest:   r.ManifestDigest,
			ManifestSize:     size,
			BootstrapLocator: r.BootstrapLocator,
		},
		Revision:  r.Revision,
		UpdatedAt: r.UpdatedAt,
		UpdatedBy: r.UpdatedBy,
	}, nil
}

func boundedSize(value *big.Int) (uint64, error) {
	if value == nil || !value.IsUint64() || value.Uint64() > MaxManifestSize {
		return 0, fmt.Errorf("successor: manifest size %v outside 1..%d", value, MaxManifestSize)
	}
	return value.Uint64(), nil
}

func tuple[T any](value any) (T, error) {
	var zero T
	converted := gethabi.ConvertType(value, new(T))
	pointer, ok := converted.(*T)
	if !ok || pointer == nil {
		return zero, fmt.Errorf("ABI tuple has type %T, want %T", value, zero)
	}
	return *pointer, nil
}

// DecodeRevert maps solidity revert payloads of the successor core to typed
// client errors using the checked-in ABI error table.
func DecodeRevert(data []byte) error {
	abi, err := CoreABI()
	if err != nil {
		return err
	}
	if len(data) < 4 {
		return fmt.Errorf("successor: undecodable revert payload (%d bytes)", len(data))
	}
	var selector [4]byte
	copy(selector[:], data[:4])
	if abiErr, ok := abi.Errors["CommitmentMismatch"]; ok && bytes.Equal(data[:4], abiErr.ID[:4]) {
		values, err := abiErr.Inputs.Unpack(data[4:])
		if err != nil || len(values) != 4 {
			return fmt.Errorf("successor: malformed CommitmentMismatch revert")
		}
		mismatch := &CommitmentMismatchError{}
		ok := true
		mismatch.ExpectedRevision, ok = uint64Field(values[0])
		if ok {
			mismatch.ExpectedDigest, ok = digestField(values[1])
		}
		if ok {
			mismatch.ActualRevision, ok = uint64Field(values[2])
		}
		if ok {
			mismatch.ActualDigest, ok = digestField(values[3])
		}
		if !ok {
			return fmt.Errorf("successor: malformed CommitmentMismatch revert fields")
		}
		return mismatch
	}
	if abiErr, ok := abi.Errors["RefNotFound"]; ok && bytes.Equal(data[:4], abiErr.ID[:4]) {
		return ErrRefNotFound
	}
	return fmt.Errorf("successor: core revert %x", data)
}

func uint64Field(value any) (uint64, bool) {
	switch v := value.(type) {
	case uint64:
		return v, true
	case *uint64:
		if v == nil {
			return 0, false
		}
		return *v, true
	}
	return 0, false
}

func digestField(value any) ([32]byte, bool) {
	switch v := value.(type) {
	case [32]byte:
		return v, true
	}
	return [32]byte{}, false
}
