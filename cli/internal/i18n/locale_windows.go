//go:build windows

package i18n

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	getUserDefaultLocaleName = kernel32.NewProc("GetUserDefaultLocaleName")
)

// IsChinese reads the Windows user locale. Environment variables are honored
// first so tests and shell-launched tools can override the system setting.
func IsChinese() bool {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		if locale := os.Getenv(key); locale != "" {
			if isGenericLocale(locale) {
				continue
			}
			return IsChineseLocale(locale)
		}
	}
	buf := make([]uint16, LOCALE_NAME_MAX_LENGTH)
	if ret, _, _ := getUserDefaultLocaleName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); ret != 0 {
		return IsChineseLocale(syscall.UTF16ToString(buf))
	}
	return false
}

const LOCALE_NAME_MAX_LENGTH = 85
