//go:build !desktop
// +build !desktop

package cmd

import (
	"errors"

	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/token"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from your account",
	Long:  "Deletes the locally stored credentials. Cached certificates are kept.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !token.IsTokenSaved() {
			return errors.New("not logged in, nothing to do")
		}
		if err := token.DeleteTokens(); err != nil {
			communication.LogoutFailure(err)
			return err
		}
		communication.LogoutSuccess()
		return nil
	},
}

func init() {
	accountCmd.AddCommand(logoutCmd)
}
