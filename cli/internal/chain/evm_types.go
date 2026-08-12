package chain

import (
	"context"
	"errors"
)

var ErrEVMSignerUnavailable = errors.New("EVM signer is not configured; import or create an encrypted EVM keystore")

// EVMTransaction is the secure signer boundary for legacy type-0 writes.
type EVMTransaction struct {
	ChainID  uint64
	Nonce    uint64
	To       string
	Data     string
	GasLimit uint64
	GasPrice string
	Value    string
}

// EVMSigner signs an EVM transaction without exposing private key material.
type EVMSigner interface {
	SignerBackend
	SignTransaction(ctx context.Context, tx EVMTransaction) (string, error)
}
