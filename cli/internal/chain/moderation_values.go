package chain

import (
	"fmt"
	"strings"
)

func moderationStatusCode(status string) (uint64, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return 0, nil
	case "frozen":
		return 1, nil
	case "delisted":
		return 2, nil
	default:
		return 0, fmt.Errorf("invalid moderation status %q (expected active, frozen, or delisted)", status)
	}
}

func moderationStatusName(status uint64) (string, error) {
	switch status {
	case 0:
		return "active", nil
	case 1:
		return "frozen", nil
	case 2:
		return "delisted", nil
	default:
		return "", fmt.Errorf("EVM moderation module returned invalid status %d", status)
	}
}

func reportStatusName(status uint64) (string, error) {
	switch status {
	case 0:
		return "open", nil
	case 1:
		return "resolved", nil
	case 2:
		return "appealed", nil
	case 3:
		return "appeal_resolved", nil
	default:
		return "", fmt.Errorf("EVM moderation module returned invalid report status %d", status)
	}
}
