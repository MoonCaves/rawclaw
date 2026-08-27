package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newCloseoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "closeout <full-session-id>",
		Short: "Show manual session-tagging recovery",
		Long: "The configured headless tagger seam is not present in this build, so closeout " +
			"does not simulate detached tagging. Recover with `rawclaw tag-prep <full-session-id>`, " +
			"then use the existing `rawclaw tag-write <full-session-id>` command.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCloseout(cmd.OutOrStdout(), args[0])
		},
	}
}

func runCloseout(w io.Writer, sessionID string) error {
	_, err := fmt.Fprintf(w, "closeout: no configured headless tagger; manual recovery: rawclaw tag-prep %s\n", sessionID)
	return err
}
