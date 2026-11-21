//go:build desktop && windows
// +build desktop,windows

package ui

import (
	webview2 "github.com/jchv/go-webview2"

	"github.com/name-cho/newhole/internal/pkg/cache"
)

func openNativeWindow(address string) bool {
	view := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  cache.GetLocalStorageDir("webview"),
		WindowOptions: webview2.WindowOptions{
			Title:  "Newhole",
			Width:  980,
			Height: 800,
			Center: true,
		},
	})
	if view == nil {
		return false
	}
	defer view.Destroy()

	view.Navigate(address)
	view.Run()
	return true
}
