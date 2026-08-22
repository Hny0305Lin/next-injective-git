// One-shot local helper for importing the operator key into an encrypted
// geth keystore. It is intentionally kept out of the normal CLI surface.
package main

import (
	"bufio"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func main() {
	exportPath := flag.String("export", "", "wallet export path")
	wallet := flag.String("wallet", "wallet_2", "wallet section name")
	keyDir := flag.String("keystore", "", "encrypted keystore directory")
	passwordPath := flag.String("password-file", "", "password file")
	name := flag.String("name", "igit-demo", "key label")
	flag.Parse()
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*keyDir) == "" || strings.TrimSpace(*passwordPath) == "" {
		fatal("--export, --keystore, and --password-file are required")
	}
	secret, err := readWalletSecret(*exportPath, *wallet)
	if err != nil {
		fatal(err.Error())
	}
	defer clear(secret)
	password, err := os.ReadFile(*passwordPath)
	if err != nil {
		fatal(fmt.Sprintf("read password file: %v", err))
	}
	password = []byte(strings.TrimSpace(string(password)))
	if len(password) == 0 {
		fatal("password file is empty")
	}
	defer clear(password)
	key, err := ethcrypto.HexToECDSA(strings.TrimPrefix(strings.TrimPrefix(string(secret), "0x"), "0X"))
	if err != nil {
		fatal(fmt.Sprintf("decode private key: %v", err))
	}
	defer clearECDSA(key)
	if err := os.MkdirAll(*keyDir, 0o700); err != nil {
		fatal(fmt.Sprintf("create keystore directory: %v", err))
	}
	ks := keystore.NewKeyStore(*keyDir, keystore.StandardScryptN, keystore.StandardScryptP)
	account, err := ks.ImportECDSA(key, string(password))
	if err != nil {
		fatal(fmt.Sprintf("import encrypted key: %v", err))
	}
	_ = os.Chmod(account.URL.Path, 0o600)
	index, err := json.MarshalIndent(map[string]string{*name: account.URL.Path}, "", "  ")
	if err != nil {
		fatal(fmt.Sprintf("encode keystore index: %v", err))
	}
	if err := os.WriteFile(*keyDir+"/index.json", index, 0o600); err != nil {
		fatal(fmt.Sprintf("write keystore index: %v", err))
	}
	fmt.Println(account.Address.Hex())
}

func readWalletSecret(path, wanted string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	inSection := false
	var secret string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]") == wanted
			continue
		}
		if inSection && strings.HasPrefix(line, "private_key_hex=") {
			secret = strings.TrimSpace(strings.TrimPrefix(line, "private_key_hex="))
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(secret) != 64 && len(secret) != 66 {
		return nil, errors.New("wallet section does not contain a 64-character private key")
	}
	return []byte(secret), nil
}

func clear(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

func clearECDSA(key *ecdsa.PrivateKey) {
	if key != nil && key.D != nil {
		key.D.SetInt64(0)
	}
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
