package chain

import (
	"fmt"
	"math/big"
	"strings"
)

func collaboratorRoleValue(role string) (uint64, error) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "none":
		return 0, nil
	case "maintainer":
		return 1, nil
	case "reader":
		return 2, nil
	default:
		return 0, fmt.Errorf("invalid collaborator role %q (maintainer|reader|none)", role)
	}
}

func collaboratorRoleName(role uint64) (string, error) {
	switch role {
	case 1:
		return "maintainer", nil
	case 2:
		return "reader", nil
	default:
		return "", fmt.Errorf("invalid collaborator role value %d", role)
	}
}

// evmINJValue converts the CLI's positive INJ base-unit representation to an
// EVM JSON-RPC quantity. Decimal INJ input is normalized by parseINJ first.
func evmINJValue(amount string) (string, error) {
	value := strings.TrimSpace(amount)
	if !strings.HasSuffix(value, "inj") {
		return "", fmt.Errorf("EVM sponsorship amount must use inj base units")
	}
	base := strings.TrimSuffix(value, "inj")
	integer, ok := new(big.Int).SetString(base, 10)
	if !ok || integer.Sign() <= 0 {
		return "", fmt.Errorf("invalid positive INJ base-unit amount %q", amount)
	}
	return "0x" + integer.Text(16), nil
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
