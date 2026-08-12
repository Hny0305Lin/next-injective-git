package chain

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/term"
)

const evmKeyIndexFile = "index.json"

// EVMKeystoreSigner stores encrypted Ethereum keystore JSON on the local
// machine. The index contains only key names and file paths; private key
// material remains inside the scrypt-encrypted keystore file.
type EVMKeystoreSigner struct {
	cfg config.Config
}

// NewEVMKeystoreSigner creates the default local keystore signer.
func NewEVMKeystoreSigner(cfg config.Config) *EVMKeystoreSigner {
	return &EVMKeystoreSigner{cfg: cfg}
}

var _ EVMSigner = (*EVMKeystoreSigner)(nil)

func (s *EVMKeystoreSigner) keyDir() (string, error) {
	var dir string
	if value := strings.TrimSpace(s.cfg.EVMKeystoreDir); value != "" {
		dir = filepath.Clean(value)
	} else {
		var err error
		dir, err = config.Dir()
		if err != nil {
			return "", err
		}
	}
	// Resolve the directory at the boundary. An indexed path may be relative,
	// while CLI commands can be invoked from any working directory.
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s.cfg.EVMKeystoreDir) == "" {
		dir = filepath.Join(dir, "keystore")
	}
	return filepath.Clean(dir), nil
}

func (s *EVMKeystoreSigner) indexPath() (string, error) {
	dir, err := s.keyDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, evmKeyIndexFile), nil
}

func readEVMKeyIndex(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var index map[string]string
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("parse EVM keystore index: %w", err)
	}
	if index == nil {
		index = map[string]string{}
	}
	return index, nil
}

func writeEVMKeyIndex(path string, index map[string]string) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		// Windows does not replace an existing destination atomically. The index
		// contains no secret material, so retry after removing only that file.
		if removeErr := os.Remove(path); removeErr != nil {
			_ = os.Remove(tmp)
			return err
		}
		if retryErr := os.Rename(tmp, path); retryErr != nil {
			_ = os.Remove(tmp)
			return retryErr
		}
	}
	// Keep the index private even when an older, permissive file existed before
	// the atomic replacement. It reveals local key names and file locations.
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return nil
}

func validEVMKeyName(name string) bool {
	if strings.TrimSpace(name) == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\\:`)
}

// keyPassword reads a password without echoing it. IGIT_EVM_KEY_PASSWORD is
// accepted only for CI/offline automation; it is never written to config or
// logs. Interactive users are prompted through the terminal.
func keyPassword(prompt string, input io.Reader, output io.Writer) ([]byte, error) {
	if value := os.Getenv("IGIT_EVM_KEY_PASSWORD"); value != "" {
		return []byte(value), nil
	}
	file, ok := input.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return nil, fmt.Errorf("EVM keystore password is required; set IGIT_EVM_KEY_PASSWORD only for non-interactive automation")
	}
	if _, err := fmt.Fprint(output, prompt); err != nil {
		return nil, err
	}
	password, err := term.ReadPassword(int(file.Fd()))
	_, _ = fmt.Fprintln(output)
	if err != nil {
		return nil, fmt.Errorf("read EVM keystore password: %w", err)
	}
	if len(password) == 0 {
		return nil, fmt.Errorf("EVM keystore password cannot be empty")
	}
	return password, nil
}

// openPasswordTerminal returns streams dedicated to the user's controlling
// terminal. Git remote helpers receive stdin/stdout as the Git protocol, so
// using those process streams for a keystore prompt would corrupt the helper
// conversation or fail when Git supplies pipes. The platform-specific device
// names keep the prompt out of the protocol on both native Windows and POSIX.
var openPasswordTerminal = openSystemPasswordTerminal

func openSystemPasswordTerminal() (io.Reader, io.Writer, io.Closer, error) {
	if runtime.GOOS == "windows" {
		input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("open Windows console input: %w", err)
		}
		output, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		if err != nil {
			_ = input.Close()
			return nil, nil, nil, fmt.Errorf("open Windows console output: %w", err)
		}
		return input, output, closePasswordTerminals{input: input, output: output}, nil
	}
	terminal, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open controlling terminal: %w", err)
	}
	return terminal, terminal, terminal, nil
}

type closePasswordTerminals struct {
	input  io.Closer
	output io.Closer
}

func (terminals closePasswordTerminals) Close() error {
	inputErr := terminals.input.Close()
	outputErr := terminals.output.Close()
	if inputErr != nil {
		return inputErr
	}
	return outputErr
}

func readEVMKeystorePassword(prompt string) ([]byte, error) {
	if value := os.Getenv("IGIT_EVM_KEY_PASSWORD"); value != "" {
		return []byte(value), nil
	}
	input, output, closer, err := openPasswordTerminal()
	if err != nil {
		return nil, fmt.Errorf("EVM keystore password is required; %w", err)
	}
	defer closer.Close()
	return keyPassword(prompt, input, output)
}

func (s *EVMKeystoreSigner) CreateKey(name string) error {
	if !validEVMKeyName(name) {
		return fmt.Errorf("invalid EVM key name %q", name)
	}
	dir, err := s.keyDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create EVM keystore directory: %w", err)
	}
	indexPath, err := s.indexPath()
	if err != nil {
		return err
	}
	index, err := readEVMKeyIndex(indexPath)
	if err != nil {
		return err
	}
	if _, exists := index[name]; exists {
		return fmt.Errorf("EVM key %q already exists", name)
	}
	password, err := readEVMKeystorePassword("New EVM keystore password: ")
	if err != nil {
		return err
	}
	if os.Getenv("IGIT_EVM_KEY_PASSWORD") == "" {
		confirm, confirmErr := readEVMKeystorePassword("Confirm EVM keystore password: ")
		if confirmErr != nil {
			return confirmErr
		}
		if string(password) != string(confirm) {
			return fmt.Errorf("EVM keystore passwords do not match")
		}
	}
	ks := keystore.NewKeyStore(dir, keystore.StandardScryptN, keystore.StandardScryptP)
	account, err := ks.NewAccount(string(password))
	if err != nil {
		return fmt.Errorf("create encrypted EVM key: %w", err)
	}
	index[name] = account.URL.Path
	if err := writeEVMKeyIndex(indexPath, index); err != nil {
		return fmt.Errorf("write EVM keystore index: %w", err)
	}
	return nil
}

func (s *EVMKeystoreSigner) keyPath() (string, error) {
	if !validEVMKeyName(s.cfg.KeyName) {
		return "", fmt.Errorf("invalid or missing EVM key_name %q", s.cfg.KeyName)
	}
	indexPath, err := s.indexPath()
	if err != nil {
		return "", err
	}
	index, err := readEVMKeyIndex(indexPath)
	if err != nil {
		return "", err
	}
	path, ok := index[s.cfg.KeyName]
	if !ok || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("EVM key %q was not found; run `igit key new %s`", s.cfg.KeyName, s.cfg.KeyName)
	}
	dir, err := s.keyDir()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(strings.ToLower(abs), strings.ToLower(dirAbs+string(os.PathSeparator))) {
		return "", fmt.Errorf("EVM keystore index points outside the keystore directory")
	}
	return abs, nil
}

// indexedEVMAddress validates the public address recorded by geth in the
// keystore JSON against the address encoded in its filename. Both values are
// non-secret metadata; private key material is never decoded here.
func indexedEVMAddress(path string) (string, error) {
	base := filepath.Base(path)
	parts := strings.Split(base, "--")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid EVM keystore filename")
	}
	filenameAddress := strings.ToLower(parts[len(parts)-1])
	if len(filenameAddress) != 40 {
		return "", fmt.Errorf("invalid EVM keystore address filename")
	}
	if _, err := decodeHexBytes("0x" + filenameAddress); err != nil {
		return "", fmt.Errorf("invalid EVM keystore address filename: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read EVM keystore metadata: %w", err)
	}
	var metadata struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("parse EVM keystore metadata: %w", err)
	}
	stored := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(metadata.Address)), "0x"), "0X")
	if len(stored) != 40 {
		return "", fmt.Errorf("invalid EVM keystore address metadata")
	}
	if _, err := decodeHexBytes("0x" + stored); err != nil {
		return "", fmt.Errorf("invalid EVM keystore address metadata: %w", err)
	}
	if stored != filenameAddress {
		return "", fmt.Errorf("EVM keystore filename and metadata addresses do not match")
	}
	return filenameAddress, nil
}

func (s *EVMKeystoreSigner) OwnerAddress() (string, error) {
	path, err := s.keyPath()
	if err != nil {
		return "", err
	}
	hexAddress, err := indexedEVMAddress(path)
	if err != nil {
		return "", err
	}
	return userAddressFromEVM("0x" + hexAddress)
}

func (s *EVMKeystoreSigner) SignTransaction(ctx context.Context, tx EVMTransaction) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	path, err := s.keyPath()
	if err != nil {
		return "", err
	}
	expectedAddress, err := indexedEVMAddress(path)
	if err != nil {
		return "", err
	}
	password, err := readEVMKeystorePassword("EVM keystore password: ")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read EVM keystore: %w", err)
	}
	key, err := keystore.DecryptKey(data, string(password))
	if err != nil {
		return "", fmt.Errorf("decrypt EVM keystore: %w", err)
	}
	if key.PrivateKey == nil {
		return "", fmt.Errorf("EVM keystore has no private key")
	}
	derivedAddress := strings.TrimPrefix(strings.ToLower(evmKeyAddress(key.PrivateKey).Hex()), "0x")
	if derivedAddress != expectedAddress {
		return "", fmt.Errorf("EVM keystore private key does not match its address metadata")
	}
	toValue, err := normalizeEVMAddress(tx.To)
	if err != nil {
		return "", fmt.Errorf("invalid EVM transaction destination: %w", err)
	}
	to := common.HexToAddress(strings.TrimPrefix(toValue, "0x"))
	dataBytes, err := decodeHexBytes(tx.Data)
	if err != nil {
		return "", err
	}
	gasPrice, err := parseHexUint(tx.GasPrice)
	if err != nil {
		return "", fmt.Errorf("invalid EVM gas price: %w", err)
	}
	value := new(big.Int)
	if strings.TrimSpace(tx.Value) != "" {
		valueText := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(tx.Value), "0x"), "0X")
		if valueText == "" {
			valueText = "0"
		}
		parsed, ok := new(big.Int).SetString(valueText, 16)
		if !ok {
			return "", fmt.Errorf("invalid EVM transaction value")
		}
		value = parsed
	}
	unsigned := types.NewTx(&types.LegacyTx{
		Nonce:    tx.Nonce,
		To:       &to,
		Value:    value,
		Gas:      tx.GasLimit,
		GasPrice: new(big.Int).SetUint64(gasPrice),
		Data:     dataBytes,
	})
	signed, err := types.SignTx(unsigned, types.LatestSignerForChainID(new(big.Int).SetUint64(tx.ChainID)), key.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("sign EVM transaction: %w", err)
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("encode signed EVM transaction: %w", err)
	}
	return "0x" + fmt.Sprintf("%x", raw), nil
}

// Keep the concrete key type referenced so future hardware-backed signers can
// share the same address validation without exposing private material.
func evmKeyAddress(key *ecdsa.PrivateKey) common.Address {
	return ethcrypto.PubkeyToAddress(key.PublicKey)
}
