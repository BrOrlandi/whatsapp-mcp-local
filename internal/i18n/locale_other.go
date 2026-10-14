//go:build !darwin && !windows

package i18n

// osLocale has nothing to add on Linux, where LANG is the setting.
func osLocale() string { return "" }
