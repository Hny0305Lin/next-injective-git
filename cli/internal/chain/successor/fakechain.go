package successor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// RefUpdatedEvent is the decoded v2 ref event. The full refName is part of the
// event data (not an indexed hash), so indexers can restore it without extra
// storage lookups.
type RefUpdatedEvent struct {
	RepoID    [32]byte
	RefName   string
	CommitSHA string
	Digest    [32]byte
	Size      uint64
	Locator   string
	Revision  uint64
	UpdatedBy common.Address
}

type fakeRef struct {
	exists    bool
	digest    [32]byte
	size      uint64
	locator   string
	revision  uint64
	updatedAt uint64
	updatedBy common.Address
}

type fakeRepo struct {
	id         [32]byte
	owner      common.Address
	name       string
	forkedFrom [32]byte
	refs       map[string]*fakeRef
}

type pendingSend struct {
	apply func() error
	label string
}

// FakeChain re-implements the successor RepositoryCore ref semantics in Go and
// speaks the real ABI on the wire: every Call/Send is decoded through the
// checked-in ABI, and reverts are produced as ABI-encoded payloads routed
// through DecodeRevert. It exists for local tests only and never touches a
// network.
type FakeChain struct {
	mu        sync.Mutex
	actor     common.Address
	chainID   *big.Int
	directory common.Address
	now       uint64
	repos     map[[32]byte]*fakeRepo
	locators  map[[32]byte][32]byte
	events    []RefUpdatedEvent
	pending   map[string]pendingSend
	nextHash  uint64
	Uncertain bool // when true, Send defers execution until Resolve
}

func NewFakeChain(actor common.Address, chainID *big.Int, directory common.Address) *FakeChain {
	return &FakeChain{
		actor: actor, chainID: chainID, directory: directory, now: 1,
		repos: make(map[[32]byte]*fakeRepo), locators: make(map[[32]byte][32]byte),
		pending: make(map[string]pendingSend),
	}
}

func (f *FakeChain) repoID(owner common.Address, name string) [32]byte {
	return crypto.Keccak256Hash([]byte("igit:suite:v3:repo"), common.LeftPadBytes(f.chainID.Bytes(), 32),
		f.directory.Bytes(), owner.Bytes(), []byte(name))
}

// CreateRepository mirrors createRepository for fixture setup.
func (f *FakeChain) CreateRepository(name string) ([32]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	locator := locatorKey(f.actor, name)
	if _, taken := f.locators[locator]; taken {
		return [32]byte{}, errors.New("fake chain: locator unavailable")
	}
	id := f.repoID(f.actor, name)
	if _, exists := f.repos[id]; exists {
		return [32]byte{}, errors.New("fake chain: repository already exists")
	}
	f.repos[id] = &fakeRepo{id: id, owner: f.actor, name: name, refs: make(map[string]*fakeRef)}
	f.locators[locator] = id
	return id, nil
}

// ForkRepository mirrors the successor fork: metadata only, no ref commitments.
func (f *FakeChain) ForkRepository(sourceID [32]byte, newName string) ([32]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	source, ok := f.repos[sourceID]
	if !ok {
		return [32]byte{}, errors.New("fake chain: source repository not found")
	}
	locator := locatorKey(f.actor, newName)
	if _, taken := f.locators[locator]; taken {
		return [32]byte{}, errors.New("fake chain: locator unavailable")
	}
	id := f.repoID(f.actor, newName)
	if _, exists := f.repos[id]; exists {
		return [32]byte{}, errors.New("fake chain: repository already exists")
	}
	f.repos[id] = &fakeRepo{id: id, owner: f.actor, name: newName, forkedFrom: source.id, refs: make(map[string]*fakeRef)}
	f.locators[locator] = id
	return id, nil
}

// Refs exposes the live ref table for direct fixture assertions.
func (f *FakeChain) Refs(repoID [32]byte) (map[string]RefState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo, ok := f.repos[repoID]
	if !ok {
		return nil, errors.New("fake chain: repository not found")
	}
	out := make(map[string]RefState, len(repo.refs))
	for name, ref := range repo.refs {
		if !ref.exists {
			continue
		}
		out[name] = RefState{
			Commitment: Commitment{ManifestDigest: ref.digest, ManifestSize: ref.size, BootstrapLocator: ref.locator},
			Revision:   ref.revision, UpdatedAt: ref.updatedAt, UpdatedBy: ref.updatedBy,
		}
	}
	return out, nil
}

// Events returns the recorded RefUpdated log.
func (f *FakeChain) Events() []RefUpdatedEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]RefUpdatedEvent(nil), f.events...)
}

// Resolve settles an uncertain transaction: include applies it, exclude drops
// it. Either way the receipt is no longer unknown.
func (f *FakeChain) Resolve(txHash string, include bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	pending, ok := f.pending[txHash]
	if !ok {
		return fmt.Errorf("fake chain: unknown pending tx %s", txHash)
	}
	delete(f.pending, txHash)
	if !include {
		return nil
	}
	return pending.apply()
}

// PendingCount reports unsettled uncertain transactions.
func (f *FakeChain) PendingCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.pending)
}

func locatorKey(owner common.Address, name string) [32]byte {
	return crypto.Keccak256Hash(owner.Bytes(), []byte(name))
}

func (f *FakeChain) tick() uint64 { f.now++; return f.now }

func (f *FakeChain) revertCommitmentMismatch(expectedRevision, actualRevision uint64, expectedDigest, actualDigest [32]byte) error {
	abi, err := CoreABI()
	if err != nil {
		return err
	}
	abiErr, ok := abi.Errors["CommitmentMismatch"]
	if !ok {
		return errors.New("fake chain: CommitmentMismatch missing from ABI")
	}
	payload, err := abiErr.Inputs.Pack(expectedRevision, expectedDigest, actualRevision, actualDigest)
	if err != nil {
		return fmt.Errorf("fake chain: encode CommitmentMismatch: %w", err)
	}
	return DecodeRevert(append(crypto.Keccak256([]byte("CommitmentMismatch(uint64,bytes32,uint64,bytes32)"))[:4], payload...))
}

func (f *FakeChain) revertRefNotFound(repoID [32]byte, refName string) error {
	abi, err := CoreABI()
	if err != nil {
		return err
	}
	abiErr, ok := abi.Errors["RefNotFound"]
	if !ok {
		return errors.New("fake chain: RefNotFound missing from ABI")
	}
	refID := crypto.Keccak256Hash([]byte(refName))
	payload, err := abiErr.Inputs.Pack(repoID, refID)
	if err != nil {
		return fmt.Errorf("fake chain: encode RefNotFound: %w", err)
	}
	return DecodeRevert(append(crypto.Keccak256([]byte("RefNotFound(bytes32,bytes32)"))[:4], payload...))
}

func (f *FakeChain) applyUpdateRef(repoID [32]byte, refName, commitSHA string, commitment Commitment, expectedRevision uint64, expectedDigest [32]byte) error {
	repo, ok := f.repos[repoID]
	if !ok {
		return errors.New("fake chain: repository not found")
	}
	if err := commitment.Valid(); err != nil {
		return err
	}
	if len(commitSHA) != 40 && len(commitSHA) != 64 {
		return errors.New("fake chain: invalid commit sha")
	}
	target := repo.refs[refName]
	if target == nil {
		target = &fakeRef{}
		repo.refs[refName] = target
	} else if target.exists || target.revision != 0 {
		// existing ref or tombstone: enforce CAS against current storage
		var currentDigest [32]byte
		if target.exists {
			currentDigest = target.digest
		}
		if expectedRevision != target.revision || expectedDigest != currentDigest {
			return f.revertCommitmentMismatch(expectedRevision, target.revision, expectedDigest, currentDigest)
		}
	} else if expectedRevision != 0 || expectedDigest != [32]byte{} {
		return f.revertCommitmentMismatch(expectedRevision, 0, expectedDigest, [32]byte{})
	}
	target.exists = true
	target.revision++
	target.digest = commitment.ManifestDigest
	target.size = commitment.ManifestSize
	target.locator = commitment.BootstrapLocator
	target.updatedAt = f.tick()
	target.updatedBy = f.actor
	f.events = append(f.events, RefUpdatedEvent{
		RepoID: repoID, RefName: refName, CommitSHA: commitSHA,
		Digest: commitment.ManifestDigest, Size: commitment.ManifestSize,
		Locator: commitment.BootstrapLocator, Revision: target.revision, UpdatedBy: f.actor,
	})
	return nil
}

func (f *FakeChain) applyDeleteRef(repoID [32]byte, refName string) error {
	repo, ok := f.repos[repoID]
	if !ok {
		return errors.New("fake chain: repository not found")
	}
	target, ok := repo.refs[refName]
	if !ok || !target.exists {
		return f.revertRefNotFound(repoID, refName)
	}
	target.exists = false
	target.digest = [32]byte{}
	target.size = 0
	target.locator = ""
	target.updatedAt = f.tick()
	target.updatedBy = f.actor
	return nil
}

func (f *FakeChain) Call(_ context.Context, calldata []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	abi, err := CoreABI()
	if err != nil {
		return nil, err
	}
	if len(calldata) < 4 {
		return nil, errors.New("fake chain: short calldata")
	}
	var selector [4]byte
	copy(selector[:], calldata[:4])
	for name, method := range abi.Methods {
		if !bytes.Equal(method.ID[:4], selector[:]) {
			continue
		}
		values, err := method.Inputs.Unpack(calldata[4:])
		if err != nil {
			return nil, fmt.Errorf("fake chain: unpack %s: %w", name, err)
		}
		switch name {
		case "getRef":
			repoID, _ := values[0].([32]byte)
			refName, _ := values[1].(string)
			repo, ok := f.repos[repoID]
			if !ok {
				return nil, errors.New("fake chain: repository not found")
			}
			target, ok := repo.refs[refName]
			if !ok || !target.exists {
				return nil, f.revertRefNotFound(repoID, refName)
			}
			encoded, err := method.Outputs.Pack(gitRefABI{
				ManifestDigest: target.digest, ManifestSize: new(big.Int).SetUint64(target.size), BootstrapLocator: target.locator,
				Revision: target.revision, UpdatedAt: target.updatedAt, UpdatedBy: target.updatedBy, Exists: true,
			})
			if err != nil {
				return nil, fmt.Errorf("fake chain: pack getRef output: %w", err)
			}
			return encoded, nil
		default:
			return nil, fmt.Errorf("fake chain: unsupported read %q", name)
		}
	}
	return nil, errors.New("fake chain: unknown selector")
}

func (f *FakeChain) Send(_ context.Context, calldata []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	abi, err := CoreABI()
	if err != nil {
		return "", err
	}
	if len(calldata) < 4 {
		return "", errors.New("fake chain: short calldata")
	}
	var selector [4]byte
	copy(selector[:], calldata[:4])
	for name, method := range abi.Methods {
		if !bytes.Equal(method.ID[:4], selector[:]) {
			continue
		}
		values, err := method.Inputs.Unpack(calldata[4:])
		if err != nil {
			return "", fmt.Errorf("fake chain: unpack %s: %w", name, err)
		}
		switch name {
		case "updateRef":
			repoID, _ := values[0].([32]byte)
			refName, _ := values[1].(string)
			commitSHA, _ := values[2].(string)
			digest, _ := values[3].([32]byte)
			sizeBig, _ := values[4].(*big.Int)
			if sizeBig == nil || !sizeBig.IsUint64() {
				return "", errors.New("fake chain: manifest size outside uint64")
			}
			size := sizeBig.Uint64()
			locator, _ := values[5].(string)
			expectedRevision, _ := values[6].(uint64)
			expectedDigest, _ := values[7].([32]byte)
			commitment := Commitment{ManifestDigest: digest, ManifestSize: size, BootstrapLocator: locator}
			apply := func() error {
				return f.applyUpdateRef(repoID, refName, commitSHA, commitment, expectedRevision, expectedDigest)
			}
			return f.dispatch(apply, "updateRef")
		case "deleteRef":
			repoID, _ := values[0].([32]byte)
			refName, _ := values[1].(string)
			apply := func() error { return f.applyDeleteRef(repoID, refName) }
			return f.dispatch(apply, "deleteRef")
		default:
			return "", fmt.Errorf("fake chain: unsupported write %q", name)
		}
	}
	return "", errors.New("fake chain: unknown selector")
}

func (f *FakeChain) dispatch(apply func() error, label string) (string, error) {
	f.nextHash++
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("fake-tx-%d-%s", f.nextHash, label))))
	if f.Uncertain {
		f.pending[hash] = pendingSend{apply: apply, label: label}
		return hash, nil
	}
	if err := apply(); err != nil {
		return "", err
	}
	return hash, nil
}

// GetRefPublic is a read helper for cross-package fixtures: it decodes through
// the real ABI so external tests observe exactly what a client would.
func (f *FakeChain) GetRefPublic(repoID [32]byte, refName string) (RefState, error) {
	client, err := NewClient(f)
	if err != nil {
		return RefState{}, err
	}
	return client.GetRef(context.Background(), repoID, refName)
}
