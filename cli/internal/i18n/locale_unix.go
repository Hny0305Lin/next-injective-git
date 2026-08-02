//go:build !windows

package i18n

import "os"

// IsChinese detects the conventional POSIX locale variables. Empty values
// fall through to English, which is the safe default for automation.
func IsChinese() bool {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		if locale := os.Getenv(key); locale != "" {
			if isGenericLocale(locale) {
				continue
			}
			return IsChineseLocale(locale)
		}
	}
	return false
}
