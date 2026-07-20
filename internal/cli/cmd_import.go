package cli

import (
	"fmt"
	"io"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/source"
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

// importResult is the machine-readable summary of one import.
type importResult struct {
	Source        string `json:"source"`
	Conversations int    `json:"conversations"`
	Messages      int    `json:"messages"`
	DB            string `json:"db"`
}

// runImport parses the export at path, indexes its conversations into the
// dedicated claude-web db, and prints how many conversations and messages
// landed. A malformed / non-Claude export surfaces as an error (from Discover),
// never a silent no-op.
func runImport(w io.Writer, path string, jsonOut bool) error {
	ad := claudeweb.New(path)

	containers, err := ad.Discover()
	if err != nil {
		return err
	}

	dbp := scopes.ClaudeWebDBPath()
	// reindex=false: an import upserts into the existing db by conversation id,
	// so re-importing appends/updates rather than wiping prior imports. The
	// full-reconciliation semantics (tombstone / mirror-prune) are a later slice.
	nSessions, _, err := index.EnsureIndexedContainers(dbp, false, containers, ad.Messages, claudeweb.ID, "")
	if err != nil {
		return fmt.Errorf("import %s: %w", path, err)
	}

	res := importResult{
		Source:        claudeweb.ID,
		Conversations: nSessions,
		Messages:      countMessages(ad.Messages, containers),
		DB:            dbp,
	}
	if jsonOut {
		return EmitJSON(w, res)
	}
	fmt.Fprintf(w, "Imported %d conversation(s), %d message(s) as source %q.\n",
		res.Conversations, res.Messages, res.Source)
	fmt.Fprintf(w, "Search them with:  rawclaw \"<term>\"   (or --source %s to scope)\n", claudeweb.ID)
	return nil
}

// countMessages sums the normalized message count across the discovered
// conversations for the summary line. The adapter caches its parse, so this
// second pass over the containers is cheap; a container whose messages fail to
// load contributes zero, matching what the index actually wrote.
func countMessages(msgs index.MessagesFunc, containers []source.Container) int {
	var n int
	for _, c := range containers {
		ms, err := msgs(c)
		if err != nil {
			continue
		}
		n += len(ms)
	}
	return n
}
