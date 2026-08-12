package chain

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestKeccak256KnownVectors(t *testing.T) {
	empty := keccak256(nil)
	if got := hex.EncodeToString(empty[:]); got != "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470" {
		t.Fatalf("keccak256(empty) = %s", got)
	}
	transfer := keccak256([]byte("transfer(address,uint256)"))
	if got := hex.EncodeToString(transfer[:4]); got != "a9059cbb" {
		t.Fatalf("transfer selector = %s, want a9059cbb", got)
	}
}

func TestStableIdentitySelectorVectors(t *testing.T) {
	for signature, want := range map[string]string{
		"LocatorNotFound(address,string)":                    "6de3a11d",
		"RepoMoved(bytes32,address,string)":                  "58629d7f",
		"resolveRepo(address,string)":                        "2c1ce86c",
		"listReposPage(address,uint256,uint256)":             "502a5e93",
		"listRefsPageById(bytes32,uint256,uint256)":          "084871ef",
		"listCollaboratorsPageById(bytes32,uint256,uint256)": "1758c17d",
	} {
		digest := keccak256([]byte(signature))
		if got := hex.EncodeToString(digest[:4]); got != want {
			t.Fatalf("%s selector = %s, want %s", signature, got, want)
		}
	}
}

func TestBech32EVMAddressRoundTrip(t *testing.T) {
	const address = "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh"
	raw, err := parseEVMAddress(address)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 20 {
		t.Fatalf("decoded address length = %d, want 20", len(raw))
	}
	encoded, err := userAddressFromEVM("0x" + hex.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if encoded != address {
		t.Fatalf("round-trip address = %s, want %s", encoded, address)
	}
}

func TestEncodeABICallUsesSoliditySelectorAndOffsets(t *testing.T) {
	data, err := encodeABICall("createRepo(string,string,string)", abiStringValue("demo"), abiStringValue("description"), abiStringValue("main"))
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(data[:4]); got != "90ec291d" {
		t.Fatalf("createRepo selector = %s, want 90ec291d", got)
	}
	if len(data) < 4+96 || len(data)%32 != 4 {
		t.Fatalf("calldata length = %d, want selector plus ABI words", len(data))
	}
	args := data[4:]
	if got := readTestWordUint(args[0:32]); got != 96 {
		t.Fatalf("first string offset = %d, want 96", got)
	}
	if got := readTestWordUint(args[32:64]); got <= 96 {
		t.Fatalf("second string offset = %d, want after first tail", got)
	}
	if got := readTestWordUint(args[64:96]); got <= readTestWordUint(args[32:64]) {
		t.Fatalf("third string offset = %d, want after second tail", got)
	}
}

func TestABIStringArrayRoundTrip(t *testing.T) {
	want := []string{"ipfs://one", "ipfs://two"}
	encoded := encodeABIStringArray(want)
	got, err := readABIStringArray(encoded, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("string array = %#v, want %#v", got, want)
	}
}

func TestCollaboratorRoleMapping(t *testing.T) {
	cases := []struct {
		input string
		want  uint64
	}{
		{input: "", want: 0},
		{input: "none", want: 0},
		{input: " NONE ", want: 0},
		{input: "maintainer", want: 1},
		{input: "MAINTAINER", want: 1},
		{input: "reader", want: 2},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := collaboratorRoleValue(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("role %q = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
	if _, err := collaboratorRoleValue("admin"); err == nil {
		t.Fatal("invalid role unexpectedly accepted")
	}
	for _, tc := range []struct {
		value uint64
		want  string
	}{{1, "maintainer"}, {2, "reader"}} {
		got, err := collaboratorRoleName(tc.value)
		if err != nil || got != tc.want {
			t.Fatalf("role value %d = %q, err=%v; want %q", tc.value, got, err, tc.want)
		}
	}
	if _, err := collaboratorRoleName(0); err == nil {
		t.Fatal("Role.None unexpectedly exposed as a collaborator")
	}
}

func TestCollaboratorAddressArrayRoundTrip(t *testing.T) {
	wantEVM := []string{
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
	}
	encoded := make([]byte, 32+len(wantEVM)*32)
	putABIWord(encoded[:32], uint64(len(wantEVM)))
	for i, value := range wantEVM {
		raw, err := parseEVMAddress(value)
		if err != nil {
			t.Fatal(err)
		}
		copy(encoded[32+i*32+12:32+(i+1)*32], raw)
	}
	got, err := readABIAddressArray(encoded, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, value := range wantEVM {
		want, err := userAddressFromEVM(value)
		if err != nil {
			t.Fatal(err)
		}
		if got[i] != want {
			t.Fatalf("address[%d] = %q, want %q", i, got[i], want)
		}
	}

	malformed := append([]byte(nil), encoded...)
	malformed[32] = 1 // non-zero ABI address padding
	if _, err := readABIAddressArray(malformed, 0); err == nil {
		t.Fatal("non-zero address padding unexpectedly accepted")
	}
}

func TestEncodeSetCollaboratorCalldataLayout(t *testing.T) {
	data, err := encodeABICall(
		"setCollaborator(address,string,address,uint8)",
		abiAddressValue("0x1111111111111111111111111111111111111111"),
		abiStringValue("demo"),
		abiAddressValue("0x2222222222222222222222222222222222222222"),
		abiUintValue(2),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(data[:4]); got != selectorFor("setCollaborator(address,string,address,uint8)") {
		t.Fatalf("selector = %s", got)
	}
	args := data[4:]
	if got := readTestWordUint(args[0:32]); got != 0x1111111111111111 {
		t.Fatalf("owner word low value = %#x", got)
	}
	if got := readTestWordUint(args[64:96]); got != 0x2222222222222222 {
		t.Fatalf("collaborator word low value = %#x", got)
	}
	if got := readTestWordUint(args[96:128]); got != 2 {
		t.Fatalf("role word = %d", got)
	}
	if got := readTestWordUint(args[32:64]); got != 128 {
		t.Fatalf("repo offset = %d, want 128", got)
	}
	repo, err := readABIString(args, 128)
	if err != nil || repo != "demo" {
		t.Fatalf("repo = %q, err=%v", repo, err)
	}
}

func TestEncodeUpdateRepoInfoCalldataPreservesPatchFlags(t *testing.T) {
	data, err := encodeABICall(
		"updateRepoInfo(string,bool,string,bool,string)",
		abiStringValue("demo"), abiBoolValue(true), abiStringValue(""),
		abiBoolValue(false), abiStringValue(""),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(data[:4]); got != selectorFor("updateRepoInfo(string,bool,string,bool,string)") {
		t.Fatalf("selector = %s", got)
	}
	args := data[4:]
	if got := readTestWordUint(args[32:64]); got != 1 {
		t.Fatalf("description flag = %d, want 1", got)
	}
	if got := readTestWordUint(args[96:128]); got != 0 {
		t.Fatalf("default-branch flag = %d, want 0", got)
	}
	for _, tc := range []struct {
		name   string
		offset int
		want   string
	}{
		{name: "repo", offset: 0, want: "demo"},
		{name: "description", offset: 64, want: ""},
		{name: "default branch", offset: 128, want: ""},
	} {
		dynamicOffset := int(readTestWordUint(args[tc.offset : tc.offset+32]))
		got, err := readABIString(args, dynamicOffset)
		if err != nil {
			t.Fatalf("decode %s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDecodeCollaboratorCustomErrors(t *testing.T) {
	cases := []struct {
		signature string
		want      string
	}{
		{signature: "InvalidCollaborator(address)", want: "invalid collaborator"},
		{signature: "InvalidRole(uint8)", want: "invalid collaborator role"},
	}
	for _, tc := range cases {
		t.Run(tc.signature, func(t *testing.T) {
			digest := keccak256([]byte(tc.signature))
			data := "0x" + hex.EncodeToString(append(digest[:4], make([]byte, 32)...))
			err := decodeContractRevert(data)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestDecodeStableIdentityCustomErrors(t *testing.T) {
	var repoID [32]byte
	repoID[0] = 0xaa
	repoID[31] = 0x55
	owner := "0x1111111111111111111111111111111111111111"
	wantOwner, err := userAddressFromEVM(owner)
	if err != nil {
		t.Fatal(err)
	}

	notFoundData, err := encodeABICall(
		"LocatorNotFound(address,string)",
		abiAddressValue(owner),
		abiStringValue("demo"),
	)
	if err != nil {
		t.Fatal(err)
	}
	notFoundErr := decodeContractRevert("0x" + hex.EncodeToString(notFoundData))
	var locatorNotFound *LocatorNotFoundError
	if !errors.As(notFoundErr, &locatorNotFound) {
		t.Fatalf("error = %T %v, want LocatorNotFoundError", notFoundErr, notFoundErr)
	}
	if locatorNotFound.Owner != wantOwner || locatorNotFound.Name != "demo" {
		t.Fatalf("locator error = %#v", locatorNotFound)
	}
	if !isEVMLocatorNotFound(locatorNotFound) {
		t.Fatal("typed locator miss was not classified for legacy read fallback")
	}

	movedData, err := encodeABICall(
		"RepoMoved(bytes32,address,string)",
		abiBytes32Value(repoID),
		abiAddressValue(owner),
		abiStringValue("demo"),
	)
	if err != nil {
		t.Fatal(err)
	}
	movedErr := decodeContractRevert("0x" + hex.EncodeToString(movedData))
	var moved *RepoMovedError
	if !errors.As(movedErr, &moved) {
		t.Fatalf("error = %T %v, want RepoMovedError", movedErr, movedErr)
	}
	if moved.RepoID != repoID || moved.CurrentOwner != wantOwner || moved.Name != "demo" {
		t.Fatalf("moved error = %#v", moved)
	}
	if got := moved.CanonicalURL(); got != "igit://"+wantOwner+"/demo" {
		t.Fatalf("canonical URL = %q", got)
	}

	repoNotFoundData, err := encodeABICall(
		"RepoNotFound(bytes32)",
		abiBytes32Value(repoID),
	)
	if err != nil {
		t.Fatal(err)
	}
	repoNotFoundErr := decodeContractRevert("0x" + hex.EncodeToString(repoNotFoundData))
	var repoNotFound *RepoNotFoundError
	if !errors.As(repoNotFoundErr, &repoNotFound) || repoNotFound.RepoID != repoID {
		t.Fatalf("error = %T %v, want typed repo ID", repoNotFoundErr, repoNotFoundErr)
	}
	if isEVMLocatorNotFound(repoNotFoundErr) {
		t.Fatal("stable repo ID miss must not be classified as a legacy locator miss")
	}
}

func readTestWordUint(word []byte) uint64 {
	var value uint64
	for _, part := range word {
		value = value<<8 | uint64(part)
	}
	return value
}
