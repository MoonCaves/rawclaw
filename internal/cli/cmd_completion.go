package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newCompletionCmd wires `rawclaw completion [bash|zsh|fish|powershell]`:
// generate shell completion scripts.
func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for rawclaw.

To load completions:

Bash:
  $ source <(rawclaw completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ rawclaw completion bash > /etc/bash_completion.d/rawclaw
  # macOS:
  $ rawclaw completion bash > $(brew --prefix)/etc/bash_completion.d/rawclaw

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ rawclaw completion zsh > "${fpath[1]}/_rawclaw"

Fish:
  $ rawclaw completion fish | source

  # To load completions for each session, execute once:
  $ rawclaw completion fish > ~/.config/fish/completions/rawclaw.fish

PowerShell:
  PS> rawclaw completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> rawclaw completion powershell > rawclaw.ps1
  # and source this file from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.ExactArgs(1),
		SilenceUsage:          true,
		SilenceErrors:         true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(out, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(out)
			case "fish":
				return cmd.Root().GenFishCompletion(out, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(out)
			default:
				return fmt.Errorf("unsupported shell type %q, must be one of: bash, zsh, fish, powershell", args[0])
			}
		},
	}
}
