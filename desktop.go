//go:build desktop
// +build desktop

package main

import (
	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/ui"
)

var (
	version = "development"
	commit  = "unknown"
	mode    = "desktop"
)

func main() {
	config.Config.Version = version
	config.Config.CommitHash = commit
	config.Config.ClientMode = mode

	ui.Display()
}
