package suitemigration

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type moduleSpec struct {
	name   string
	domain string
	id     common.Hash
}

var requiredModules = func() []moduleSpec {
	values := []moduleSpec{
		{name: "repository-core", domain: "igit.module.repository-core"},
		{name: "recovery", domain: "igit.module.recovery"},
		{name: "moderation", domain: "igit.module.moderation"},
		{name: "economic", domain: "igit.module.economic"},
		{name: "username", domain: "igit.module.username"},
		{name: "badge", domain: "igit.module.badge"},
		{name: "release", domain: "igit.module.release"},
	}
	for index := range values {
		values[index].id = crypto.Keccak256Hash([]byte(values[index].domain))
	}
	return values
}()

type coreRepositoryABI struct {
	ID            [32]byte       `abi:"id"`
	Owner         common.Address `abi:"owner"`
	Name          string         `abi:"name"`
	Description   string         `abi:"description"`
	DefaultBranch string         `abi:"defaultBranch"`
	ForkedFrom    [32]byte       `abi:"forkedFrom"`
	CreatedAt     uint64         `abi:"createdAt"`
	UpdatedAt     uint64         `abi:"updatedAt"`
}

type coreAliasABI struct {
	RepoID [32]byte       `abi:"repoId"`
	Owner  common.Address `abi:"owner"`
	Name   string         `abi:"name"`
}

type coreRefABI struct {
	RepoID    [32]byte       `abi:"repoId"`
	RefName   string         `abi:"refName"`
	CommitSHA string         `abi:"commitSha"`
	PackURIs  []string       `abi:"packUris"`
	UpdatedAt uint64         `abi:"updatedAt"`
	UpdatedBy common.Address `abi:"updatedBy"`
}

type coreCollaboratorABI struct {
	RepoID  [32]byte       `abi:"repoId"`
	Account common.Address `abi:"account"`
	Role    uint8          `abi:"role"`
}

type guardianConfigABI struct {
	RepoID       [32]byte         `abi:"repoId"`
	ConfiguredBy common.Address   `abi:"configuredBy"`
	Threshold    uint8            `abi:"threshold"`
	Guardians    []common.Address `abi:"guardians"`
}

type moderationStatusABI struct {
	RepoID [32]byte `abi:"repoId"`
	Status uint8    `abi:"status"`
}

type moderationReportRecordABI struct {
	ID         *big.Int       `abi:"id"`
	RepoID     [32]byte       `abi:"repoId"`
	Reporter   common.Address `abi:"reporter"`
	Status     uint8          `abi:"status"`
	Resolution uint8          `abi:"resolution"`
	ReasonHash string         `abi:"reasonHash"`
	CreatedAt  uint64         `abi:"createdAt"`
	UpdatedAt  uint64         `abi:"updatedAt"`
	Exists     bool           `abi:"exists"`
}

type moderationReportTrailABI struct {
	Action     uint8          `abi:"action"`
	Actor      common.Address `abi:"actor"`
	Status     uint8          `abi:"status"`
	ReasonHash string         `abi:"reasonHash"`
	Timestamp  uint64         `abi:"timestamp"`
}

type moderationReportABI struct {
	Report moderationReportRecordABI  `abi:"report"`
	Trail  []moderationReportTrailABI `abi:"trail"`
}

type moderationStatusLogABI struct {
	Status     uint8          `abi:"status"`
	Actor      common.Address `abi:"actor"`
	ReasonHash string         `abi:"reasonHash"`
	Timestamp  uint64         `abi:"timestamp"`
	ReportID   *big.Int       `abi:"reportId"`
}

type moderationStatusTrailABI struct {
	RepoID [32]byte                 `abi:"repoId"`
	Trail  []moderationStatusLogABI `abi:"trail"`
}

type economicSplitABI struct {
	Recipient common.Address `abi:"recipient"`
	BPS       uint16         `abi:"bps"`
}

type economicSplitsABI struct {
	RepoID [32]byte           `abi:"repoId"`
	Splits []economicSplitABI `abi:"splits"`
}

type economicTotalABI struct {
	RepoID [32]byte `abi:"repoId"`
	Denom  string   `abi:"denom"`
	Amount *big.Int `abi:"amount"`
}

type usernameOwnerABI struct {
	Name  string         `abi:"name"`
	Owner common.Address `abi:"owner"`
}

type badgeABI struct {
	ID        *big.Int       `abi:"id"`
	RepoID    [32]byte       `abi:"repoId"`
	Recipient common.Address `abi:"recipient"`
	AwardedBy common.Address `abi:"awardedBy"`
	Reason    string         `abi:"reason"`
	AwardedAt uint64         `abi:"awardedAt"`
	Exists    bool           `abi:"exists"`
}

type releaseABI struct {
	Version      string         `abi:"version"`
	Platform     string         `abi:"platform"`
	SHA256       [32]byte       `abi:"sha256"`
	RegisteredBy common.Address `abi:"registeredBy"`
	RegisteredAt uint64         `abi:"registeredAt"`
	Exists       bool           `abi:"exists"`
}

var (
	outerPayloadArguments = abi.Arguments{
		{Type: mustABIType("uint8", nil)},
		{Type: mustABIType("bytes", nil)},
	}
	rollingRootArguments = abi.Arguments{
		{Type: mustABIType("bytes32", nil)},
		{Type: mustABIType("bytes32", nil)},
		{Type: mustABIType("uint256", nil)},
		{Type: mustABIType("uint256", nil)},
		{Type: mustABIType("bytes32", nil)},
	}
	rollingSeedArguments = abi.Arguments{
		{Type: mustABIType("bytes32", nil)},
		{Type: mustABIType("bytes32", nil)},
	}

	coreRepositoryArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "id", Type: "bytes32"}, {Name: "owner", Type: "address"},
		{Name: "name", Type: "string"}, {Name: "description", Type: "string"},
		{Name: "defaultBranch", Type: "string"}, {Name: "forkedFrom", Type: "bytes32"},
		{Name: "createdAt", Type: "uint64"}, {Name: "updatedAt", Type: "uint64"},
	})
	coreAliasArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "repoId", Type: "bytes32"}, {Name: "owner", Type: "address"}, {Name: "name", Type: "string"},
	})
	coreRefArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "repoId", Type: "bytes32"}, {Name: "refName", Type: "string"},
		{Name: "commitSha", Type: "string"}, {Name: "packUris", Type: "string[]"},
		{Name: "updatedAt", Type: "uint64"}, {Name: "updatedBy", Type: "address"},
	})
	coreCollaboratorArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "repoId", Type: "bytes32"}, {Name: "account", Type: "address"}, {Name: "role", Type: "uint8"},
	})
	guardianArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "repoId", Type: "bytes32"}, {Name: "configuredBy", Type: "address"},
		{Name: "threshold", Type: "uint8"}, {Name: "guardians", Type: "address[]"},
	})
	moderationStatusArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "repoId", Type: "bytes32"}, {Name: "status", Type: "uint8"},
	})
	moderationReportArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "report", Type: "tuple", Components: []abi.ArgumentMarshaling{
			{Name: "id", Type: "uint256"}, {Name: "repoId", Type: "bytes32"},
			{Name: "reporter", Type: "address"}, {Name: "status", Type: "uint8"},
			{Name: "resolution", Type: "uint8"}, {Name: "reasonHash", Type: "string"},
			{Name: "createdAt", Type: "uint64"}, {Name: "updatedAt", Type: "uint64"},
			{Name: "exists", Type: "bool"},
		}},
		{Name: "trail", Type: "tuple[]", Components: []abi.ArgumentMarshaling{
			{Name: "action", Type: "uint8"}, {Name: "actor", Type: "address"},
			{Name: "status", Type: "uint8"}, {Name: "reasonHash", Type: "string"},
			{Name: "timestamp", Type: "uint64"},
		}},
	})
	moderationStatusTrailArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "repoId", Type: "bytes32"},
		{Name: "trail", Type: "tuple[]", Components: []abi.ArgumentMarshaling{
			{Name: "status", Type: "uint8"}, {Name: "actor", Type: "address"},
			{Name: "reasonHash", Type: "string"}, {Name: "timestamp", Type: "uint64"},
			{Name: "reportId", Type: "uint256"},
		}},
	})
	economicSplitsArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "repoId", Type: "bytes32"},
		{Name: "splits", Type: "tuple[]", Components: []abi.ArgumentMarshaling{
			{Name: "recipient", Type: "address"}, {Name: "bps", Type: "uint16"},
		}},
	})
	economicTotalArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "repoId", Type: "bytes32"}, {Name: "denom", Type: "string"}, {Name: "amount", Type: "uint256"},
	})
	usernameOwnerArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "name", Type: "string"}, {Name: "owner", Type: "address"},
	})
	usernameReservedArguments = abi.Arguments{{Type: mustABIType("string[]", nil)}}
	badgeArguments            = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "id", Type: "uint256"}, {Name: "repoId", Type: "bytes32"},
		{Name: "recipient", Type: "address"}, {Name: "awardedBy", Type: "address"},
		{Name: "reason", Type: "string"}, {Name: "awardedAt", Type: "uint64"}, {Name: "exists", Type: "bool"},
	})
	releaseArguments = tupleArrayArguments([]abi.ArgumentMarshaling{
		{Name: "version", Type: "string"}, {Name: "platform", Type: "string"},
		{Name: "sha256", Type: "bytes32"}, {Name: "registeredBy", Type: "address"},
		{Name: "registeredAt", Type: "uint64"}, {Name: "exists", Type: "bool"},
	})
)

func mustABIType(name string, components []abi.ArgumentMarshaling) abi.Type {
	typeValue, err := abi.NewType(name, "", components)
	if err != nil {
		panic(err)
	}
	return typeValue
}

func tupleArrayArguments(components []abi.ArgumentMarshaling) abi.Arguments {
	return abi.Arguments{{Type: mustABIType("tuple[]", components)}}
}

func encodeDirectPayload(arguments abi.Arguments, records any) ([]byte, error) {
	return arguments.Pack(records)
}

func encodeKindPayload(kind uint8, arguments abi.Arguments, records any) ([]byte, error) {
	inner, err := arguments.Pack(records)
	if err != nil {
		return nil, err
	}
	return outerPayloadArguments.Pack(kind, inner)
}

func decodePayloadCount(module string, kind uint8, payload []byte) (uint64, error) {
	arguments, wrapped, err := payloadArguments(module, kind)
	if err != nil {
		return 0, err
	}
	encoded := payload
	if wrapped {
		outer, err := outerPayloadArguments.Unpack(payload)
		if err != nil {
			return 0, fmt.Errorf("decode kind wrapper: %w", err)
		}
		if len(outer) != 2 || outer[0].(uint8) != kind {
			return 0, errors.New("payload kind wrapper does not match plan")
		}
		repacked, err := outerPayloadArguments.Pack(outer...)
		if err != nil || !bytes.Equal(repacked, payload) {
			return 0, errors.New("payload kind wrapper is not canonical ABI")
		}
		encoded = outer[1].([]byte)
	}
	values, err := arguments.Unpack(encoded)
	if err != nil {
		return 0, fmt.Errorf("decode records: %w", err)
	}
	if len(values) != 1 || reflect.ValueOf(values[0]).Kind() != reflect.Slice {
		return 0, errors.New("payload does not contain exactly one record array")
	}
	repacked, err := arguments.Pack(values...)
	if err != nil || !bytes.Equal(repacked, encoded) {
		return 0, errors.New("record payload is not canonical ABI")
	}
	return uint64(reflect.ValueOf(values[0]).Len()), nil
}

func payloadArguments(module string, kind uint8) (abi.Arguments, bool, error) {
	switch module {
	case "repository-core":
		values := []abi.Arguments{coreRepositoryArguments, coreAliasArguments, coreRefArguments, coreCollaboratorArguments}
		if int(kind) >= len(values) {
			return nil, true, fmt.Errorf("invalid repository-core kind %d", kind)
		}
		return values[kind], true, nil
	case "recovery":
		if kind != 0 {
			return nil, false, fmt.Errorf("invalid recovery kind %d", kind)
		}
		return guardianArguments, false, nil
	case "moderation":
		values := []abi.Arguments{moderationStatusArguments, moderationReportArguments, moderationStatusTrailArguments}
		if int(kind) >= len(values) {
			return nil, true, fmt.Errorf("invalid moderation kind %d", kind)
		}
		return values[kind], true, nil
	case "economic":
		values := []abi.Arguments{economicSplitsArguments, economicTotalArguments}
		if int(kind) >= len(values) {
			return nil, true, fmt.Errorf("invalid economic kind %d", kind)
		}
		return values[kind], true, nil
	case "username":
		values := []abi.Arguments{usernameOwnerArguments, usernameReservedArguments}
		if int(kind) >= len(values) {
			return nil, true, fmt.Errorf("invalid username kind %d", kind)
		}
		return values[kind], true, nil
	case "badge":
		if kind != 0 {
			return nil, false, fmt.Errorf("invalid badge kind %d", kind)
		}
		return badgeArguments, false, nil
	case "release":
		if kind != 0 {
			return nil, false, fmt.Errorf("invalid release kind %d", kind)
		}
		return releaseArguments, false, nil
	default:
		return nil, false, fmt.Errorf("unknown module %q", module)
	}
}

func rollingSeed(id, snapshotRoot common.Hash) common.Hash {
	encoded, err := rollingSeedArguments.Pack(id, snapshotRoot)
	if err != nil {
		panic(err)
	}
	return crypto.Keccak256Hash(encoded)
}

func rollingStep(previous, id common.Hash, sequence, count uint64, payloadHash common.Hash) common.Hash {
	encoded, err := rollingRootArguments.Pack(
		previous, id, new(big.Int).SetUint64(sequence), new(big.Int).SetUint64(count), payloadHash,
	)
	if err != nil {
		panic(err)
	}
	return crypto.Keccak256Hash(encoded)
}

func hexBytes(value []byte) string { return "0x" + hex.EncodeToString(value) }

func parseHexBytes(label, value string) ([]byte, error) {
	if !strings.HasPrefix(value, "0x") || value != strings.ToLower(value) || len(value)%2 != 0 {
		return nil, fmt.Errorf("%s must be canonical lowercase 0x-prefixed hex", label)
	}
	decoded, err := hex.DecodeString(value[2:])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return decoded, nil
}

func parseHash(label, value string) (common.Hash, error) {
	decoded, err := parseHexBytes(label, value)
	if err != nil {
		return common.Hash{}, err
	}
	if len(decoded) != common.HashLength {
		return common.Hash{}, fmt.Errorf("%s must be 32 bytes", label)
	}
	var result common.Hash
	copy(result[:], decoded)
	return result, nil
}

func parseRawHash(label, value string) ([32]byte, error) {
	hash, err := parseHash(label, value)
	return [32]byte(hash), err
}

func canonicalAddress(label, value string) (common.Address, error) {
	if value != strings.ToLower(value) || !common.IsHexAddress(value) || len(value) != 42 || value == "0x0000000000000000000000000000000000000000" {
		return common.Address{}, fmt.Errorf("%s must be a nonzero canonical lowercase EVM address", label)
	}
	return common.HexToAddress(value), nil
}

func rawSHA256(label, value string) ([32]byte, error) {
	if len(value) != 64 || value != strings.ToLower(value) {
		return [32]byte{}, fmt.Errorf("%s must be a lowercase 32-byte hex digest", label)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return [32]byte{}, fmt.Errorf("%s: %w", label, err)
	}
	var result [32]byte
	copy(result[:], decoded)
	if result == ([32]byte{}) {
		return [32]byte{}, fmt.Errorf("%s must not be zero", label)
	}
	return result, nil
}
