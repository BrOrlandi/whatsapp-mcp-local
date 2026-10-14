package i18n

import (
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

// osLocale is the display language of the person's Windows account: the
// first of the languages they prefer for Windows itself. The regional format
// (dates, numbers) can differ, as on an English Windows set to Brazil, and is
// only the fallback.
func osLocale() string {
	if lang := preferredUILanguage(); lang != "" {
		return lang
	}
	proc := kernel32.NewProc("GetUserDefaultLocaleName")
	if proc.Find() != nil {
		return ""
	}
	buf := make([]uint16, 85) // LOCALE_NAME_MAX_LENGTH
	if n, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// preferredUILanguage is the first name GetUserPreferredUILanguages gives,
// such as "en-US", or empty.
func preferredUILanguage() string {
	const muiLanguageName = 0x8
	proc := kernel32.NewProc("GetUserPreferredUILanguages")
	if proc.Find() != nil {
		return ""
	}
	var count, size uint32
	if ok, _, _ := proc.Call(muiLanguageName, uintptr(unsafe.Pointer(&count)), 0, uintptr(unsafe.Pointer(&size))); ok == 0 || size == 0 {
		return ""
	}
	buf := make([]uint16, size)
	if ok, _, _ := proc.Call(muiLanguageName, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); ok == 0 {
		return ""
	}
	// A list of names, each ended by a zero: the first is the one in use.
	return syscall.UTF16ToString(buf)
}
