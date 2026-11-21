//go:build !desktop
// +build !desktop

package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/state"
	"github.com/name-cho/newhole/internal/pkg/token"
	"github.com/spf13/cobra"
)

var hostnamesJSON bool

var hostnamesCmd = &cobra.Command{
	Use:     "hostnames",
	Aliases: []string{"list-hostnames", "hosts"},
	Short:   "List the hostnames used by this account",
	Long: `Lists the hostnames this machine has used with the account, newest first.

The history is stored locally in hostnames.json inside the Newhole storage
directory, so it survives restarts and logins.

  newhole account hostnames
  newhole account hostnames --json
  newhole account hostnames --forget my-hostname`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if forget, _ := cmd.Flags().GetString("forget"); forget != "" {
			if err := state.ForgetHostname(strings.ToLower(strings.TrimSpace(forget))); err != nil {
				return err
			}
			communication.Info(fmt.Sprintf("Removed %s from the local hostname history", forget))
			return nil
		}

		records := state.Hostnames()
		if len(records) == 0 {
			communication.Info("No hostnames have been used on this machine yet")
			return nil
		}
		if hostnamesJSON {
			payload, err := state.HostnamesJSON()
			if err != nil {
				return err
			}
			fmt.Println(string(payload))
			return nil
		}
		printHostnames(records)
		return nil
	},
}

func printHostnames(records []state.HostnameRecord) {
	hostnameWidth := len("HOSTNAME")
	domainWidth := len("DOMAIN")
	modeWidth := len("MODE")
	for _, record := range records {
		if len(record.Hostname) > hostnameWidth {
			hostnameWidth = len(record.Hostname)
		}
		if len(record.Domain) > domainWidth {
			domainWidth = len(record.Domain)
		}
		if len(record.ExposeMode) > modeWidth {
			modeWidth = len(record.ExposeMode)
		}
	}
	format := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%5s  %%s\n", hostnameWidth, domainWidth, modeWidth)
	fmt.Printf(format, "HOSTNAME", "DOMAIN", "MODE", "USES", "LAST USED")
	for _, record := range records {
		mode := record.ExposeMode
		if mode == "" {
			mode = "-"
		}
		fmt.Printf(format, record.Hostname, record.Domain, mode, fmt.Sprintf("%d", record.Uses), humanizeTime(record.LastUsed))
	}
}

func humanizeTime(value time.Time) string {
	if value.IsZero() {
		return "never"
	}
	elapsed := time.Since(value)
	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(elapsed.Hours()))
	case elapsed < 30*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(elapsed.Hours()/24))
	default:
		return value.Format("2006-01-02")
	}
}

var unregisterCmd = &cobra.Command{
	Use:     "unregister <hostname>",
	Aliases: []string{"release"},
	Short:   "Release a custom hostname",
	Long: `Releases a custom hostname so that other accounts can claim it.

The Loophole API has no endpoint for deleting a site, so Newhole releases the
hostname on the client side: the local history entry is dropped and the
hostname is not requested again. The gateway releases the name automatically
once the reservation expires (30 days after the last tunnel).

  newhole account unregister my-hostname`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !token.IsTokenSaved() {
			return errors.New("not logged in, run `newhole account login`")
		}
		hostname := strings.ToLower(strings.TrimSpace(args[0]))
		if hostname == "" {
			return errors.New("the hostname cannot be empty")
		}
		if err := state.ForgetHostname(hostname); err != nil {
			return err
		}
		communication.Info(fmt.Sprintf("Released %s", hostname))
		communication.Info("The gateway frees an unused custom hostname 30 days after its last tunnel")
		return nil
	},
}

func init() {
	hostnamesCmd.Flags().BoolVar(&hostnamesJSON, "json", false, "print the history as JSON")
	hostnamesCmd.Flags().String("forget", "", "remove a hostname from the local history")
	accountCmd.AddCommand(hostnamesCmd)
	accountCmd.AddCommand(unregisterCmd)
}
