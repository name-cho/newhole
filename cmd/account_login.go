//go:build !desktop
// +build !desktop

package cmd

import (
	"errors"

	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/token"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your account",
	Long: `Newhole requires authentication. This command starts the device authorisation flow:
open the printed URL, enter the printed code and the credentials are stored locally.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if token.IsTokenSaved() {
			return errors.New("already logged in, run `newhole account logout` first")
		}

		deviceCodeSpec, err := token.RegisterDevice()
		if err != nil {
			return err
		}
		communication.LoginStart(*deviceCodeSpec)

		tokens, err := token.PollForToken(deviceCodeSpec.DeviceCode, deviceCodeSpec.Interval, make(chan bool))
		if err != nil {
			return err
		}
		if err := token.SaveToken(tokens); err != nil {
			return err
		}
		communication.LoginSuccess(tokens.IDToken)
		return nil
	},
}

func init() {
	accountCmd.AddCommand(loginCmd)
}
