//go:build !desktop
// +build !desktop

package main

import (
	"github.com/name-cho/newhole/cmd"
	"github.com/name-cho/newhole/config"
)

var (
	version = "development"
	commit  = "unknown"
	mode    = "cli"
)

func main() {
	config.Config.Version = version
	config.Config.CommitHash = commit
	config.Config.ClientMode = mode

	cmd.Execute()
}
