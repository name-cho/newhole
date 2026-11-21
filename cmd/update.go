//go:build !desktop
// +build !desktop

package cmd

import (
	"fmt"

	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/selfupdate"
	"github.com/spf13/cobra"
)

var (
	updateCheckOnly bool
	updateDesktop   bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Newhole to the newest release",
	Long: `Downloads the newest published release and replaces the running binary in place.

The download is verified by size and written next to the current binary, so the
update never leaves a half written executable behind.

  newhole update
  newhole update --check
  newhole update --desktop`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := selfupdate.Update(updateDesktop, updateCheckOnly)
		if err != nil {
			return err
		}
		if result.Skipped != "" {
			communication.Info(result.Skipped)
			return nil
		}
		communication.Info(fmt.Sprintf("Updated Newhole to %s (%s)", result.Version, result.Path))
		communication.Info("Restart the tunnel to use the new version")
		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "only report whether an update is available")
	updateCmd.Flags().BoolVar(&updateDesktop, "desktop", false, "update the desktop build instead of the CLI")
	rootCmd.AddCommand(updateCmd)
}
