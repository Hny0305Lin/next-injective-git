package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

type passwordTerminalCloser struct{ closed bool }

func (closer *passwordTerminalCloser) Close() error {
	closer.closed = true
	return nil
}

func TestReadEVMKeystorePasswordUsesDedicatedTerminalStreams(t *testing.T) {
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "")
	terminalErr := errors.New("dedicated terminal reached")
	openPasswordTerminal = func() (io.Reader, io.Writer, io.Closer, error) {
		return nil, nil, nil, terminalErr
	}
	t.Cleanup(func() { openPasswordTerminal = openSystemPasswordTerminal })

	_, err := readEVMKeystorePassword("password: ")
	if !errors.Is(err, terminalErr) || !strings.Contains(err.Error(), "EVM keystore password is required") {
		t.Fatalf("password error = %v", err)
	}
}

func TestReadEVMKeystorePasswordAutomationBypassesTerminal(t *testing.T) {
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "automation-only")
	openPasswordTerminal = func() (io.Reader, io.Writer, io.Closer, error) {
		t.Fatal("automation password opened the controlling terminal")
		return nil, nil, nil, nil
	}
	t.Cleanup(func() { openPasswordTerminal = openSystemPasswordTerminal })

	password, err := readEVMKeystorePassword("password: ")
	if err != nil || string(password) != "automation-only" {
		t.Fatalf("password = %q, err=%v", password, err)
	}
}

func TestPasswordTerminalCloserClosesBothWindowsHandles(t *testing.T) {
	input := &passwordTerminalCloser{}
	output := &passwordTerminalCloser{}
	if err := (closePasswordTerminals{input: input, output: output}).Close(); err != nil {
		t.Fatal(err)
	}
	if !input.closed || !output.closed {
		t.Fatalf("input closed=%v output closed=%v", input.closed, output.closed)
	}
}

func TestKeyPasswordRejectsProtocolPipeWithoutWritingPrompt(t *testing.T) {
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "")
	var protocol bytes.Buffer
	_, err := keyPassword("must-not-appear", strings.NewReader("secret\n"), &protocol)
	if err == nil || !strings.Contains(err.Error(), "non-interactive automation") {
		t.Fatalf("password error = %v", err)
	}
	if protocol.Len() != 0 {
		t.Fatalf("protocol stream was polluted: %q", protocol.String())
	}
}

func TestEVMKeystoreCreateAndSignKeepsIndexedKeyInsideDirectory(t *testing.T) {
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "test-password")
	dir := filepath.Join(t.TempDir(), "keys")
	cfg := config.Defaults()
	cfg.EVMKeystoreDir = dir
	cfg.KeyName = "dev"
	signer := NewEVMKeystoreSigner(cfg)

	if err := signer.CreateKey("dev"); err != nil {
		t.Fatal(err)
	}
	address, err := signer.OwnerAddress()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(address, "inj1") {
		t.Fatalf("owner address = %q, want inj1 prefix", address)
	}

	indexData, err := os.ReadFile(filepath.Join(dir, evmKeyIndexFile))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]string
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatal(err)
	}
	keyPath, ok := index["dev"]
	if !ok || !filepath.IsAbs(keyPath) {
		t.Fatalf("index = %#v, want an absolute dev key path", index)
	}
	keyAbs, _ := filepath.Abs(keyPath)
	dirAbs, _ := filepath.Abs(dir)
	if !strings.HasPrefix(strings.ToLower(keyAbs), strings.ToLower(dirAbs+string(os.PathSeparator))) {
		t.Fatalf("indexed key path %q escapes directory %q", keyAbs, dirAbs)
	}

	raw, err := signer.SignTransaction(context.Background(), EVMTransaction{
		ChainID:  31337,
		Nonce:    7,
		To:       "0x2222222222222222222222222222222222222222",
		Data:     "0x1234",
		GasLimit: 50_000,
		GasPrice: "0x3b9aca00",
		Value:    "0x",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := decodeHexBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(encoded); err != nil {
		t.Fatalf("decode signed transaction: %v", err)
	}
	if tx.ChainId().Uint64() != 31337 || tx.Nonce() != 7 || tx.Gas() != 50_000 || tx.GasPrice().Uint64() != 1_000_000_000 {
		t.Fatalf("signed transaction fields: chain=%d nonce=%d gas=%d gasPrice=%s", tx.ChainId(), tx.Nonce(), tx.Gas(), tx.GasPrice())
	}
	if tx.To() == nil || strings.ToLower(tx.To().Hex()) != "0x2222222222222222222222222222222222222222" {
		t.Fatalf("signed transaction destination = %v", tx.To())
	}

	if err := signer.CreateKey("dev"); err == nil {
		t.Fatal("duplicate key creation unexpectedly succeeded")
	}
}

func TestEVMKeystoreImportEncryptsStandardScryptKeyAndRebuildsAddress(t *testing.T) {
	const privateKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "import-password")
	dir := filepath.Join(t.TempDir(), "keys")
	cfg := config.Defaults()
	cfg.EVMKeystoreDir = dir
	signer := NewEVMKeystoreSigner(cfg)

	secret := []byte("0x" + privateKey + "\n")
	if err := signer.importKey("rotated-testnet", secret); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, []byte("0x"+privateKey+"\n")) {
		t.Fatal("importKey mutated caller-owned input before returning")
	}

	indexData, err := os.ReadFile(filepath.Join(dir, evmKeyIndexFile))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]string
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatal(err)
	}
	keyPath := index["rotated-testnet"]
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bytes.ToLower(keyData), []byte(privateKey)) {
		t.Fatal("plaintext private key appears in encrypted keystore JSON")
	}
	var envelope struct {
		Crypto struct {
			KDF       string         `json:"kdf"`
			KDFParams map[string]any `json:"kdfparams"`
		} `json:"crypto"`
	}
	if err := json.Unmarshal(keyData, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Crypto.KDF != "scrypt" || envelope.Crypto.KDFParams["n"] != float64(keystore.StandardScryptN) ||
		envelope.Crypto.KDFParams["p"] != float64(keystore.StandardScryptP) {
		t.Fatalf("keystore KDF = %#v, want standard scrypt", envelope.Crypto)
	}

	cfg.KeyName = "rotated-testnet"
	address, err := NewEVMKeystoreSigner(cfg).OwnerAddress()
	if err != nil {
		t.Fatal(err)
	}
	key, err := ethcrypto.HexToECDSA(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	wantAddress, err := userAddressFromEVM(ethcrypto.PubkeyToAddress(key.PublicKey).Hex())
	if err != nil {
		t.Fatal(err)
	}
	if address != wantAddress {
		t.Fatalf("imported address = %q, want %q", address, wantAddress)
	}
	if _, err := keystore.DecryptKey(keyData, "import-password"); err != nil {
		t.Fatalf("decrypt imported keystore: %v", err)
	}
	if err := signer.importKey("rotated-testnet", []byte(privateKey)); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate import error = %v", err)
	}
}

func TestEVMKeystoreRejectsIndexedPathOutsideDirectory(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.EVMKeystoreDir = dir
	cfg.KeyName = "dev"
	indexPath := filepath.Join(dir, evmKeyIndexFile)
	index, err := json.Marshal(map[string]string{"dev": filepath.Join("..", "outside")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, index, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEVMKeystoreSigner(cfg).OwnerAddress(); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("OwnerAddress error = %v, want outside-directory rejection", err)
	}
}

func TestEVMKeystoreRejectsInvalidTransactionDestination(t *testing.T) {
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "test-password")
	dir := filepath.Join(t.TempDir(), "keys")
	cfg := config.Defaults()
	cfg.EVMKeystoreDir = dir
	cfg.KeyName = "dev"
	signer := NewEVMKeystoreSigner(cfg)
	if err := signer.CreateKey("dev"); err != nil {
		t.Fatal(err)
	}
	_, err := signer.SignTransaction(context.Background(), EVMTransaction{
		ChainID:  31337,
		To:       "0x1234",
		GasPrice: "0x1",
	})
	if err == nil || !strings.Contains(err.Error(), "destination") {
		t.Fatalf("SignTransaction error = %v, want invalid destination", err)
	}
}

func TestEVMKeystoreSignsLegacyContractCreation(t *testing.T) {
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "test-password")
	dir := filepath.Join(t.TempDir(), "keys")
	cfg := config.Defaults()
	cfg.EVMKeystoreDir = dir
	cfg.KeyName = "deployer"
	signer := NewEVMKeystoreSigner(cfg)
	if err := signer.CreateKey("deployer"); err != nil {
		t.Fatal(err)
	}
	raw, err := signer.SignTransaction(context.Background(), EVMTransaction{
		ChainID: 1439, Nonce: 2, Data: "0x60006000f3", GasLimit: 100_000,
		GasPrice: "0x9896800", Value: "0x0",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := decodeHexBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	var transaction types.Transaction
	if err := transaction.UnmarshalBinary(encoded); err != nil {
		t.Fatal(err)
	}
	if transaction.Type() != types.LegacyTxType || transaction.To() != nil {
		t.Fatalf("contract creation type=%d to=%v", transaction.Type(), transaction.To())
	}
}

func TestEVMKeystoreRejectsAddressMetadataMismatch(t *testing.T) {
	t.Setenv("IGIT_EVM_KEY_PASSWORD", "test-password")
	dir := filepath.Join(t.TempDir(), "keys")
	cfg := config.Defaults()
	cfg.EVMKeystoreDir = dir
	cfg.KeyName = "dev"
	signer := NewEVMKeystoreSigner(cfg)
	if err := signer.CreateKey("dev"); err != nil {
		t.Fatal(err)
	}
	indexData, err := os.ReadFile(filepath.Join(dir, evmKeyIndexFile))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]string
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatal(err)
	}
	keyPath := index["dev"]
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(keyData, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata["address"] = "0000000000000000000000000000000000000000"
	tampered, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := signer.OwnerAddress(); err == nil || !strings.Contains(err.Error(), "addresses do not match") {
		t.Fatalf("OwnerAddress error = %v, want metadata mismatch", err)
	}
}
