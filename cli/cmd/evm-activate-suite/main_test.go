package main

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestComputeExpectedRootUsesSnapshotForEveryModule(t *testing.T) {
	for _, module := range moduleNames {
		moduleID := computeModuleID(module.fullName)
		encoded := append(append([]byte{}, moduleID[:]...), snapshotRoot[:]...)
		want := crypto.Keccak256Hash(encoded)
		if got := computeExpectedRoot(moduleID, snapshotRoot); got != want {
			t.Fatalf("%s root = %s, want %s", module.name, got.Hex(), want.Hex())
		}
	}
}

func TestParseRequiredHash(t *testing.T) {
	if value, err := parseRequiredHash(""); err != nil || value != (common.Hash{}) {
		t.Fatalf("empty hash = %s, %v", value.Hex(), err)
	}
	if _, err := parseRequiredHash(common.Hash{}.Hex()); err == nil {
		t.Fatal("zero evidence hash unexpectedly accepted")
	}
	want := common.HexToHash("0x1234")
	if got, err := parseRequiredHash(want.Hex()); err != nil || got != want {
		t.Fatalf("hash = %s, %v, want %s", got.Hex(), err, want.Hex())
	}
}
