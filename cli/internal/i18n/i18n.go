// Package i18n provides the small runtime locale switch used by the CLI.
package i18n

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorCode identifies behavior independently from the localized message
// presented to the user.
type ErrorCode string

// CodedError keeps a stable machine-readable code alongside a localized error.
type CodedError struct {
	code ErrorCode
	err  error
}

func (e *CodedError) Error() string { return e.err.Error() }

func (e *CodedError) Unwrap() error { return e.err }

// Code returns the stable behavior identifier for this error.
func (e *CodedError) Code() ErrorCode { return e.code }

// Is lets errors.Is traverse nested and joined error trees by stable code.
func (e *CodedError) Is(target error) bool {
	coded, ok := target.(*CodedError)
	return ok && e.code == coded.code
}

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

// ErrorfCode formats a localized error and attaches a stable behavior code.
func ErrorfCode(code ErrorCode, english, chinese string, args ...any) error {
	return &CodedError{code: code, err: fmt.Errorf(Text(english, chinese), args...)}
}

// HasCode reports whether err or a wrapped error carries code.
func HasCode(err error, code ErrorCode) bool {
	return errors.Is(err, &CodedError{code: code})
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
