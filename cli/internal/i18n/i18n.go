// Package i18n provides the small runtime locale switch used by the CLI.
package i18n

import (
	"fmt"
	"strings"
)

// Text returns the Chinese translation when the user's locale is one of the
// four supported Chinese regions, and English otherwise.
func Text(english, chinese string) string {
	if IsChinese() {
		return chinese
	}
	return english
}

// Errorf formats a localized error message.
func Errorf(english, chinese string, args ...any) error {
	return fmt.Errorf(Text(english, chinese), args...)
}

// IsChineseLocale reports whether locale is one of the explicitly supported
// Chinese regions. Generic zh, Singaporean Chinese, and other locales remain
// English by design.
func IsChineseLocale(locale string) bool {
	locale = normalizeLocale(locale)
	if locale == "zh" || !strings.HasPrefix(locale, "zh-") {
		return false
	}
	for _, part := range strings.Split(locale[3:], "-") {
		switch part {
		case "cn", "hk", "mo", "tw":
			return true
		}
	}
	return false
}

func normalizeLocale(locale string) string {
	if i := strings.IndexByte(locale, ':'); i >= 0 {
		locale = locale[:i]
	}
	if i := strings.IndexAny(locale, ".@"); i >= 0 {
		locale = locale[:i]
	}
	locale = strings.ReplaceAll(locale, "_", "-")
	locale = lowerASCII(locale)
	if len(locale) >= 2 && locale[0:2] == "zh" {
		if len(locale) > 2 && locale[2] == '-' {
			return "zh-" + locale[3:]
		}
		return "zh"
	}
	return lowerASCII(locale)
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

func isGenericLocale(locale string) bool {
	locale = normalizeLocale(locale)
	return locale == "c" || locale == "posix"
}
