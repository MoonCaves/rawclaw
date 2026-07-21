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
	var yes bool
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
			"(all-projects) search.\n\n" +
			"DELETION: by default nothing is ever deleted — a conversation you removed in Claude is kept and " +
			"labeled retained. Setting RAWCLAW_RETENTION=mirror makes a re-import prune imported conversations " +
			"absent from the newer export. It never deletes silently: it lists exactly which conversations would " +
			"go and asks you to confirm first (use --yes to approve non-interactively). In mirror mode import " +
			"only a COMPLETE export — an incomplete multi-part (…-batch-NNNN.zip) set can propose deleting " +
			"conversations that live in the parts you did not download.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd.OutOrStdout(), cmd.InOrStdin(), args[0], jsonOut, yes)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "approve mirror-mode deletions non-interactively (RAWCLAW_RETENTION=mirror)")
	return cmd
}

// importResult is the machine-readable summary of one import.
type importResult struct {
	Source        string `json:"source"`
	Accounts      int    `json:"accounts"`
	Conversations int    `json:"conversations"`
	Messages      int    `json:"messages"`
	Pruned        int    `json:"pruned"`
	Root          string `json:"transcript_root"`
}

// runImport materializes the export at path into the claude-web transcript tree
// (the durable source of truth). Under RAWCLAW_RETENTION=mirror it also computes
// which imported conversations are absent from this export and, only after
// explicit approval (a y/N prompt, or --yes), deletes them — never silently. The
// per-account cache dbs are (re)built lazily from these files at SEARCH time
// (scopes.ClaudeWeb), so import writes only the raw truth. A malformed /
// non-Claude export surfaces as an error with NO partial write.
func runImport(w io.Writer, in io.Reader, path string, jsonOut, yes bool) error {
	root := paths.ClaudeWebRoot()
	mirror := retention.RetentionMirror()

	// Materialize the export → raw JSONL transcripts (merged, never-drop;
	// fail-closed; an account-less export is refused under mirror).
	res, err := claudeweb.Materialize(path, root, mirror)
	if err != nil {
		return fmt.Errorf("import %s: %w", path, err)
	}

	// Under mirror, gather what WOULD be deleted (conversations absent from this
	// export) without deleting anything yet — deletes are gated behind approval.
	type acctPlan struct {
		ai    *claudeweb.AccountImport
		prune []string
	}
	var plans []acctPlan
	var totalConvs, totalMsgs, totalPrune int
	for _, ai := range res.Accounts {
		p := claudeweb.PrunePlan(ai, mirror)
		plans = append(plans, acctPlan{ai, p})
		totalConvs += ai.Written
		totalMsgs += ai.Messages
		totalPrune += len(p)
	}

	// Approval gate. Machine mode (--json) refuses rather than prompt; a human
	// gets the exact list of what dies and a y/N confirm; --yes pre-approves.
	approved := yes
	if totalPrune > 0 && !yes {
		if jsonOut {
			return ExitError{Code: 2, Msg: fmt.Sprintf(
				"mirror would delete %d conversation(s) absent from this export; re-run with --yes to approve", totalPrune)}
		}
		fmt.Fprintf(w, "mirror mode: %d imported conversation(s) are absent from this export and will be DELETED:\n", totalPrune)
		for _, pl := range plans {
			for _, uuid := range pl.prune {
				fmt.Fprintf(w, "  - %s\n", uuid)
			}
		}
		ok, cerr := confirm(in, w, "Delete them? This is irreversible. [y/N]: ")
		if cerr != nil {
			return fmt.Errorf("read confirmation: %w", cerr)
		}
		approved = ok
	}

	// Commit: delete the approved prune set (or nothing) and advance each
	// account's freshness watermark.
	var totalPruned int
	for _, pl := range plans {
		toDelete := pl.prune
		if !approved {
			toDelete = nil
		}
		if err := claudeweb.Commit(pl.ai, toDelete); err != nil {
			return fmt.Errorf("import %s: %w", path, err)
		}
		if approved {
			totalPruned += len(pl.prune)
		}
	}

	out := importResult{
		Source:        claudeweb.ID,
		Accounts:      len(res.Accounts),
		Conversations: totalConvs,
		Messages:      totalMsgs,
		Pruned:        totalPruned,
		Root:          root,
	}
	if jsonOut {
		return EmitJSON(w, out)
	}
	fmt.Fprintf(w, "Imported %d conversation(s) across %d account(s) as source %q.\n",
		out.Conversations, out.Accounts, out.Source)
	switch {
	case totalPruned > 0:
		fmt.Fprintf(w, "mirror mode: DELETED %d conversation(s) absent from this export.\n", totalPruned)
	case totalPrune > 0 && !approved:
		fmt.Fprintf(w, "Kept %d conversation(s) absent from this export (deletion declined).\n", totalPrune)
	}
	fmt.Fprintf(w, "Raw transcripts: %s\n", root)
	fmt.Fprintf(w, "Search with:  rawclaw \"<term>\"   (or --source %s to scope)\n", claudeweb.ID)
	return nil
}
