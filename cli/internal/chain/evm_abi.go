package chain

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
)

// This is the legacy Keccak-256 variant used by Solidity selectors. The Go
// standard library exposes FIPS-202 SHA3, whose domain-separation byte differs
// from Ethereum's Keccak variant, so the small permutation below is kept local
// and covered by known-vector tests. It is hashing only; no signing primitive
// is implemented here.
func keccak256(input []byte) [32]byte {
	const rate = 136
	state := [25]uint64{}
	for len(input) >= rate {
		for i := 0; i < rate/8; i++ {
			state[i] ^= binary.LittleEndian.Uint64(input[i*8:])
		}
		keccakF1600(&state)
		input = input[rate:]
	}
	var block [rate]byte
	copy(block[:], input)
	block[len(input)] = 0x01
	block[rate-1] ^= 0x80
	for i := 0; i < rate/8; i++ {
		state[i] ^= binary.LittleEndian.Uint64(block[i*8:])
	}
	keccakF1600(&state)
	var out [32]byte
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint64(out[i*8:], state[i])
	}
	return out
}

func keccakF1600(state *[25]uint64) {
	var roundConstants = [...]uint64{
		0x0000000000000001, 0x0000000000008082,
		0x800000000000808a, 0x8000000080008000,
		0x000000000000808b, 0x0000000080000001,
		0x8000000080008081, 0x8000000000008009,
		0x000000000000008a, 0x0000000000000088,
		0x0000000080008009, 0x000000008000000a,
		0x000000008000808b, 0x800000000000008b,
		0x8000000000008089, 0x8000000000008003,
		0x8000000000008002, 0x8000000000000080,
		0x000000000000800a, 0x800000008000000a,
		0x8000000080008081, 0x8000000000008080,
		0x0000000080000001, 0x8000000080008008,
	}
	var rotations = [...]uint{
		0, 1, 62, 28, 27,
		36, 44, 6, 55, 20,
		3, 10, 43, 25, 39,
		41, 45, 15, 21, 8,
		18, 2, 61, 56, 14,
	}
	var c [5]uint64
	var d [5]uint64
	var b [25]uint64
	for _, rc := range roundConstants {
		for x := 0; x < 5; x++ {
			c[x] = state[x] ^ state[x+5] ^ state[x+10] ^ state[x+15] ^ state[x+20]
		}
		for x := 0; x < 5; x++ {
			d[x] = c[(x+4)%5] ^ bitsRotateLeft64(c[(x+1)%5], 1)
		}
		for y := 0; y < 5; y++ {
			for x := 0; x < 5; x++ {
				state[x+5*y] ^= d[x]
			}
		}
		for y := 0; y < 5; y++ {
			for x := 0; x < 5; x++ {
				b[y+5*((2*x+3*y)%5)] = bitsRotateLeft64(state[x+5*y], rotations[x+5*y])
			}
		}
		for y := 0; y < 5; y++ {
			for x := 0; x < 5; x++ {
				state[x+5*y] = b[x+5*y] ^ (^b[(x+1)%5+5*y] & b[(x+2)%5+5*y])
			}
		}
		state[0] ^= rc
	}
}

func bitsRotateLeft64(value uint64, shift uint) uint64 {
	if shift == 0 {
		return value
	}
	return (value << shift) | (value >> (64 - shift))
}

type evmABIKind uint8

const (
	abiAddress evmABIKind = iota
	abiBool
	abiBytes32
	abiString
	abiStringArray
	abiUint
	abiAddressArray
	abiUintArray
)

type evmABIValue struct {
	kind      evmABIKind
	address   string
	boolean   bool
	bytes32   [32]byte
	number    uint64
	text      string
	texts     []string
	addresses []string
	numbers   []uint64
}

func abiAddressValue(value string) evmABIValue { return evmABIValue{kind: abiAddress, address: value} }
func abiBoolValue(value bool) evmABIValue      { return evmABIValue{kind: abiBool, boolean: value} }
func abiBytes32Value(value [32]byte) evmABIValue {
	return evmABIValue{kind: abiBytes32, bytes32: value}
}
func abiUintValue(value uint64) evmABIValue   { return evmABIValue{kind: abiUint, number: value} }
func abiStringValue(value string) evmABIValue { return evmABIValue{kind: abiString, text: value} }
func abiStringArrayValue(value []string) evmABIValue {
	return evmABIValue{kind: abiStringArray, texts: value}
}
func abiAddressArrayValue(value []string) evmABIValue {
	return evmABIValue{kind: abiAddressArray, addresses: value}
}
func abiUintArrayValue(value []uint64) evmABIValue {
	return evmABIValue{kind: abiUintArray, numbers: value}
}

func encodeABICall(signature string, values ...evmABIValue) ([]byte, error) {
	if strings.TrimSpace(signature) == "" {
		return nil, fmt.Errorf("ABI signature is empty")
	}
	digest := keccak256([]byte(signature))
	selector := digest[:4]
	head := make([]byte, 32*len(values))
	var tail []byte
	for i, value := range values {
		word := head[i*32 : (i+1)*32]
		switch value.kind {
		case abiAddress:
			address, err := parseEVMAddress(value.address)
			if err != nil {
				return nil, err
			}
			copy(word[12:], address)
		case abiBool:
			if value.boolean {
				word[31] = 1
			}
		case abiBytes32:
			copy(word, value.bytes32[:])
		case abiUint:
			putABIWord(word, value.number)
		case abiString, abiStringArray, abiAddressArray, abiUintArray:
			putABIWord(word, uint64(len(head)+len(tail)))
			encoded, err := encodeABIDynamic(value)
			if err != nil {
				return nil, err
			}
			tail = append(tail, encoded...)
		default:
			return nil, fmt.Errorf("unsupported ABI argument kind %d", value.kind)
		}
	}
	result := make([]byte, 0, 4+len(head)+len(tail))
	result = append(result, selector...)
	result = append(result, head...)
	result = append(result, tail...)
	return result, nil
}

func encodeABIDynamic(value evmABIValue) ([]byte, error) {
	switch value.kind {
	case abiString:
		return encodeABIString(value.text), nil
	case abiStringArray:
		return encodeABIStringArray(value.texts), nil
	case abiAddressArray:
		return encodeABIAddressArray(value.addresses)
	case abiUintArray:
		return encodeABIUintArray(value.numbers), nil
	default:
		return nil, fmt.Errorf("argument kind %d is not dynamic", value.kind)
	}
}

func encodeABIAddressArray(values []string) ([]byte, error) {
	result := make([]byte, 32+len(values)*32)
	putABIWord(result[:32], uint64(len(values)))
	for i, value := range values {
		address, err := parseEVMAddress(value)
		if err != nil {
			return nil, fmt.Errorf("ABI address array item %d: %w", i, err)
		}
		copy(result[32+i*32+12:32+(i+1)*32], address)
	}
	return result, nil
}

func encodeABIUintArray(values []uint64) []byte {
	result := make([]byte, 32+len(values)*32)
	putABIWord(result[:32], uint64(len(values)))
	for i, value := range values {
		putABIWord(result[32+i*32:32+(i+1)*32], value)
	}
	return result
}

func encodeABIString(value string) []byte {
	raw := []byte(value)
	result := make([]byte, 32+((len(raw)+31)/32)*32)
	putABIWord(result[:32], uint64(len(raw)))
	copy(result[32:], raw)
	return result
}

func encodeABIStringArray(values []string) []byte {
	// A dynamic array of dynamic strings contains a length word, one offset
	// word per element, then each string tail. Offsets are relative to the
	// first offset word (the array head after its length word).
	headLen := 32 + len(values)*32
	head := make([]byte, headLen)
	putABIWord(head[:32], uint64(len(values)))
	var tail []byte
	for i, value := range values {
		putABIWord(head[32+i*32:32+(i+1)*32], uint64(headLen+len(tail)-32))
		tail = append(tail, encodeABIString(value)...)
	}
	return append(head, tail...)
}

func putABIWord(word []byte, value uint64) {
	for i := 0; i < 8; i++ {
		word[31-i] = byte(value >> (8 * i))
	}
}

func readABIWord(data []byte, offset int) ([]byte, error) {
	if offset < 0 || offset > len(data)-32 {
		return nil, fmt.Errorf("ABI word offset %d is out of bounds for %d bytes", offset, len(data))
	}
	return data[offset : offset+32], nil
}

func readABIUint(data []byte, offset int) (uint64, error) {
	word, err := readABIWord(data, offset)
	if err != nil {
		return 0, err
	}
	var out uint64
	for _, value := range word {
		if out > math.MaxUint64>>8 {
			return 0, fmt.Errorf("ABI uint at offset %d overflows uint64", offset)
		}
		out = (out << 8) | uint64(value)
	}
	return out, nil
}

func readABIOffset(data []byte, offset, base int) (int, error) {
	value, err := readABIUint(data, offset)
	if err != nil {
		return 0, err
	}
	if value > uint64(len(data)) {
		return 0, fmt.Errorf("ABI dynamic offset %d exceeds result length %d", value, len(data))
	}
	absolute := base + int(value)
	if absolute < 0 || absolute > len(data) {
		return 0, fmt.Errorf("ABI dynamic offset %d with base %d is out of bounds", value, base)
	}
	return absolute, nil
}

func readABIString(data []byte, offset int) (string, error) {
	length, err := readABIUint(data, offset)
	if err != nil {
		return "", err
	}
	if length > uint64(len(data)-offset-32) {
		return "", fmt.Errorf("ABI string length %d exceeds result length", length)
	}
	return string(data[offset+32 : offset+32+int(length)]), nil
}

func readABIStringArray(data []byte, offset int) ([]string, error) {
	count, err := readABIUint(data, offset)
	if err != nil {
		return nil, err
	}
	if count > uint64((len(data)-offset-32)/32) {
		return nil, fmt.Errorf("ABI string array length %d exceeds result length", count)
	}
	values := make([]string, 0, count)
	for i := uint64(0); i < count; i++ {
		itemOffset, err := readABIOffset(data, offset+32+int(i)*32, offset+32)
		if err != nil {
			return nil, err
		}
		value, err := readABIString(data, itemOffset)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func readABIAddressArray(data []byte, offset int) ([]string, error) {
	count, err := readABIUint(data, offset)
	if err != nil {
		return nil, err
	}
	if count > uint64((len(data)-offset-32)/32) {
		return nil, fmt.Errorf("ABI address array length %d exceeds result length", count)
	}
	values := make([]string, 0, count)
	for i := uint64(0); i < count; i++ {
		word, err := readABIWord(data, offset+32+int(i)*32)
		if err != nil {
			return nil, err
		}
		for _, high := range word[:12] {
			if high != 0 {
				return nil, fmt.Errorf("ABI address at index %d has non-zero padding", i)
			}
		}
		address, err := userAddressFromEVM("0x" + hex.EncodeToString(word[12:]))
		if err != nil {
			return nil, err
		}
		values = append(values, address)
	}
	return values, nil
}

func readABIUintArray(data []byte, offset int) ([]uint64, error) {
	count, err := readABIUint(data, offset)
	if err != nil {
		return nil, err
	}
	if count > uint64((len(data)-offset-32)/32) {
		return nil, fmt.Errorf("ABI uint array length %d exceeds result length", count)
	}
	values := make([]uint64, 0, count)
	for i := uint64(0); i < count; i++ {
		value, err := readABIUint(data, offset+32+int(i)*32)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func readABIBytes32Array(data []byte, offset int) ([][32]byte, error) {
	count, err := readABIUint(data, offset)
	if err != nil {
		return nil, err
	}
	if count > uint64((len(data)-offset-32)/32) {
		return nil, fmt.Errorf("ABI bytes32 array length %d exceeds result length", count)
	}
	values := make([][32]byte, 0, count)
	for i := uint64(0); i < count; i++ {
		word, err := readABIWord(data, offset+32+int(i)*32)
		if err != nil {
			return nil, err
		}
		var value [32]byte
		copy(value[:], word)
		values = append(values, value)
	}
	return values, nil
}

func parseEVMAddress(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "0x") || strings.HasPrefix(trimmed, "0X") {
		trimmed = trimmed[2:]
		if len(trimmed) != 40 {
			return nil, fmt.Errorf("EVM address must contain 20 bytes: %q", value)
		}
		decoded, err := hex.DecodeString(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid EVM address %q: %w", value, err)
		}
		return decoded, nil
	}
	return decodeBech32Address(trimmed)
}

func normalizeEVMAddress(value string) (string, error) {
	decoded, err := parseEVMAddress(value)
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(decoded), nil
}

// NormalizeEVMAddress converts either an inj1 bech32 address or an EVM hex
// address into the canonical lower-case 0x form. Migration tooling uses the
// same conversion as the runtime backend so snapshot planning cannot drift
// from normal CLI address semantics.
func NormalizeEVMAddress(value string) (string, error) {
	return normalizeEVMAddress(value)
}

func userAddressFromEVM(value string) (string, error) {
	decoded, err := parseEVMAddress(value)
	if err != nil {
		return "", err
	}
	return encodeBech32Address("inj", decoded), nil
}

func decodeABITupleBase(data []byte) (int, error) {
	// A single dynamic tuple is returned as an outer offset. Supporting a
	// direct tuple as well makes the decoder tolerant of hand-built test data.
	if len(data) >= 32 {
		if offset, err := readABIOffset(data, 0, 0); err == nil && offset >= 32 && offset < len(data) {
			return offset, nil
		}
	}
	return 0, nil
}

type evmDecodedRef struct {
	CommitSha string
	PackURIs  []string
	UpdatedAt uint64
	UpdatedBy string
}

func decodeEVMRepoInfo(data []byte) (*RepoInfo, error) {
	base, err := decodeABITupleBase(data)
	if err != nil {
		return nil, err
	}
	return decodeEVMRepoAt(data, base)
}

func decodeEVMRepoAt(data []byte, base int) (*RepoInfo, error) {
	ownerWord, err := readABIWord(data, base)
	if err != nil {
		return nil, err
	}
	owner, err := userAddressFromEVM("0x" + hex.EncodeToString(ownerWord[12:]))
	if err != nil {
		return nil, err
	}
	nameOffset, err := readABIOffset(data, base+32, base)
	if err != nil {
		return nil, err
	}
	descriptionOffset, err := readABIOffset(data, base+64, base)
	if err != nil {
		return nil, err
	}
	branchOffset, err := readABIOffset(data, base+96, base)
	if err != nil {
		return nil, err
	}
	name, err := readABIString(data, nameOffset)
	if err != nil {
		return nil, err
	}
	description, err := readABIString(data, descriptionOffset)
	if err != nil {
		return nil, err
	}
	defaultBranch, err := readABIString(data, branchOffset)
	if err != nil {
		return nil, err
	}
	createdAt, err := readABIUint(data, base+128)
	if err != nil {
		return nil, err
	}
	updatedAt, err := readABIUint(data, base+160)
	if err != nil {
		return nil, err
	}
	moderationStatus, err := readABIUint(data, base+192)
	if err != nil {
		return nil, err
	}
	existsWord, err := readABIWord(data, base+224)
	if err != nil {
		return nil, err
	}
	if existsWord[31] == 0 {
		return nil, fmt.Errorf("EVM contract returned a non-existent repository")
	}
	status := ""
	switch moderationStatus {
	case 0:
		status = "active"
	case 1:
		status = "frozen"
	case 2:
		status = "delisted"
	default:
		return nil, fmt.Errorf("EVM contract returned invalid moderation status %d", moderationStatus)
	}
	return &RepoInfo{
		Owner:            owner,
		Name:             name,
		Description:      description,
		DefaultBranch:    defaultBranch,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		ModerationStatus: status,
	}, nil
}

func decodeEVMResolvedRepo(data []byte, requestedOwner, requestedName string) (*ResolvedRepo, error) {
	repoID, err := readABIBytes32(data, 0)
	if err != nil {
		return nil, err
	}
	canonicalWord, err := readABIWord(data, 32)
	if err != nil {
		return nil, err
	}
	canonicalHex := hex.EncodeToString(canonicalWord)
	if strings.Trim(canonicalHex[:62], "0") != "" {
		return nil, fmt.Errorf("EVM resolveRepo returned malformed canonical flag")
	}
	canonicalValue := canonicalHex[63]
	if canonicalValue != '0' && canonicalValue != '1' {
		return nil, fmt.Errorf("EVM resolveRepo returned invalid canonical flag %q", canonicalValue)
	}
	repoOffset, err := readABIOffset(data, 64, 0)
	if err != nil {
		return nil, err
	}
	info, err := decodeEVMRepoAt(data, repoOffset)
	if err != nil {
		return nil, err
	}
	return &ResolvedRepo{
		RepoID:      repoID,
		Backend:     BackendEVM,
		Requested:   RepoLocator{Owner: requestedOwner, Name: requestedName},
		Canonical:   RepoLocator{Owner: info.Owner, Name: info.Name},
		IsCanonical: canonicalValue == '1',
		Info:        *info,
	}, nil
}

// decodeEVMRepoPage decodes
// (uint256 nextCursor,bool hasMore,bytes32[] repoIds,Repo[] repositories).
func decodeEVMRepoPage(data []byte) (uint64, bool, []RepoInfo, error) {
	nextCursor, err := readABIUint(data, 0)
	if err != nil {
		return 0, false, nil, err
	}
	hasMoreWord, err := readABIWord(data, 32)
	if err != nil {
		return 0, false, nil, err
	}
	for _, value := range hasMoreWord[:31] {
		if value != 0 {
			return 0, false, nil, fmt.Errorf("EVM repository page returned malformed hasMore flag")
		}
	}
	if hasMoreWord[31] > 1 {
		return 0, false, nil, fmt.Errorf("EVM repository page returned invalid hasMore flag %d", hasMoreWord[31])
	}
	idsOffset, err := readABIOffset(data, 64, 0)
	if err != nil {
		return 0, false, nil, err
	}
	repositoriesOffset, err := readABIOffset(data, 96, 0)
	if err != nil {
		return 0, false, nil, err
	}
	ids, err := readABIBytes32Array(data, idsOffset)
	if err != nil {
		return 0, false, nil, err
	}
	count, err := readABIUint(data, repositoriesOffset)
	if err != nil {
		return 0, false, nil, err
	}
	if count != uint64(len(ids)) {
		return 0, false, nil, fmt.Errorf("EVM repository page returned %d IDs and %d repositories", len(ids), count)
	}
	repositories := make([]RepoInfo, 0, count)
	for i := uint64(0); i < count; i++ {
		itemOffset, err := readABIOffset(
			data,
			repositoriesOffset+32+int(i)*32,
			repositoriesOffset+32,
		)
		if err != nil {
			return 0, false, nil, err
		}
		info, err := decodeEVMRepoAt(data, itemOffset)
		if err != nil {
			return 0, false, nil, err
		}
		repositories = append(repositories, *info)
	}
	return nextCursor, hasMoreWord[31] == 1, repositories, nil
}

func decodeEVMPendingOwnershipTransfer(data []byte) (*OwnershipTransferInfo, error) {
	// PendingOwnershipTransfer contains only static ABI fields, so Solidity
	// returns its four words directly without an outer tuple offset.
	const base = 0
	ownerWord, err := readABIWord(data, base)
	if err != nil {
		return nil, fmt.Errorf("decode pending ownership target: %w", err)
	}
	proposedAt, err := readABIUint(data, base+32)
	if err != nil {
		return nil, fmt.Errorf("decode pending ownership proposed time: %w", err)
	}
	executeAfter, err := readABIUint(data, base+64)
	if err != nil {
		return nil, fmt.Errorf("decode pending ownership execution time: %w", err)
	}
	expiresAt, err := readABIUint(data, base+96)
	if err != nil {
		return nil, fmt.Errorf("decode pending ownership expiry time: %w", err)
	}

	zeroOwner := true
	for _, value := range ownerWord {
		zeroOwner = zeroOwner && value == 0
	}
	if zeroOwner {
		if proposedAt != 0 || executeAfter != 0 || expiresAt != 0 {
			return nil, fmt.Errorf("EVM pending ownership response has timestamps without a target")
		}
		return nil, nil
	}
	newOwner, err := readABIAddress(data, base)
	if err != nil {
		return nil, fmt.Errorf("decode pending ownership target: %w", err)
	}
	return &OwnershipTransferInfo{
		NewOwner:     newOwner,
		ProposedAt:   proposedAt,
		ExecuteAfter: executeAfter,
		ExpiresAt:    expiresAt,
	}, nil
}

func decodeEVMRef(data []byte, base int) (evmDecodedRef, error) {
	commitOffset, err := readABIOffset(data, base, base)
	if err != nil {
		return evmDecodedRef{}, err
	}
	packOffset, err := readABIOffset(data, base+32, base)
	if err != nil {
		return evmDecodedRef{}, err
	}
	commitSha, err := readABIString(data, commitOffset)
	if err != nil {
		return evmDecodedRef{}, err
	}
	packURIs, err := readABIStringArray(data, packOffset)
	if err != nil {
		return evmDecodedRef{}, err
	}
	updatedAt, err := readABIUint(data, base+64)
	if err != nil {
		return evmDecodedRef{}, err
	}
	updatedByWord, err := readABIWord(data, base+96)
	if err != nil {
		return evmDecodedRef{}, err
	}
	updatedBy, err := userAddressFromEVM("0x" + hex.EncodeToString(updatedByWord[12:]))
	if err != nil {
		return evmDecodedRef{}, err
	}
	existsWord, err := readABIWord(data, base+128)
	if err != nil {
		return evmDecodedRef{}, err
	}
	if existsWord[31] == 0 {
		return evmDecodedRef{}, fmt.Errorf("EVM contract returned a non-existent ref")
	}
	return evmDecodedRef{CommitSha: commitSha, PackURIs: packURIs, UpdatedAt: updatedAt, UpdatedBy: updatedBy}, nil
}

func decodeEVMResolveRef(data []byte) (evmDecodedRef, error) {
	base, err := decodeABITupleBase(data)
	if err != nil {
		return evmDecodedRef{}, err
	}
	return decodeEVMRef(data, base)
}

func decodeEVMListRefs(data []byte) ([]RefInfo, error) {
	namesOffset, err := readABIOffset(data, 0, 0)
	if err != nil {
		return nil, err
	}
	valuesOffset, err := readABIOffset(data, 32, 0)
	if err != nil {
		return nil, err
	}
	names, err := readABIStringArray(data, namesOffset)
	if err != nil {
		return nil, err
	}
	count, err := readABIUint(data, valuesOffset)
	if err != nil {
		return nil, err
	}
	if count != uint64(len(names)) {
		return nil, fmt.Errorf("EVM listRefs returned %d names and %d values", len(names), count)
	}
	refs := make([]RefInfo, 0, len(names))
	for i := range names {
		itemOffset, err := readABIOffset(data, valuesOffset+32+i*32, valuesOffset+32)
		if err != nil {
			return nil, err
		}
		ref, err := decodeEVMRef(data, itemOffset)
		if err != nil {
			return nil, err
		}
		refs = append(refs, RefInfo{
			RefName:   names[i],
			CommitSha: ref.CommitSha,
			PackURIs:  ref.PackURIs,
			UpdatedAt: ref.UpdatedAt,
			UpdatedBy: ref.UpdatedBy,
		})
	}
	return refs, nil
}

// decodeEVMRefPage decodes
// (uint256 nextCursor,bool hasMore,string[] names,Ref[] values).
func decodeEVMRefPage(data []byte) (uint64, bool, []RefInfo, error) {
	nextCursor, err := readABIUint(data, 0)
	if err != nil {
		return 0, false, nil, err
	}
	hasMoreWord, err := readABIWord(data, 32)
	if err != nil {
		return 0, false, nil, err
	}
	for _, value := range hasMoreWord[:31] {
		if value != 0 {
			return 0, false, nil, fmt.Errorf("EVM ref page returned malformed hasMore flag")
		}
	}
	if hasMoreWord[31] > 1 {
		return 0, false, nil, fmt.Errorf("EVM ref page returned invalid hasMore flag %d", hasMoreWord[31])
	}
	namesOffset, err := readABIOffset(data, 64, 0)
	if err != nil {
		return 0, false, nil, err
	}
	valuesOffset, err := readABIOffset(data, 96, 0)
	if err != nil {
		return 0, false, nil, err
	}
	names, err := readABIStringArray(data, namesOffset)
	if err != nil {
		return 0, false, nil, err
	}
	count, err := readABIUint(data, valuesOffset)
	if err != nil {
		return 0, false, nil, err
	}
	if count != uint64(len(names)) {
		return 0, false, nil, fmt.Errorf("EVM ref page returned %d names and %d values", len(names), count)
	}
	refs := make([]RefInfo, 0, len(names))
	for i := range names {
		itemOffset, err := readABIOffset(data, valuesOffset+32+i*32, valuesOffset+32)
		if err != nil {
			return 0, false, nil, err
		}
		ref, err := decodeEVMRef(data, itemOffset)
		if err != nil {
			return 0, false, nil, err
		}
		refs = append(refs, RefInfo{
			RefName:   names[i],
			CommitSha: ref.CommitSha,
			PackURIs:  ref.PackURIs,
			UpdatedAt: ref.UpdatedAt,
			UpdatedBy: ref.UpdatedBy,
		})
	}
	return nextCursor, hasMoreWord[31] == 1, refs, nil
}

// decodeEVMCollaboratorPage decodes
// (uint256 nextCursor,bool hasMore,address[] collaborators,Role[] roles).
func decodeEVMCollaboratorPage(data []byte) (uint64, bool, []CollaboratorInfo, error) {
	nextCursor, err := readABIUint(data, 0)
	if err != nil {
		return 0, false, nil, err
	}
	hasMoreWord, err := readABIWord(data, 32)
	if err != nil {
		return 0, false, nil, err
	}
	for _, value := range hasMoreWord[:31] {
		if value != 0 {
			return 0, false, nil, fmt.Errorf("EVM collaborator page returned malformed hasMore flag")
		}
	}
	if hasMoreWord[31] > 1 {
		return 0, false, nil, fmt.Errorf("EVM collaborator page returned invalid hasMore flag %d", hasMoreWord[31])
	}
	addressesOffset, err := readABIOffset(data, 64, 0)
	if err != nil {
		return 0, false, nil, err
	}
	rolesOffset, err := readABIOffset(data, 96, 0)
	if err != nil {
		return 0, false, nil, err
	}
	addresses, err := readABIAddressArray(data, addressesOffset)
	if err != nil {
		return 0, false, nil, err
	}
	roles, err := readABIUintArray(data, rolesOffset)
	if err != nil {
		return 0, false, nil, err
	}
	if len(addresses) != len(roles) {
		return 0, false, nil, fmt.Errorf("EVM collaborator page returned %d addresses and %d roles", len(addresses), len(roles))
	}
	values := make([]CollaboratorInfo, 0, len(addresses))
	for i := range addresses {
		role, err := collaboratorRoleName(roles[i])
		if err != nil {
			return 0, false, nil, fmt.Errorf("EVM collaborator page entry %d: %w", i, err)
		}
		values = append(values, CollaboratorInfo{Address: addresses[i], Role: role})
	}
	return nextCursor, hasMoreWord[31] == 1, values, nil
}

type RepoNotFoundError struct {
	RepoID [32]byte
}

func (e *RepoNotFoundError) Error() string {
	return fmt.Sprintf("repository not found (repo ID 0x%s)", hex.EncodeToString(e.RepoID[:]))
}

type LocatorNotFoundError struct {
	Owner string
	Name  string
}

func (e *LocatorNotFoundError) Error() string {
	return fmt.Sprintf("repository locator not found: igit://%s/%s", e.Owner, e.Name)
}

type RepoMovedError struct {
	RepoID       [32]byte
	CurrentOwner string
	Name         string
}

func (e *RepoMovedError) CanonicalURL() string {
	return fmt.Sprintf("igit://%s/%s", e.CurrentOwner, e.Name)
}

func (e *RepoMovedError) Error() string {
	return fmt.Sprintf("repository moved to %s", e.CanonicalURL())
}

func readABIBytes32(data []byte, offset int) ([32]byte, error) {
	word, err := readABIWord(data, offset)
	if err != nil {
		return [32]byte{}, err
	}
	var value [32]byte
	copy(value[:], word)
	return value, nil
}

func readABIAddress(data []byte, offset int) (string, error) {
	word, err := readABIWord(data, offset)
	if err != nil {
		return "", err
	}
	for _, high := range word[:12] {
		if high != 0 {
			return "", fmt.Errorf("ABI address at offset %d has non-zero padding", offset)
		}
	}
	return userAddressFromEVM("0x" + hex.EncodeToString(word[12:]))
}

func decodeRepoNotFound(payload []byte) error {
	repoID, err := readABIBytes32(payload, 0)
	if err != nil {
		return fmt.Errorf("malformed RepoNotFound error: %w", err)
	}
	return &RepoNotFoundError{RepoID: repoID}
}

func decodeLocatorNotFound(payload []byte) error {
	owner, err := readABIAddress(payload, 0)
	if err != nil {
		return fmt.Errorf("malformed LocatorNotFound error: %w", err)
	}
	nameOffset, err := readABIOffset(payload, 32, 0)
	if err != nil {
		return fmt.Errorf("malformed LocatorNotFound error: %w", err)
	}
	name, err := readABIString(payload, nameOffset)
	if err != nil {
		return fmt.Errorf("malformed LocatorNotFound error: %w", err)
	}
	return &LocatorNotFoundError{Owner: owner, Name: name}
}

func decodeRepoMoved(payload []byte) error {
	repoID, err := readABIBytes32(payload, 0)
	if err != nil {
		return fmt.Errorf("malformed RepoMoved error: %w", err)
	}
	owner, err := readABIAddress(payload, 32)
	if err != nil {
		return fmt.Errorf("malformed RepoMoved error: %w", err)
	}
	nameOffset, err := readABIOffset(payload, 64, 0)
	if err != nil {
		return fmt.Errorf("malformed RepoMoved error: %w", err)
	}
	name, err := readABIString(payload, nameOffset)
	if err != nil {
		return fmt.Errorf("malformed RepoMoved error: %w", err)
	}
	return &RepoMovedError{RepoID: repoID, CurrentOwner: owner, Name: name}
}

func decodeContractRevert(data string) error {
	raw, err := decodeHexBytes(data)
	if err != nil {
		return err
	}
	if len(raw) < 4 {
		return fmt.Errorf("EVM contract reverted with %s", normalizeHex(data))
	}
	selector := hex.EncodeToString(raw[:4])
	payload := raw[4:]
	for signature, decode := range map[string]func([]byte) error{
		"RepoNotFound(bytes32)":             decodeRepoNotFound,
		"LocatorNotFound(address,string)":   decodeLocatorNotFound,
		"RepoMoved(bytes32,address,string)": decodeRepoMoved,
	} {
		digest := keccak256([]byte(signature))
		if selector == hex.EncodeToString(digest[:4]) {
			return decode(payload)
		}
	}
	if selector == "08c379a0" {
		if len(payload) < 32 {
			return fmt.Errorf("EVM contract reverted with malformed Error(string)")
		}
		offset, err := readABIUint(payload, 0)
		if err != nil {
			return fmt.Errorf("EVM contract Error(string): %w", err)
		}
		message, err := readABIString(payload, int(offset))
		if err != nil {
			return fmt.Errorf("EVM contract Error(string): %w", err)
		}
		return fmt.Errorf("EVM contract reverted: %s", message)
	}
	if selector == "4e487b71" && len(payload) >= 32 {
		code, _ := readABIUint(payload, 0)
		return fmt.Errorf("EVM contract panicked (0x%x)", code)
	}
	known := make(map[string]string)
	for signature, message := range map[string]string{
		"EmptyInput()":                                "empty input",
		"EmptyPackUris()":                             "empty pack URI list",
		"FrozenRepo(bytes32)":                         "repository is frozen",
		"RepoAlreadyExists(bytes32)":                  "repository already exists",
		"LocatorUnavailable(address,string)":          "repository locator is unavailable",
		"RefNotFound(bytes32,bytes32)":                "ref not found",
		"Unauthorized(address)":                       "unauthorized",
		"InvalidRepoName(string)":                     "invalid repository name",
		"InvalidRefName(string)":                      "invalid ref name",
		"InvalidCommitSha(string)":                    "invalid commit SHA",
		"InvalidPackUri(string)":                      "invalid pack URI",
		"InvalidCollaborator(address)":                "invalid collaborator",
		"InvalidRole(uint8)":                          "invalid collaborator role",
		"ShaMismatch(string,string)":                  "ref SHA conflict",
		"InputTooLong(uint256,uint256)":               "input too long",
		"TooManyPackUris(uint256,uint256)":            "too many pack URIs",
		"TooManyRefs(uint256,uint256)":                "too many refs",
		"TooManyCollaborators(uint256,uint256)":       "too many collaborators",
		"InvalidTransferTarget(address)":              "invalid ownership transfer target",
		"TransferAlreadyPending(bytes32)":             "ownership transfer is already pending",
		"TransferNotPending(bytes32)":                 "ownership transfer is not pending",
		"TransferTooEarly(bytes32,uint64)":            "ownership transfer is not ready",
		"TransferExpired(bytes32,uint64)":             "ownership transfer has expired",
		"TransferNotExpired(bytes32,uint64)":          "ownership transfer has not expired",
		"TransferUnauthorized(bytes32,address)":       "ownership transfer is unauthorized",
		"LocatorReservationMismatch(bytes32,bytes32)": "ownership locator reservation mismatch",
		"TimestampOverflow(uint256)":                  "timestamp exceeds contract range",
		"InvalidRegistry()":                           "module registry is invalid",
		"RepoNotActive(bytes32,uint8)":                "repository is not active",
		"InvalidRecipient(address)":                   "invalid badge recipient",
		"InvalidReasonLength(uint256,uint256)":        "invalid badge reason length",
		"BadgeNotFound(uint256)":                      "badge not found",
		"InvalidPageSize(uint256,uint256)":            "invalid query page size",
		"InvalidCursor(uint256,uint256)":              "invalid query cursor",
		"NoFunds()":                                   "sponsorship requires funds",
		"InvalidMessageLength(uint256,uint256)":       "invalid sponsor message length",
		"SplitLengthMismatch(uint256,uint256)":        "revenue split arrays have different lengths",
		"TooManySplitRecipients(uint256,uint256)":     "too many revenue split recipients",
		"InvalidSplitRecipient(address)":              "invalid revenue split recipient",
		"DuplicateSplitRecipient(address)":            "duplicate revenue split recipient",
		"InvalidSplitBps(address,uint256)":            "invalid revenue split basis points",
		"SplitTotalTooHigh(uint256,uint256)":          "revenue split total exceeds the limit",
		"PlatformFeeTooHigh(uint256,uint256)":         "platform fee exceeds the limit",
		"PayoutFailed(address,uint256)":               "economic payout failed",
		"ReentrantSettlement()":                       "economic settlement re-entry was rejected",
		"InvalidAdmin()":                              "module administrator is invalid",
		"InvalidTreasury()":                           "module treasury is invalid",
	} {
		digest := keccak256([]byte(signature))
		known[hex.EncodeToString(digest[:4])] = message
	}
	if message, ok := known[selector]; ok {
		return fmt.Errorf("EVM contract reverted: %s (selector 0x%s)", message, selector)
	}
	return fmt.Errorf("EVM contract reverted with custom error 0x%s", selector)
}
