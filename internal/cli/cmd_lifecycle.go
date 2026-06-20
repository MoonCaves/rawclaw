package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/spf13/cobra"
)

// newArchiveCmd wires `rawclaw archive <session>`: move one session's .jsonl out
// of the active projects tree into the archive dir, printing the new path. The
// session may be a bare id or a path; a missing one is a friendly "not found".
func newArchiveCmd() *cobra.Command {
	var archiveDir string
	cmd := &cobra.Command{
		Use:   "archive <session>",
		Short: "Move a session's transcript out of the active tree into the archive",
		Long: "Move a session's transcript (.jsonl) out of the active projects tree into the " +
			"archive dir. <session> is the 8+ char session id (or a path to the .jsonl). " +
			"Archiving is idempotent: an already-archived session reports success.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			newPath, err := lifecycle.Archive(args[0], archiveDir)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return ExitError{Code: 1, Msg: fmt.Sprintf("session not found: %q", args[0])}
				}
				return fmt.Errorf("archive %q: %w", args[0], err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), newPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&archiveDir, "archive-dir", "",
		"destination dir for the archived transcript (default ~/.claude/archive)")
	return cmd
}

// deleteFlags holds the parsed --delete flags for one invocation.
type deleteFlags struct {
	before      string
	project     string
	maxMessages int
	dryRun      bool
}

// newDeleteCmd wires `rawclaw delete`: filter-gated, dry-run-first deletion of
// sessions. It always computes the plan via a dry run first, prints it, and —
// unless --dry-run — prompts y/N on stdin before the real delete.
func newDeleteCmd() *cobra.Command {
	f := &deleteFlags{}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete sessions matching a filter (dry-run first, then y/N confirm)",
		Long: "Delete transcript sessions matching a filter. At least one filter " +
			"(--before/--project/--max-messages) is required; deleting every session " +
			"is refused. The plan is always shown first; without --dry-run you are " +
			"prompted y/N before anything is removed. A real delete writes a tombstone " +
			"so a reindex skips the removed sessions.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd, f)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.before, "before", "", "only sessions last modified before this date (RFC3339 or YYYY-MM-DD)")
	fl.StringVar(&f.project, "project", "", "only sessions whose transcript-dir path contains this substring")
	fl.IntVar(&f.maxMessages, "max-messages", 0, "only sessions with at most N messages (drops thin/bootstrap threads)")
	fl.BoolVar(&f.dryRun, "dry-run", false, "report the plan without deleting anything")
	return cmd
}

// runDelete builds the DeleteOpts, runs the dry-run plan, prints it, and (unless
// --dry-run) confirms y/N before the real delete.
func runDelete(cmd *cobra.Command, f *deleteFlags) error {
	out := cmd.OutOrStdout()

	before, err := parseBefore(f.before)
	if err != nil {
		return ExitError{Code: 2, Msg: err.Error()}
	}

	opts := lifecycle.DeleteOpts{
		Before:      before,
		Project:     f.project,
		MaxMessages: f.maxMessages,
		DryRun:      true, // always plan first
	}

	root := paths.ProjectsRoot()
	plan, err := lifecycle.Delete(root, "", opts)
	if err != nil {
		if errors.Is(err, lifecycle.ErrNoFilter) {
			return ExitError{
				Code: 1,
				Msg:  "refusing to delete all sessions; pass --before/--project/--max-messages",
			}
		}
		return fmt.Errorf("plan delete: %w", err)
	}

	printPlan(out, plan)

	// Dry run, or nothing matched: stop here without touching disk.
	if f.dryRun {
		return nil
	}
	if len(plan.Matched) == 0 {
		fmt.Fprintln(out, "Nothing to delete.")
		return nil
	}

	ok, err := confirm(cmd.InOrStdin(), out, len(plan.Matched))
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if !ok {
		fmt.Fprintln(out, "Aborted; nothing deleted.")
		return nil
	}

	opts.DryRun = false
	done, err := lifecycle.Delete(root, "", opts)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	fmt.Fprintf(out, "Deleted %d session(s), reclaimed %s. Tombstone: %s\n",
		len(done.Matched), humanizeBytes(done.TotalBytes), done.TombstonePath)
	return nil
}

// parseBefore parses the --before flag, accepting RFC3339 and YYYY-MM-DD. An
// empty value yields the zero time (unset filter).
func parseBefore(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid --before %q: want RFC3339 or YYYY-MM-DD", s)
}

// printPlan renders the delete plan: one line per matched session plus the total.
func printPlan(w io.Writer, plan lifecycle.DeletePlan) {
	if len(plan.Matched) == 0 {
		fmt.Fprintln(w, "No sessions match the filter.")
		return
	}
	fmt.Fprintf(w, "%d session(s) matched (%s total):\n\n", len(plan.Matched), humanizeBytes(plan.TotalBytes))
	for _, it := range plan.Matched {
		fmt.Fprintf(w, "  %s · %s · %d msg · %s\n",
			trunc8(it.SessionID), it.Project, it.Messages, humanizeBytes(it.Bytes))
	}
	fmt.Fprintln(w)
}

// confirm prompts y/N on stdin and reports whether the user typed 'y'/'yes'.
// EOF (non-tty / closed stdin) or anything else is a "no" — never an error path
// that deletes by default.
func confirm(in io.Reader, w io.Writer, n int) (bool, error) {
	fmt.Fprintf(w, "Delete %d session(s)? This is irreversible. [y/N]: ", n)
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return false, err
		}
		// EOF with no line (non-tty / closed stdin): treat as abort.
		fmt.Fprintln(w)
		return false, nil
	}
	ans := strings.ToLower(strings.TrimSpace(sc.Text()))
	return ans == "y" || ans == "yes", nil
}

// humanizeBytes renders a byte count as a short human string (B/KB/MB/GB/TB),
// using 1024-based units. Negative inputs are clamped to 0.
func humanizeBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
