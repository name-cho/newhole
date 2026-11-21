//go:build !desktop
// +build !desktop

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/app/newhole"
	"github.com/name-cho/newhole/internal/pkg/cache"
	"github.com/name-cho/newhole/internal/pkg/closehandler"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/inpututil"
	"github.com/name-cho/newhole/internal/pkg/token"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "newhole",
	Short: "Newhole - expose local services through end to end TLS encrypted tunnels",
	Long: `Newhole exposes a local HTTP server, a directory or a WebDAV share to the world
through an end to end TLS encrypted tunnel.

Typical usage:

  newhole account login
  newhole http 3000
  newhole path ./public
  newhole webdav ./files`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		if lastFlag {
			if err := replayLastTunnel(cmd); err != nil {
				communication.Fatal(err.Error())
			}
			return
		}
		if !inpututil.IsTerminal() || inpututil.IsUsingPipe() {
			_ = cmd.Help()
			return
		}
		if err := runInteractive(cmd, token.IsTokenSaved()); err != nil {
			communication.Fatal(err.Error())
		}
	},
}

const logFileName = "newhole-%s.log"

func init() {
	rootCmd.PersistentFlags().BoolVarP(&config.Config.Display.Verbose, "verbose", "v", config.Config.Display.Verbose, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&config.Config.TLS.Insecure, "insecure", config.Config.TLS.Insecure, "skip TLS verification when talking to the Newhole API")
	rootCmd.PersistentFlags().StringVar(&config.Config.TLS.CABundle, "ca-bundle", config.Config.TLS.CABundle, "additional root CA bundle used to verify the Newhole API")
	rootCmd.PersistentFlags().StringVar(&config.Config.AuthToken, "token", config.Config.AuthToken, "access token to use instead of the stored credentials")
	rootCmd.PersistentFlags().BoolVar(&lastFlag, "last", false, "repeat the previous tunnel (interactive mode)")
}

func initLogger() {
	location := cache.GetLocalStorageFile(fmt.Sprintf(logFileName, time.Now().Format("2006-01-02--15-04-05")), "logs")
	file, err := os.OpenFile(location, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err == nil {
		communication.SetLogFile(file)
	}
	communication.SetVerbose(config.Config.Display.Verbose)
	communication.Debug("log file: " + location)
}

func Execute() {
	rootCmd.Version = fmt.Sprintf("%s (%s)", config.Config.Version, config.Config.CommitHash)
	newhole.SetVersion(config.Config.Version)

	initLogger()
	closehandler.SetupCloseHandler(config.Config.FeedbackFormURL)

	if err := rootCmd.Execute(); err != nil {
		communication.Fatal(err.Error())
	}
}
