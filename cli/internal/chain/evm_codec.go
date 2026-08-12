package chain

import (
	"encoding/hex"
	"fmt"
	"strings"
)

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

// NormalizeEVMAddress converts either an inj1 or hex address into canonical
// lower-case EVM form for deployment and migration tooling.
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

func hexEncode(data []byte) string {
	return hex.EncodeToString(data)
}
