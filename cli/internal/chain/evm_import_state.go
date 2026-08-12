package chain

import (
	"context"
	"fmt"
	"strings"
)

// EVMImportProgress mirrors RepoRegistryV2.importProgress. The Solidity values
// are uint256, but the planner bounds every count to values that fit uint64;
// the ABI decoder fails closed if a response exceeds that range.
type EVMImportProgress struct {
	Exists                 bool
	Active                 bool
	Finalized              bool
	NextSequence           uint64
	ExpectedBatches        uint64
	ImportedRepositories   uint64
	ExpectedRepositories   uint64
	ImportedRefs           uint64
	RemainingRefs          uint64
	ImportedCollaborators  uint64
	RemainingCollaborators uint64
	IncompleteRepositories uint64
}

// ChainID exposes the read-only RPC identity check used by migration tooling.
func (e *EVMRegistryV2) ChainID(ctx context.Context) (uint64, error) {
	if _, err := e.contract(); err != nil {
		return 0, err
	}
	chainID, err := e.rpc.ChainID(ctx)
	if err != nil {
		return 0, wrapEVMRPCError(err)
	}
	return chainID, nil
}

// TransactionReceipt returns one receipt without polling or sending a
// transaction. A post-import exporter requires an already-mined finalize hash.
func (e *EVMRegistryV2) TransactionReceipt(ctx context.Context, hash string) (*EVMReceipt, error) {
	if _, err := e.contract(); err != nil {
		return nil, err
	}
	receipt, err := e.rpc.TransactionReceipt(ctx, hash)
	if err != nil {
		return nil, wrapEVMRPCError(err)
	}
	return receipt, nil
}

func (e *EVMRegistryV2) BlockByNumber(ctx context.Context, blockTag string) (*EVMBlock, error) {
	if _, err := e.contract(); err != nil {
		return nil, err
	}
	if err := requireFixedEVMBlockTag(blockTag); err != nil {
		return nil, err
	}
	block, err := e.rpc.BlockByNumber(ctx, blockTag)
	if err != nil {
		return nil, wrapEVMRPCError(err)
	}
	return block, nil
}

func (e *EVMRegistryV2) ImportProgressAt(
	ctx context.Context,
	sessionID [32]byte,
	blockTag string,
) (*EVMImportProgress, error) {
	if err := requireFixedEVMBlockTag(blockTag); err != nil {
		return nil, fmt.Errorf("import progress: %w", err)
	}
	data, err := encodeABICall("importProgress(bytes32)", abiBytes32Value(sessionID))
	if err != nil {
		return nil, err
	}
	result, err := e.callAt(ctx, data, blockTag)
	if err != nil {
		return nil, err
	}
	return decodeEVMImportProgress(result)
}

func (e *EVMRegistryV2) GetRepoByIDAt(
	ctx context.Context,
	repoID [32]byte,
	blockTag string,
) (*RepoInfo, error) {
	if err := requireFixedEVMBlockTag(blockTag); err != nil {
		return nil, fmt.Errorf("repository state read: %w", err)
	}
	data, err := encodeABICall("getRepoById(bytes32)", abiBytes32Value(repoID))
	if err != nil {
		return nil, err
	}
	result, err := e.callAt(ctx, data, blockTag)
	if err != nil {
		return nil, err
	}
	return decodeEVMRepoInfo(result)
}

func (e *EVMRegistryV2) ListRefsByIDAt(
	ctx context.Context,
	repoID [32]byte,
	blockTag string,
) ([]RefInfo, error) {
	if err := requireFixedEVMBlockTag(blockTag); err != nil {
		return nil, fmt.Errorf("ref state read: %w", err)
	}
	refs := make([]RefInfo, 0)
	var cursor uint64
	for {
		data, err := encodeABICall(
			"listRefsPageById(bytes32,uint256,uint256)",
			abiBytes32Value(repoID),
			abiUintValue(cursor),
			abiUintValue(evmQueryPageSize),
		)
		if err != nil {
			return nil, err
		}
		result, err := e.callAt(ctx, data, blockTag)
		if err != nil {
			return nil, err
		}
		nextCursor, hasMore, page, err := decodeEVMRefPage(result)
		if err != nil {
			return nil, err
		}
		refs = append(refs, page...)
		if !hasMore {
			return refs, nil
		}
		if nextCursor <= cursor {
			return nil, fmt.Errorf("EVM import ref page cursor did not advance from %d", cursor)
		}
		cursor = nextCursor
	}
}

func (e *EVMRegistryV2) ListCollaboratorsByIDAt(
	ctx context.Context,
	repoID [32]byte,
	blockTag string,
) ([]CollaboratorInfo, error) {
	if err := requireFixedEVMBlockTag(blockTag); err != nil {
		return nil, fmt.Errorf("collaborator state read: %w", err)
	}
	values := make([]CollaboratorInfo, 0)
	var cursor uint64
	for {
		data, err := encodeABICall(
			"listCollaboratorsPageById(bytes32,uint256,uint256)",
			abiBytes32Value(repoID),
			abiUintValue(cursor),
			abiUintValue(evmQueryPageSize),
		)
		if err != nil {
			return nil, err
		}
		result, err := e.callAt(ctx, data, blockTag)
		if err != nil {
			return nil, err
		}
		nextCursor, hasMore, page, err := decodeEVMCollaboratorPage(result)
		if err != nil {
			return nil, err
		}
		values = append(values, page...)
		if !hasMore {
			return values, nil
		}
		if nextCursor <= cursor {
			return nil, fmt.Errorf("EVM import collaborator page cursor did not advance from %d", cursor)
		}
		cursor = nextCursor
	}
}

func decodeEVMImportProgress(data []byte) (*EVMImportProgress, error) {
	const words = 12
	if len(data) != words*32 {
		return nil, fmt.Errorf("EVM import progress returned %d bytes, want %d", len(data), words*32)
	}
	exists, err := readABIBool(data, 0)
	if err != nil {
		return nil, fmt.Errorf("decode import progress exists: %w", err)
	}
	active, err := readABIBool(data, 32)
	if err != nil {
		return nil, fmt.Errorf("decode import progress active: %w", err)
	}
	finalized, err := readABIBool(data, 64)
	if err != nil {
		return nil, fmt.Errorf("decode import progress finalized: %w", err)
	}
	values := make([]uint64, 9)
	for index := range values {
		value, err := readABIUint(data, (index+3)*32)
		if err != nil {
			return nil, fmt.Errorf("decode import progress word %d: %w", index+3, err)
		}
		values[index] = value
	}
	return &EVMImportProgress{
		Exists: exists, Active: active, Finalized: finalized,
		NextSequence: values[0], ExpectedBatches: values[1],
		ImportedRepositories: values[2], ExpectedRepositories: values[3],
		ImportedRefs: values[4], RemainingRefs: values[5],
		ImportedCollaborators: values[6], RemainingCollaborators: values[7],
		IncompleteRepositories: values[8],
	}, nil
}

func requireFixedEVMBlockTag(blockTag string) error {
	if blockTag == "" || blockTag != strings.TrimSpace(blockTag) ||
		!strings.HasPrefix(blockTag, "0x") || len(blockTag) == 2 {
		return fmt.Errorf("fixed block tag must be a lowercase hexadecimal JSON-RPC quantity")
	}
	digits := blockTag[2:]
	if len(digits) > 1 && digits[0] == '0' {
		return fmt.Errorf("fixed block tag must use canonical JSON-RPC quantity encoding")
	}
	for _, digit := range digits {
		if (digit < '0' || digit > '9') && (digit < 'a' || digit > 'f') {
			return fmt.Errorf("fixed block tag must be a lowercase hexadecimal JSON-RPC quantity")
		}
	}
	return nil
}

func readABIBool(data []byte, offset int) (bool, error) {
	word, err := readABIWord(data, offset)
	if err != nil {
		return false, err
	}
	for _, value := range word[:31] {
		if value != 0 {
			return false, fmt.Errorf("ABI bool at offset %d has non-zero padding", offset)
		}
	}
	if word[31] > 1 {
		return false, fmt.Errorf("ABI bool at offset %d has invalid value %d", offset, word[31])
	}
	return word[31] == 1, nil
}
