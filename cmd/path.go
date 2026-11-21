//go:build !desktop
// +build !desktop

package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/name-cho/newhole/internal/app/newhole"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/token"
	"github.com/spf13/cobra"
)

var dirEndpointSpecs lm.LocalDirectorySpecs

var dirCmd = &cobra.Command{
	Use:     "path <path>",
	Aliases: []string{"dir", "directory"},
	Short:   "Expose a local directory over HTTP",
	Long: `Exposes a local directory over HTTP through a Newhole tunnel.

  newhole path ./public
  newhole path . --directory-listing`,
	RunE: func(cmd *cobra.Command, args []string) error {
		prepareRemote(0)
		remoteEndpointSpecs.ExposeMode = "path"

		directory, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("unable to resolve %q: %w", args[0], err)
		}
		info, err := os.Stat(directory)
		if err != nil {
			return fmt.Errorf("unable to read %q: %w", args[0], err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", args[0])
		}
		dirEndpointSpecs.Path = directory

		communication.ApplicationStart(token.IsTokenSaved(), token.GetIdToken())
		checkVersion()

		exposeConfig := lm.ExposeDirectoryConfig{
			Local:  dirEndpointSpecs,
			Remote: remoteEndpointSpecs,
		}

		authMethod, err := newhole.RegisterTunnel(&exposeConfig.Remote)
		if err != nil {
			return err
		}

		return newhole.ForwardDirectory(exposeConfig, authMethod, quitChannel())
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if daemonRequested(cmd, args) {
			return respawnAsDaemon(cmd, args)
		}
		return nil
	},
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("missing argument: path")
		}
		return nil
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return parseBasicAuthFlags(cmd.Flags())
	},
}

func init() {
	initServeCommand(dirCmd)
	dirCmd.Flags().BoolVar(&dirEndpointSpecs.DirectoryListing, "directory-listing", false, "show a file listing when there is no index.html")
	rootCmd.AddCommand(dirCmd)
}
