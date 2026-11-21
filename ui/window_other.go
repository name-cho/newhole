//go:build desktop && !windows
// +build desktop,!windows

package ui

func openNativeWindow(address string) bool {
	return false
}
