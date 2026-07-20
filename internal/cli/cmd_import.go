package cli

import (
	"fmt"
	"io"

	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/retention"
	"github.com/MoonCaves/rawclaw/internal/source/claudeweb"
	"github.com/spf13/cobra"
)

// newImportCmd wires `rawclaw import <zip|dir>`: ingest a Claude account
// data-export (the emailed Settings -> Privacy -> Export data ZIP, or an
// already-extracted copy of it) as the claude-web source. Import MATERIALIZES
// each conversation as a raw JSONL transcript under paths.ClaudeWebRoot(), then
// indexes those files — so cloud conversations become searchable through the
// normal `rawclaw "query"` / `read` / `outline` surface, and the raw transcripts
// are the durable truth (the index db is a rebuildable cache).
func newImportCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "import <zip|dir>",
		Short: "Import a Claude account data-export (cloud chat history) as the claude-web source",
		Long: "Import a Claude account data-export so your cloud conversations (claude.ai, the Desktop app, " +
			"Cowork) become searchable alongside your local sessions.\n\n" +
			"Point it at the emailed export ZIP or an already-extracted copy:\n\n" +
			"  rawclaw import ~/Downloads/data-<account>-<date>.zip\n" +
			"  rawclaw import ~/Downloads/claude-export/\n\n" +
			"Each conversation is saved as a raw transcript under the rawclaw data dir and indexed. " +
			"Imported conversations carry the `claude-web` source; scope a search with --source claude-web. " +
			"Cloud chats have no working directory, so they stay out of --this-project and surface on a normal " +
			"(all-projects) search.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd.OutOrStdout(), args[0], jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	return cmd
}

// importResult is the machine-readable summary of one import.
type importResult struct {
	Source        string `json:"source"`
	Accounts      int    `json:"accounts"`
	Conversations int    `json:"conversations"`
	Messages      int    `json:"messages"`
	Root          string `json:"transcript_root"`
}

// runImport materializes the export at path into the claude-web transcript tree
// (the durable source of truth), then reconciles each account (mirror-prunes
// files absent from a fresher export, staleness-guarded). The per-account cache
// dbs are (re)built lazily from these files at SEARCH time (scopes.ClaudeWeb),
// so import writes only the raw truth. A malformed / non-Claude export surfaces
// as an error with NO partial write (materialize stages then commits).
func runImport(w io.Writer, path string, jsonOut bool) error {
	root := paths.ClaudeWebRoot()
	mirror := retention.RetentionMirror()

	// Materialize the export → raw JSONL transcripts (merged, never-drop;
	// fail-closed; an account-less export is refused under mirror).
	res, err := claudeweb.Materialize(path, root, mirror)
	if err != nil {
		return fmt.Errorf("import %s: %w", path, err)
	}

	var totalConvs, totalMsgs int
	for _, ai := range res.Accounts {
		if err := claudeweb.Reconcile(ai, mirror); err != nil {
			return fmt.Errorf("import %s: reconcile account: %w", path, err)
		}
		totalConvs += ai.Written
		totalMsgs += ai.Messages
	}

	out := importResult{
		Source:        claudeweb.ID,
		Accounts:      len(res.Accounts),
		Conversations: totalConvs,
		Messages:      totalMsgs,
		Root:          root,
	}
	if jsonOut {
		return EmitJSON(w, out)
	}
	fmt.Fprintf(w, "Imported %d conversation(s) across %d account(s) as source %q.\n",
		out.Conversations, out.Accounts, out.Source)
	fmt.Fprintf(w, "Raw transcripts: %s\n", root)
	fmt.Fprintf(w, "Search with:  rawclaw \"<term>\"   (or --source %s to scope)\n", claudeweb.ID)
	return nil
}
