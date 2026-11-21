//go:build !desktop
// +build !desktop

package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate a shell completion script",
	Long: `Generate a completion script for the shell and load it.

Interactive shells, load once per session:

  bash:        source <(newhole completion bash)
  zsh:         source <(newhole completion zsh)
  fish:        newhole completion fish | source
  powershell:  newhole completion powershell | Out-String | Invoke-Expression

To keep the completions, write the script to the place the shell reads on start up:

  bash:        newhole completion bash > /etc/bash_completion.d/newhole
               (or ~/.local/share/bash-completion/completions/newhole)
  zsh:         newhole completion zsh > "${fpath[1]}/_newhole"
  fish:        newhole completion fish > ~/.config/fish/completions/newhole.fish
  powershell:  newhole completion powershell > newhole.ps1
               then dot-source it ( . .\newhole.ps1 ) from the PowerShell profile

On Windows the binary is newhole.exe, the arguments are identical.`,
	Args:                  cobra.ExactArgs(1),
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletion(os.Stdout)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
