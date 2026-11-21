//go:build !desktop
// +build !desktop

package cmd

import (
	"fmt"
	"time"

	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/token"
	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Print the account currently in use",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !token.IsTokenSaved() {
			return fmt.Errorf("not logged in, run `newhole account login`")
		}
		claims := token.Claims()
		name := "unknown account"
		for _, key := range []string{"name", "nickname", "email", "sub"} {
			if value, ok := claims[key]; ok {
				if text, ok := value.(string); ok && text != "" {
					name = text
					break
				}
			}
		}
		communication.Info(fmt.Sprintf("Logged in as %s", name))
		if lastUsed := token.LastUsedAt(); !lastUsed.IsZero() {
			communication.Info(fmt.Sprintf("Session last used at %s", lastUsed.Format(time.RFC3339)))
			inactive := time.Since(lastUsed)
			if inactive > 30*24*time.Hour {
				communication.Warn(fmt.Sprintf("The session was idle for %d days, the refresh token may have expired, log in again", int(inactive.Hours()/24)))
			}
		}
		return nil
	},
}

func init() {
	accountCmd.AddCommand(whoamiCmd)
}
