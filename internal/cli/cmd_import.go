package cli

import (
	"fmt"
	"io"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/source/claudeweb"
	"github.com/spf13/cobra"
)

// newImportCmd wires `rawclaw import <zip|dir>`: ingest a Claude account
// data-export (the emailed Settings -> Privacy -> Export data ZIP, or an
// already-extracted copy of it) as the claude-web source, so cloud
// conversations become searchable through the normal `rawclaw "query"` /
// `read` / `outline` surface. Unlike claude/codex, claude-web has no live
// directory to re-scan — the export is a one-shot file the user drops anywhere —
// so this explicit command is its only ingest path.
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

// importResult is the machine-readable summary of one import: the
// reconciliation stats plus the source and db path.
type importResult struct {
	Source string                     `json:"source"`
	DB     string                     `json:"db"`
	Stats  index.ClaudeWebImportStats `json:"stats"`
}

// runImport parses the export at path and reconciles its conversations into the
// dedicated claude-web db (idempotent: re-importing appends only new messages
// and reconciles conversations absent from a newer export). It prints an
// added / updated / skipped summary. A malformed / non-Claude export surfaces
// as an error (from Discover), never a silent no-op.
func runImport(w io.Writer, path string, jsonOut bool) error {
	ad := claudeweb.New(path)

	containers, err := ad.Discover()
	if err != nil {
		return err
	}
	newest, err := ad.NewestUpdatedAt()
	if err != nil {
		return err
	}
	account, err := ad.Account()
	if err != nil {
		return err
	}

	// Split any legacy single db into per-account dbs FIRST (fail-closed), so a
	// pre-per-account db's conversations aren't stranded when this import writes
	// to the account db. A failure here blocks the import rather than risk a
	// split-brain across the two db layouts.
	if err := scopes.MigrateLegacyClaudeWeb(); err != nil {
		return fmt.Errorf("import %s: claude-web migration: %w", path, err)
	}

	dbp := scopes.ClaudeWebDBPath(account)
	stats, err := index.ImportClaudeWeb(dbp, containers, ad.Messages, claudeweb.ID, account, newest)
	if err != nil {
		return fmt.Errorf("import %s: %w", path, err)
	}

	res := importResult{Source: claudeweb.ID, DB: dbp, Stats: stats}
	if jsonOut {
		return EmitJSON(w, res)
	}
	fmt.Fprintf(w, "Imported as source %q: +%d new, ~%d updated, =%d unchanged conversation(s); +%d message(s).\n",
		res.Source, stats.AddedConversations, stats.UpdatedConversations, stats.SkippedConversations, stats.AddedMessages)
	if stats.RetainedAbsent > 0 || stats.PrunedAbsent > 0 {
		fmt.Fprintf(w, "Reconciled conversations absent from this export: %d retained (deleted-upstream), %d pruned.\n",
			stats.RetainedAbsent, stats.PrunedAbsent)
	}
	fmt.Fprintf(w, "%d conversation(s) total. Search with:  rawclaw \"<term>\"   (or --source %s to scope)\n",
		stats.TotalConversations, claudeweb.ID)
	return nil
}
