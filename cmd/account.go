//go:build !desktop
// +build !desktop

package cmd

import (
	"github.com/spf13/cobra"
)

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage the Newhole account",
	Long:  "Commands that manage the credentials stored by Newhole.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(accountCmd)
}
