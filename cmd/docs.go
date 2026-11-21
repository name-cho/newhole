//go:build !desktop
// +build !desktop

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docsCommand = &cobra.Command{
	Use:    "docs",
	Short:  "Generate command documentation",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		directory := "./docs"
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
		if err := doc.GenMarkdownTree(rootCmd, directory); err != nil {
			return fmt.Errorf("unable to generate docs: %w", err)
		}
		fmt.Printf("documentation generated in %s\n", directory)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(docsCommand)
}
