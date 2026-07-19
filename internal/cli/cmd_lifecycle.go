package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/archive"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/spf13/cobra"
)

// foreignDeleteMatches asks the configured archive (feature off → none) which
// foreign machines the delete's --project filter reaches. Best-effort by
// design: an unreadable archive config degrades to "no foreign matches"
// rather than blocking a purely local delete.
func foreignDeleteMatches(project string) []string {
	if project == "" {
		return nil
	}
	a, err := archive.Load()
	if err != nil || a == nil {
		return nil
	}
	return a.ForeignProjectMatches(project)
}

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
			"Archiving is idempotent: an already-archived session reports success.\n\n" +
			"The init/push subcommands manage something different: the transcript archive, " +
			"a private git remote mirroring this machine's transcript trees " +
			"(see `rawclaw archive init --help`).",
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
	yes         bool
	files       bool
}

// newDeleteCmd wires `rawclaw delete`: filter-gated, dry-run-first deletion of
// sessions — by filter flags, or of ONE session named positionally (the
// session8 prefix or full id from search output). It always computes the plan
// via a dry run first, prints it, and — unless --dry-run — prompts y/N on
// stdin before the real delete.
func newDeleteCmd() *cobra.Command {
	f := &deleteFlags{}
	cmd := &cobra.Command{
		Use:   "delete [session]",
		Short: "Delete a session by id, or sessions matching a filter (dry-run first, then y/N confirm)",
		Long: "Delete transcript sessions. Name ONE session positionally — the 8-char id " +
			"from search output, or the full id — or match a set with the filter flags " +
			"(--before/--project/--max-messages). One of the two is required; deleting " +
			"every session is refused. The plan is always shown first; without --dry-run " +
			"you are prompted y/N before anything is removed. A real delete writes a " +
			"tombstone so a reindex skips the removed sessions.\n\n" +
			"What a delete removes: the session's transcript file (when still on disk) " +
			"and rawclaw's copy (index + archive — the index row via tombstone, this " +
			"machine's archive copy on the next push). A RETAINED session — one whose transcript " +
			"the source tool already purged — loses only rawclaw's copy; Claude Code / " +
			"Codex transcript files are untouched. " +
			"Non-interactive: --yes alone covers retained-only deletes; a delete that " +
			"removes original transcript files additionally requires --files. " +
			"Foreign (other-machine) sessions are read-only from this machine and are " +
			"refused with a pointer at their origin machine.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd, f, args)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.before, "before", "", "only sessions last modified before this date (RFC3339 or YYYY-MM-DD)")
	fl.StringVar(&f.project, "project", "", "only sessions whose transcript-dir path contains this substring")
	fl.IntVar(&f.maxMessages, "max-messages", 0, "only sessions with at most N messages (drops thin/bootstrap threads)")
	fl.BoolVar(&f.dryRun, "dry-run", false, "report the plan without deleting anything")
	fl.BoolVarP(&f.yes, "yes", "y", false, "skip the interactive y/N prompt (deletes touching original files also need --files)")
	fl.BoolVar(&f.files, "files", false, "with --yes: also authorize deleting original transcript files non-interactively")
	return cmd
}

// runDelete builds the DeleteOpts (filters plus the optional positional
// session id), runs the dry-run plan, prints it, and (unless --dry-run)
// confirms y/N before the real delete.
func runDelete(cmd *cobra.Command, f *deleteFlags, args []string) error {
	out := cmd.OutOrStdout()

	sid := ""
	if len(args) == 1 {
		sid = strings.TrimSpace(args[0])
		if sid == "" {
			return ExitError{Code: 2, Msg: "empty session id; pass the 8-char id from search output (or the full id)"}
		}
	}

	before, err := parseBefore(f.before)
	if err != nil {
		return ExitError{Code: 2, Msg: err.Error()}
	}

	opts := lifecycle.DeleteOpts{
		Before:      before,
		Project:     f.project,
		MaxMessages: f.maxMessages,
		SessionID:   sid,
		DryRun:      true, // always plan first
	}

	root := paths.ProjectsRoot()
	plan, err := lifecycle.Delete(root, "", opts)
	if err != nil {
		if errors.Is(err, lifecycle.ErrNoFilter) {
			return ExitError{
				Code: 1,
				Msg:  "refusing to delete all sessions; name a session id, or pass --before/--project/--max-messages",
			}
		}
		return fmt.Errorf("plan delete: %w", err)
	}

	// Delete must also reach RETAINED sessions — rows whose backing .jsonl the
	// source tool already purged, but which still live in an index db
	// (missing_since set). matchSessions above only walks the live projects
	// tree, so without this union exactly the rows durable retention creates
	// are undeletable. Filters are the same ones the live plan just used
	// (including the positional session id), so a filter/id that matched
	// nothing live can still reach a retained row.
	retained, err := index.RetainedMatches("", f.project, before, f.maxMessages, sid)
	if err != nil {
		return fmt.Errorf("plan retained matches: %w", err)
	}

	// De-duplicate by session id: one session can match more than one row —
	// live AND stale-retained (file back on disk while missing_since lingers
	// from an earlier purge), or retained in TWO index dbs (the old and new
	// db of a renamed project dir). Counting duplicates would trip the
	// positional ambiguity guard on a single session — making it undeletable
	// by its own full id — and double-credit the summary. One tombstone entry
	// covers every row carrying the id, so keeping the live match (else the
	// first retained row) loses nothing.
	if len(retained) > 0 {
		seen := make(map[string]struct{}, len(plan.Matched)+len(retained))
		for _, it := range plan.Matched {
			seen[it.SessionID] = struct{}{}
		}
		kept := retained[:0]
		for _, r := range retained {
			if _, dup := seen[r.SessionID]; dup {
				continue
			}
			seen[r.SessionID] = struct{}{}
			kept = append(kept, r)
		}
		retained = kept
	}
	total := len(plan.Matched) + len(retained)

	// Foreign guard: a --project (or a positional session id) that reaches
	// another machine's archived scopes is named, never silently ignored —
	// foreign sessions are read-only from every box (delete them on their
	// origin machine). The probe is offline (clone-only) and best-effort: a
	// broken archive config must not block a purely local delete.
	foreign := foreignDeleteMatches(f.project)
	if sid != "" {
		foreign = mergeNames(foreign, foreignDeleteSessionMatches(sid))
	}

	// Positional unknown-id: a clear error, exit 1 — before any plan output.
	// (The filter form keeps its quieter "Nothing to delete." exit-0 shape: a
	// filter matching nothing is an answer; a named session that does not
	// exist is a mistake.) A foreign-only hit gets the origin-machine pointer.
	if sid != "" && total == 0 {
		if len(foreign) > 0 {
			return ExitError{Code: 1, Msg: fmt.Sprintf(
				"session %q was recorded on machine(s) %s; foreign sessions are read-only from this machine — delete it on its origin machine",
				sid, strings.Join(foreign, ", "))}
		}
		if f.project != "" || f.before != "" || f.maxMessages > 0 {
			return ExitError{Code: 1, Msg: fmt.Sprintf(
				"no session matches %q under the given filter flags — drop the filters, or use the 8-char id from search output (or the full id)", sid)}
		}
		return ExitError{Code: 1, Msg: fmt.Sprintf(
			"no session matches %q — use the 8-char id from search output (or the full id)", sid)}
	}

	printPlan(out, plan)
	printRetainedPlan(out, retained)

	// Positional ambiguity: an id/prefix addressing MORE than one session
	// deletes none of them — with --yes in play, fanning out silently would
	// let one session8 take out a second session by accident. The refusal
	// lists the FULL colliding ids (the plan rows above truncate to 8 chars,
	// which for a prefix collision renders identically) so the narrowed
	// retry can be copy-pasted.
	if sid != "" && total > 1 {
		ids := make([]string, 0, total)
		for _, it := range plan.Matched {
			ids = append(ids, it.SessionID)
		}
		for _, r := range retained {
			ids = append(ids, r.SessionID)
		}
		return ExitError{Code: 1, Msg: fmt.Sprintf(
			"%d sessions match %q — narrow it to the full session id:\n  %s",
			total, sid, strings.Join(ids, "\n  "))}
	}

	if len(foreign) > 0 {
		fmt.Fprintf(out,
			"Sessions on machine(s) %s in the archive also match; foreign sessions are read-only from this machine — delete them on their origin machine.\n",
			strings.Join(foreign, ", "))
	}

	// Foreign-only match is refused on BOTH the dry run and the real run —
	// the two must return the same verdict, or a script gating on the dry
	// run's exit code would read a different answer than the delete gives.
	if total == 0 && len(foreign) > 0 {
		return ExitError{Code: 1, Msg: fmt.Sprintf(
			"only foreign sessions matched (machine(s) %s); foreign sessions are read-only from this machine — run the delete on the origin machine",
			strings.Join(foreign, ", "))}
	}

	// Dry run, or nothing matched (live or retained): stop here without
	// touching disk.
	if f.dryRun {
		return nil
	}
	if total == 0 {
		fmt.Fprintln(out, "Nothing to delete.")
		return nil
	}

	// Confirmation gate. Interactive: the prompt says what dies — a delete that
	// removes LIVE sessions (original transcript files still on disk) names the
	// originals; a retained-only delete keeps the rawclaw's-copy-only shape.
	// Non-interactive: --yes alone covers retained-only deletes; when original
	// files would be deleted it refuses (exit 2) unless --files also authorizes
	// it — an agent must not take out the source tool's files on a bare --yes.
	// An EOF (non-tty / closed stdin) still aborts safely; an abort prints its
	// message and exits 1 (silently — the message is already out) so a script
	// can distinguish "aborted" from "deleted"; --dry-run above stays exit 0.
	liveFiles := len(plan.Matched) > 0
	if !f.yes {
		prompt := fmt.Sprintf("Delete %d session(s)? This is irreversible. [y/N]: ", total)
		if liveFiles {
			prompt = "This removes rawclaw's copy (index and archive) and the original session transcript files. Confirm with your user. [y/N]: "
		}
		ok, err := confirm(cmd.InOrStdin(), out, prompt)
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		if !ok {
			fmt.Fprintln(out, "Aborted; nothing deleted.")
			return ExitError{Code: 1}
		}
	} else if liveFiles && !f.files {
		return ExitError{Code: 2, Msg: "This delete removes original transcript files. Confirm with your user, then re-run with --yes --files."}
	}

	opts.DryRun = false
	done, err := lifecycle.Delete(root, "", opts)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	// Retained rows have no file to remove: tombstone-only. The next reconcile
	// pass (live UpdateIndex or the orphan-db discovery path) sees the
	// tombstone and prunes the row for real — the same mechanism an explicit
	// delete of a live session already relies on.
	if len(retained) > 0 {
		ids := make([]string, 0, len(retained))
		for _, r := range retained {
			ids = append(ids, r.SessionID)
		}
		if err := lifecycle.TombstoneIDs("", ids); err != nil {
			return fmt.Errorf("tombstone retained sessions: %w", err)
		}
	}

	fmt.Fprintf(out, "Deleted %d session(s) (%d retained), reclaimed %s. Tombstone: %s\n",
		len(done.Matched)+len(retained), len(retained), humanizeBytes(done.TotalBytes), done.TombstonePath)
	printProvenance(out, len(done.Matched))
	return nil
}

// printProvenance states what a real delete just removed, so nobody has to
// guess whose files died. A delete that removed live transcript files says so
// plainly; a retained-only delete removed no on-disk transcript — only
// rawclaw's copy — and the source tools' files are untouched.
func printProvenance(w io.Writer, liveRemoved int) {
	if liveRemoved > 0 {
		fmt.Fprintln(w, "Removed rawclaw's copy (index and archive) and the original session transcript files.")
		return
	}
	fmt.Fprintln(w, "Removed rawclaw's copy (index + archive). Claude Code / Codex transcript files are untouched.")
}

// foreignDeleteSessionMatches asks the configured archive (feature off →
// none) which foreign machines hold a session the positional id addresses.
// Best-effort like foreignDeleteMatches: an unreadable archive config
// degrades to "no foreign matches" rather than blocking a local delete.
func foreignDeleteSessionMatches(id string) []string {
	a, err := archive.Load()
	if err != nil || a == nil {
		return nil
	}
	return a.ForeignSessionMatches(id)
}

// mergeNames unions two name lists preserving order, dropping duplicates.
func mergeNames(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	add := func(names []string) {
		for _, n := range names {
			if _, dup := seen[n]; dup {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	add(a)
	add(b)
	return out
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

// printRetainedPlan renders the retained-session half of a delete plan: rows
// whose backing .jsonl is already gone, so unlike printPlan there is no file
// size to report — the label makes the distinct disk state explicit rather
// than silently reporting 0 B as if it were a small live file.
func printRetainedPlan(w io.Writer, retained []index.RetainedSession) {
	if len(retained) == 0 {
		return
	}
	fmt.Fprintf(w, "%d retained session(s) matched (source file already gone):\n\n", len(retained))
	for _, r := range retained {
		fmt.Fprintf(w, "  %s · %s · %d msg · 0 B (retained, source file already gone)\n",
			trunc8(r.SessionID), r.Label, r.MessageCount)
	}
	fmt.Fprintln(w)
}

// confirm prompts y/N on stdin and reports whether the user typed 'y'/'yes'.
// EOF (non-tty / closed stdin) or anything else is a "no" — never an error path
// that acts by default. Shared by every prompting command (delete, setup); the
// caller supplies its own prompt text.
func confirm(in io.Reader, w io.Writer, prompt string) (bool, error) {
	fmt.Fprint(w, prompt)
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
