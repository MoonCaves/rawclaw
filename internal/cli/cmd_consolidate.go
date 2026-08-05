package cli

import (
	"fmt"
	"time"

	"github.com/MoonCaves/rawclaw/internal/durable"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/spf13/cobra"
)

// newConsolidateCmd wires `rawclaw consolidate`: fold every per-project index
// into the one store that holds all of them.
//
// Why the one store exists: relevance scores are computed per index, so a hit
// from a small project and a hit from a large one were never comparable, and
// stitching separate result lists together could not fix that. With every
// message in one index the scores share a denominator, and "which project" is
// a column you filter on rather than a file you picked in advance.
func newConsolidateCmd() *cobra.Command {
	var rebuild bool
	var fromTranscripts bool
	cmd := &cobra.Command{
		Use:   "consolidate",
		Short: "Fold every per-project index into the single search store",
		Long: "Fold every per-project index into the single store that holds all sessions, " +
			"so one search ranks every project against the same yardstick.\n\n" +
			"This reads the existing indexes directly — no transcript is parsed again — " +
			"and is safe to re-run: an unchanged index is skipped, and a session that " +
			"appears in more than one index is merged into one session carrying the union " +
			"of its messages. Indexing keeps the store up to date on its own; run this to " +
			"backfill it the first time, or after --rebuild.\n\n" +
			"--from-transcripts rebuilds the store from rawclaw's own transcript copies " +
			"instead of from the per-project indexes. That is the recovery path: the " +
			"indexes are a cache you may delete at any time, and this restores every " +
			"session from the raw transcripts — including sessions whose original file " +
			"the source tool has since purged. It needs no archive and no network.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if fromTranscripts {
				return runRebuildFromTranscripts(cmd)
			}
			srcs, err := index.PerProjectDBs()
			if err != nil {
				return fmt.Errorf("list indexes: %w", err)
			}
			if len(srcs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No indexes found — run a search first to build one.")
				return nil
			}
			start := time.Now()
			st, err := index.ConsolidateFrom(srcs, rebuild)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Consolidated %d indexes in %s\n", st.Sources-st.Skipped, time.Since(start).Round(time.Millisecond))
			fmt.Fprintf(out, "  %d sessions, %d messages\n", st.Sessions, st.Messages)
			// Say it out loud. A skipped index is one whose rows are NOT in the
			// store, and a pass that skipped everything would otherwise print a
			// success line over an empty result.
			if st.Skipped > 0 {
				fmt.Fprintf(out, "  %d of %d indexes skipped — too old to read; they are rebuilt the next time their project is searched\n",
					st.Skipped, st.Sources)
			}
			// Only worth saying when it happened: it is the merge doing its job,
			// and the plain counts alone would look like rows went missing.
			if merged := st.SessionsSeen - st.Sessions; merged > 0 {
				fmt.Fprintf(out, "  %d session rows merged (the same session indexed under more than one project)\n", merged)
			}
			fmt.Fprintf(out, "  store: %s\n", index.ConsolidatedPath())
			return nil
		},
	}
	cmd.Flags().BoolVar(&rebuild, "rebuild", false,
		"discard the store and refill it from every index")
	cmd.Flags().BoolVar(&fromTranscripts, "from-transcripts", false,
		"discard the store and rebuild it from rawclaw's own transcript copies (recovery path)")
	return cmd
}

// runRebuildFromTranscripts rebuilds the store from the transcript vault and
// reports what came back. The counts are printed rather than a bare "done"
// because this is the path a user reaches for after losing the store: they need
// to see that the retained-but-purged sessions survived, not just that a
// command exited zero.
func runRebuildFromTranscripts(cmd *cobra.Command) error {
	start := time.Now()
	st, err := index.RebuildFromTranscripts(index.ConsolidatedPath())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Rebuilt from transcripts in %s\n", time.Since(start).Round(time.Millisecond))
	fmt.Fprintf(out, "  %d sessions, %d messages\n", st.Sessions, st.Messages)
	if st.Missing > 0 {
		fmt.Fprintf(out, "  %d of them retained after their original transcript was purged\n", st.Missing)
	}
	if st.Tombstoned > 0 {
		fmt.Fprintf(out, "  %d skipped — you deleted them\n", st.Tombstoned)
	}
	// Never silent about a transcript we could not read: it is history that did
	// NOT come back, and a plain success line would hide that.
	if st.Unreadable > 0 {
		fmt.Fprintf(out, "  %d transcripts unreadable — those sessions did NOT come back\n", st.Unreadable)
	}
	fmt.Fprintf(out, "  transcripts: %s\n", durable.Root())
	fmt.Fprintf(out, "  store: %s\n", index.ConsolidatedPath())
	return nil
}
