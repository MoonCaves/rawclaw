package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
	"github.com/spf13/cobra"
)

// condenseCap caps each message's collapsed one-line content (in runes) in the
// condensed view dumped for a tagging subagent — enough to name the topic, not
// the full text.
const condenseCap = 200

// dumpByteCap is the total size ceiling (in bytes) for a tag-prep dump. Past
// this, the dump stops and prints a truncation note instead of handing a
// tagging subagent a session too large for its context — this happens for
// real: outlier sessions run into the thousands of messages. var, not const,
// so a test can shrink it to force truncation without fabricating huge sessions.
var dumpByteCap = 120_000

// uuid8Len is the prefix length of a message uuid printed by tag-prep and
// resolved by tag-write — mirrors the <uuid8> refs the search/read path uses.
const uuid8Len = 8

// rawSegment is one segment of the tag-write STDIN JSON: a uuid8 prefix marking
// where the topic begins, plus the subagent's topic label and inconclusive
// summary. Segments are expected in session order.
type rawSegment struct {
	StartUUID string `json:"start_uuid"`
	Topic     string `json:"topic"`
	Summary   string `json:"summary"`
}

// ── verb: tag-prep ────────────────────────────────────────────────────────────

// newTagPrepCmd wires `rawclaw tag-prep <session8>`: dump a session's messages
// condensed (one line per message, `<uuid8> [<role>] <text>`) to stdout for a
// tagging subagent to read. rawclaw calls no LLM — the subagent does the judging
// and feeds segments back via `tag-write`.
func newTagPrepCmd() *cobra.Command {
	var (
		thisProject bool
		dir         string
	)
	cmd := &cobra.Command{
		Use:   "tag-prep <session8>",
		Short: "Dump a session condensed for a tagging subagent to read",
		Long: "Print a session's messages condensed to one line each — `<uuid8> [<role>] <text>` — for a tagging " +
			"subagent to read and split into topic segments. rawclaw calls NO LLM: the subagent decides the " +
			"TOPIC segments + inconclusive summaries and feeds them back via `tag-write`. Takes the 8-char " +
			"session id from a search hit.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, more := verbScope(cmd.Context(), thisProject, dir, cmd.Flags().Changed("dir"))
			return runTagPrepCmd(cmd.OutOrStdout(), args[0], scope, more)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	return cmd
}

// runTagPrep is the testable core: load a session's messages and print the
// condensed dump (header + one `<uuid8> [<role>] <text>` line per message).
func runTagPrep(w io.Writer, con *sql.DB, fullSID string) error {
	msgs, err := loadSessionMessages(con, fullSID)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return fmt.Errorf("session %q has no messages to tag", lastSlice8(fullSID))
	}

	vis := visibleMessages(msgs)

	fmt.Fprintf(w, "# condensed session %s — one line per message: <uuid8> [<role>] <text>\n",
		lastSlice8(fullSID))
	fmt.Fprintf(w, "# split into contiguous TOPIC segments; feed back via: rawclaw tag-write %s\n",
		lastSlice8(fullSID))
	for i, m := range vis.msgs {
		if i == vis.gapAt {
			fmt.Fprintf(w, "# ⋯ middle of session omitted (dump exceeds %dKB) — a topic segment may NOT "+
				"span this gap; start a new segment for what follows\n", dumpByteCap/1000)
		}
		fmt.Fprintln(w, condensedLine(m))
	}
	return nil
}

// visibleSet is what a tag-prep dump actually shows: the messages themselves,
// plus the bookend gap boundary (gapAt = -1 if the whole session fit). The two
// always travel together — every consumer needs both to resolve or clamp
// segments correctly, so they're one value rather than two parameters that
// could drift out of sync.
type visibleSet struct {
	msgs  []store.SessionMessage
	gapAt int
}

// visibleMessages returns the subset of msgs a tag-prep dump actually shows:
// non-displayable rows dropped (tool-only, envelope-only, bare [THINKING]).
// When what's left still exceeds dumpByteCap, it bookends rather than
// truncating from the head alone — a session's setup AND its conclusion both
// matter for topic segmentation — keeping a head prefix and a tail suffix
// (each up to half the budget) and dropping the middle.
//
// writeSegments walks this SAME set (not the raw msgs) to resolve start_uuid
// and to compute segment end_uuid, clamping at gapAt — so a bookended dump can
// never cause tag-write to silently stretch a segment over the dropped middle,
// even if the tagging subagent ignores the gap note above and never splits.
func visibleMessages(msgs []store.SessionMessage) visibleSet {
	var displayable []store.SessionMessage
	for _, m := range msgs {
		if view.IsDisplayable(m.Content) {
			displayable = append(displayable, m)
		}
	}

	lines := make([]string, len(displayable))
	total := 0
	for i, m := range displayable {
		lines[i] = condensedLine(m)
		total += len(lines[i]) + 1
	}
	if total <= dumpByteCap {
		return visibleSet{msgs: displayable, gapAt: -1}
	}

	half := dumpByteCap / 2
	head, size := 0, 0
	for head < len(displayable) && size+len(lines[head])+1 <= half {
		size += len(lines[head]) + 1
		head++
	}
	remaining := dumpByteCap - size
	tail, tailSize := 0, 0
	for tail < len(displayable)-head && tailSize+len(lines[len(displayable)-1-tail])+1 <= remaining {
		tailSize += len(lines[len(displayable)-1-tail]) + 1
		tail++
	}

	shown := make([]store.SessionMessage, 0, head+tail)
	shown = append(shown, displayable[:head]...)
	shown = append(shown, displayable[len(displayable)-tail:]...)
	return visibleSet{msgs: shown, gapAt: head}
}

// ── verb: tag-write ────────────────────────────────────────────────────────────

// newTagWriteCmd wires `rawclaw tag-write <session8>`: read a JSON array of
// topic segments from STDIN (as decided by a tagging subagent) or mark the
// session routine via --routine, and upsert them into the index. rawclaw calls
// no LLM — this is the dumb write-back half of the prep/write pair.
func newTagWriteCmd() *cobra.Command {
	var (
		thisProject bool
		dir         string
		routine     bool
		source      string
	)
	cmd := &cobra.Command{
		Use:     "tag-write <session8>",
		Aliases: []string{"tag-floor"},
		Short:   "Write a tagging subagent's topic segments (JSON on STDIN) or routine verdict to the index",
		Long: "Read a JSON array of topic segments from STDIN and store them in the topic index used by " +
			"search/outline. Each segment: {\"start_uuid\":\"<uuid8 prefix>\",\"topic\":\"...\",\"summary\":\"...\"}, " +
			"in session order. start_uuid is prefix-resolved against the session's message uuids; each segment's " +
			"end is the message just before the next segment's start (the last message for the final segment).\n\n" +
			"Pass --routine to mark the session with a routine verdict (trivial / low-signal; sorts down, never hidden). " +
			"Use the tag-floor alias with no session argument to sweep the consolidated corpus using the " +
			"deterministic math floor; it makes no LLM or API calls. rawclaw calls NO LLM — a tagging subagent " +
			"decides the segments or routine verdict and pipes/flags them here.",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.CalledAs() == "tag-floor" {
				if len(args) > 1 {
					return cobra.MaximumNArgs(1)(cmd, args)
				}
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, more := verbScope(cmd.Context(), thisProject, dir, cmd.Flags().Changed("dir"))
			if cmd.CalledAs() == "tag-floor" {
				return runTagFloorCmd(cmd.OutOrStdout(), scope)
			}
			return runTagWriteCmd(cmd.OutOrStdout(), cmd.InOrStdin(), args[0], scope, more, routine, source)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	f.BoolVar(&routine, "routine", false, "mark the session with a routine verdict (trivial / low-signal; sorts down)")
	f.StringVar(&source, "source", store.VerdictSourceAgent, "verdict source (agent|floor)")
	return cmd
}

// runTagFloorCmd evaluates every session in the consolidated store. The
// consolidated messages are already parser output, so the floor uses the
// parser's substantive-human filter instead of the inflated sessions count.
// A per-session read error is unknown and is never written as routine.
func runTagFloorCmd(w io.Writer, scope []view.Scope) error {
	con, err := store.ConnectRW(index.ConsolidatedPath())
	if err != nil {
		return fmt.Errorf("open consolidated store read-write: %w", err)
	}
	defer con.Close()
	if err := store.EnsureTopicSchema(con); err != nil {
		return fmt.Errorf("ensure topic schema: %w", err)
	}

	query := "SELECT id FROM sessions"
	args := []any{}
	if len(scope) > 0 {
		if len(scope) != 1 || scope[0].Project == "" {
			return fmt.Errorf("tag-floor: unsupported project scope")
		}
		query += " WHERE project=?"
		args = append(args, scope[0].Project)
	}
	query += " ORDER BY id"
	rows, err := con.Query(query, args...)
	if err != nil {
		return fmt.Errorf("list sessions for floor: %w", err)
	}
	var sessionIDs []string
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan session for floor: %w", err)
		}
		sessionIDs = append(sessionIDs, sid)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate sessions for floor: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close sessions for floor: %w", err)
	}

	var evaluated, marked, normal, unknown int
	for _, sid := range sessionIDs {
		evaluated++
		msgs, err := store.SessionMessages(con, sid)
		if err != nil {
			unknown++
			continue
		}
		modelMsgs := make([]model.Message, len(msgs))
		for i, msg := range msgs {
			modelMsgs[i] = model.Message{Role: msg.Role, Text: msg.Content}
		}
		isRoutine, _ := lifecycle.EvaluateMathFloor(modelMsgs)
		if err := applyFloorRoutine(con, sid, isRoutine, nowUnix()); err != nil {
			return err
		}
		if isRoutine {
			marked++
		} else {
			normal++
		}
	}
	fmt.Fprintf(w, "Evaluated %d sessions: marked %d as routine (floor), %d normal, %d unknown\n",
		evaluated, marked, normal, unknown)
	return nil
}

func applyFloorRoutine(con *sql.DB, sessionID string, isRoutine bool, taggedAt float64) error {
	if !isRoutine {
		if _, err := con.Exec("DELETE FROM session_verdict WHERE session_id=? AND source=?", sessionID, store.VerdictSourceFloor); err != nil {
			return fmt.Errorf("retract floor verdict for %s: %w", lastSlice8(sessionID), err)
		}
		return nil
	}
	return runTagWriteRoutine(con, sessionID, store.VerdictSourceFloor, taggedAt)
}

// runTagWriteCmd resolves session8 → db + full id, opens the db read-write,
// ensures the topic schema, and runs the populate pass reading JSON from r (or
// writing the routine verdict) with a now() timestamp. Thin wrapper around the
// testable runTagWrite / runTagWriteRoutine core.
func runTagWriteCmd(w io.Writer, r io.Reader, session8 string, scope []view.Scope, more agentproto.ScopeFn, routine bool, source string) error {
	dbp, fullSID, err := agentproto.LocateSession(session8, scope, more)
	if err != nil {
		return err
	}

	con, err := store.ConnectRW(dbp)
	if err != nil {
		return fmt.Errorf("open %q read-write: %w", dbp, err)
	}

	var writeErr error
	var n int
	if routine {
		writeErr = runTagWriteRoutine(con, fullSID, source, nowUnix())
	} else {
		n, writeErr = runTagWrite(con, fullSID, r, nowUnix())
	}
	_ = con.Close() // close before folding in: the fold attaches this db read-only
	if writeErr != nil {
		return writeErr
	}

	// Fold the new topic rows into the consolidated store, the same write-through
	// an indexing run does. Without it a tag written today stays invisible to the
	// one-store readers until the next `rawclaw consolidate`. Advisory: the
	// consolidated store is a derived artifact, so a failed fold is a stale cache,
	// never a failed tag-write.
	if err := index.SyncConsolidatedFrom(dbp); err != nil {
		slog.Debug("tag-write: consolidated write-through failed", "db", filepath.Base(dbp), "err", err)
	}

	if routine {
		if source == "" {
			source = store.VerdictSourceAgent
		}
		fmt.Fprintf(w, "marked %s as routine (source: %s)\n", lastSlice8(fullSID), source)
	} else {
		fmt.Fprintf(w, "wrote %d topic segments for %s\n", n, lastSlice8(fullSID))
	}
	return nil
}

// runTagWriteRoutine writes a session's routine verdict with the given source
// and taggedAt timestamp, clearing any prior topic segments so the session is
// effectively routine (a real topic segment demotes routine at read time).
func runTagWriteRoutine(con *sql.DB, fullSID, source string, taggedAt float64) error {
	if err := store.EnsureTopicSchema(con); err != nil {
		return fmt.Errorf("ensure topic schema: %w", err)
	}
	if source == "" {
		source = store.VerdictSourceAgent
	}
	if source == store.VerdictSourceFloor {
		source, ok, err := verdictSource(con, fullSID)
		if err != nil {
			return fmt.Errorf("read existing verdict: %w", err)
		}
		if ok && source == store.VerdictSourceAgent {
			return nil
		}
	}
	if err := store.UpsertVerdict(con, store.Verdict{
		SessionID: fullSID,
		Verdict:   store.VerdictRoutine,
		Source:    source,
		TaggedAt:  taggedAt,
	}); err != nil {
		return err
	}
	if source == store.VerdictSourceFloor {
		return nil
	}
	return store.ReplaceSessionSegments(con, fullSID, nil)
}

func verdictSource(con *sql.DB, sessionID string) (string, bool, error) {
	var source string
	err := con.QueryRow("SELECT source FROM session_verdict WHERE session_id=?", sessionID).Scan(&source)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return source, true, nil
}

// runTagWrite is the testable core: decode the segment array from r, resolve each
// start_uuid prefix against the session's messages, compute each segment's
// end_uuid (the message just before the next segment's start; the last message
// for the final segment), and upsert one topic_segment row per valid segment.
// Returns the number of rows written. taggedAt is passed in (the CLI uses
// time.Now; a test pins it) so the populate logic stays free of a clock read.
func runTagWrite(con *sql.DB, fullSID string, r io.Reader, taggedAt float64) (int, error) {
	if err := store.EnsureTopicSchema(con); err != nil {
		return 0, fmt.Errorf("ensure topic schema: %w", err)
	}

	msgs, err := loadSessionMessages(con, fullSID)
	if err != nil {
		return 0, err
	}
	if len(msgs) == 0 {
		return 0, fmt.Errorf("session %q has no messages to tag", lastSlice8(fullSID))
	}
	vis := visibleMessages(msgs)
	if len(vis.msgs) == 0 {
		return 0, fmt.Errorf("session %q has no messages to tag", lastSlice8(fullSID))
	}

	var segs []rawSegment
	dec := json.NewDecoder(r)
	if err := dec.Decode(&segs); err != nil {
		return 0, fmt.Errorf("decode tag-write JSON (want an array of {start_uuid,topic,summary}): %w", err)
	}
	if len(segs) == 0 {
		return 0, fmt.Errorf("tag-write got an empty segment array — nothing to write")
	}

	return writeSegments(con, fullSID, vis, segs, taggedAt)
}

// ── shared helpers ────────────────────────────────────────────────────────────

// loadSessionMessages reads a session's messages in id order (id ascending) — the
// chronological spine the dump and the segment-range mapping both walk.
func loadSessionMessages(con *sql.DB, fullSID string) ([]store.SessionMessage, error) {
	msgs, err := store.SessionMessages(con, fullSID)
	if err != nil {
		return nil, fmt.Errorf("load session messages: %w", err)
	}
	return msgs, nil
}

// condensedLine renders one message as `<uuid8> [<role>] <text>`, collapsing the
// content to a single capped line via parse.Disp (tools stripped). uuid8 is the
// ref a tagging subagent echoes back in tag-write's start_uuid.
func condensedLine(m store.SessionMessage) string {
	text := parse.Disp(m.Content, false, condenseCap)
	return fmt.Sprintf("%s [%s] %s", uuid8(m.UUID), m.Role, text)
}

// uuid8 returns the first uuid8Len characters of a uuid (the printed prefix); a
// shorter uuid is returned whole.
func uuid8(u string) string {
	if len(u) <= uuid8Len {
		return u
	}
	return u[:uuid8Len]
}

// writeSegments maps the subagent's segments to topic_segment rows and REPLACES
// the session's whole segment set with them (ReplaceSessionSegments) — so a
// re-tag redoes the tags instead of stacking a second set beside the first.
// start_uuid is prefix-resolved to a message's full uuid; end_uuid is the full
// uuid of the message just BEFORE the next segment's start (the session's last
// message for the final segment). A segment missing start_uuid/topic, or whose
// start_uuid resolves to no/ambiguous message, returns a clear error (and, since
// the whole set is applied in one transaction, leaves the prior tags untouched).
//
// vis.gapAt is visibleMessages' bookend boundary (-1 if none): a segment
// starting before it never gets an end past it, even if the subagent's
// segments imply otherwise — a bookended dump's dropped middle can never be
// silently claimed by whatever topic was open on either side of it. And a
// bookended dump with a real tail (gapAt < len(msgs)) REQUIRES at least one
// segment starting at or after the gap: a lazy single segment covering "the
// whole thing" is rejected outright, rather than accepted-but-clamped. A
// session never gets left half-labeled — either it's tagged properly on both
// sides of the gap, or tag-write errors and the subagent tries again.
//
// Returns the number of rows written.
func writeSegments(con *sql.DB, fullSID string, vis visibleSet, segs []rawSegment, taggedAt float64) (int, error) {
	msgs, gapAt := vis.msgs, vis.gapAt
	if len(msgs) == 0 {
		return 0, nil
	}
	lastIdx := len(msgs) - 1

	// First pass: validate each row and resolve its start_uuid → message index, so
	// the end-boundary computation can look at the next segment's resolved index.
	startIdx := make([]int, len(segs))
	coversTail := false
	for i, seg := range segs {
		if strings.TrimSpace(seg.StartUUID) == "" {
			return 0, fmt.Errorf("segment %d: missing start_uuid", i)
		}
		if strings.TrimSpace(seg.Topic) == "" {
			return 0, fmt.Errorf("segment %d (start_uuid %q): missing topic", i, seg.StartUUID)
		}
		idx, err := resolveStartUUID(msgs, seg.StartUUID)
		if err != nil {
			return 0, fmt.Errorf("segment %d: %w", i, err)
		}
		startIdx[i] = idx
		if idx >= gapAt {
			coversTail = true
		}
	}
	if gapAt >= 0 && gapAt < len(msgs) && !coversTail {
		return 0, fmt.Errorf("dump was bookended (middle omitted) but no segment starts at or after " +
			"the tail shown in the dump — segments must cover BOTH sides of the gap, not just the head")
	}

	out := make([]store.TopicSegment, len(segs))
	for i, seg := range segs {
		// end = the index of the message just before the NEXT segment's start.
		endIdx := lastIdx
		if i+1 < len(segs) {
			nextIdx := startIdx[i+1]
			if nextIdx > 0 {
				endIdx = nextIdx - 1
			} else {
				endIdx = nextIdx
			}
		}
		// Clamp: a segment starting before the gap can't end past it.
		if gapAt >= 0 && startIdx[i] < gapAt && endIdx >= gapAt {
			endIdx = gapAt - 1
		}

		out[i] = store.TopicSegment{
			SessionID: fullSID,
			StartUUID: msgs[startIdx[i]].UUID,
			EndUUID:   msgs[endIdx].UUID,
			Topic:     seg.Topic,
			Summary:   seg.Summary,
			TaggedAt:  taggedAt,
			// OriginMachine left empty — local authoring, stored NULL.
		}
	}

	// Apply the tagging as ONE unit: DELETE the session's prior rows, INSERT this
	// set. A re-tag replaces; it never stacks a stale set beside the fresh one.
	if err := store.ReplaceSessionSegments(con, fullSID, out); err != nil {
		return 0, err
	}
	return len(out), nil
}

// resolveStartUUID resolves a uuid prefix against the session's message uuids,
// mirroring the read path's uuid8 resolution: exactly one match wins; zero
// matches and more-than-one match are both clear errors. Returns the matching
// message's index in msgs.
func resolveStartUUID(msgs []store.SessionMessage, prefix string) (int, error) {
	match := -1
	for i, m := range msgs {
		if strings.HasPrefix(m.UUID, prefix) {
			if match >= 0 {
				return 0, fmt.Errorf("start_uuid %q is ambiguous (matches multiple messages)", prefix)
			}
			match = i
		}
	}
	if match < 0 {
		return 0, fmt.Errorf("start_uuid %q matches no message in this session", prefix)
	}
	return match, nil
}

// nowUnix is the CLI-runtime wall-clock stamp for tagged_at (seconds since the
// epoch). It lives at the command edge — runTagWrite takes the stamp as a
// parameter so the populate logic stays free of a clock read (and a test pins it).
func nowUnix() float64 {
	return float64(time.Now().Unix())
}
