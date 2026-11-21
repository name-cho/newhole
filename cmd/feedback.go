//go:build !desktop
// +build !desktop

package cmd

import (
	"fmt"

	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

var feedbackNoBrowser bool

var feedbackCmd = &cobra.Command{
	Use:     "feedback",
	Aliases: []string{"contact", "report"},
	Short:   "Send feedback or report a problem",
	Long: `Opens the feedback form of the project in the browser.

The same link is printed when a tunnel fails, so it is always one command away.

  newhole feedback
  newhole feedback --print`,
	RunE: func(cmd *cobra.Command, args []string) error {
		url := config.Config.FeedbackFormURL
		if url == "" {
			return fmt.Errorf("no feedback form is configured")
		}
		if feedbackNoBrowser {
			fmt.Println(url)
			return nil
		}
		if err := open.Run(url); err != nil {
			communication.Info(url)
			return fmt.Errorf("unable to open the browser: %w", err)
		}
		communication.Info("Opened the feedback form in the browser")
		return nil
	},
}

func init() {
	feedbackCmd.Flags().BoolVar(&feedbackNoBrowser, "print", false, "print the feedback URL instead of opening a browser")
	rootCmd.AddCommand(feedbackCmd)
}
