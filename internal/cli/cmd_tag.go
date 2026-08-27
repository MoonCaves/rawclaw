package cli

import (
	"context"
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

// chunkByteCap is the total size ceiling (in bytes) for a single tag-prep pass.
// Sessions exceeding this budget are processed incrementally in chunked passes,
// keeping each pass in the reliably-attended context band (~15k tokens / 60KB).
// var, not const, so a test can shrink it to force chunking without huge sessions.
var chunkByteCap = 60_000

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

// untaggedWindow describes the earliest contiguous untagged stretch of displayable messages.
type untaggedWindow struct {
	start        int
	end          int
	moreUntagged bool
	fullyTagged  bool
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

// runTagPrep is the testable core: load a session's messages, find the earliest
// contiguous untagged window capped at chunkByteCap, and print the condensed dump.
func runTagPrep(w io.Writer, con *sql.DB, fullSID string) error {
	existingSegs, err := store.TopicsForSession(con, fullSID)
	if err != nil {
		return fmt.Errorf("load existing topics: %w", err)
	}
	return runTagPrepWithTopics(w, con, fullSID, existingSegs)
}

// runTagPrepWithTopics dumps a session's messages from con, using the provided
// existing topic segments to compute the untagged window.
func runTagPrepWithTopics(w io.Writer, con *sql.DB, fullSID string, existingSegs []store.TopicSegment) error {
	msgs, err := loadSessionMessages(con, fullSID)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return fmt.Errorf("session %q has no messages to tag", lastSlice8(fullSID))
	}

	var displayable []store.SessionMessage
	for _, m := range msgs {
		if view.IsDisplayable(m.Content) {
			displayable = append(displayable, m)
		}
	}
	if len(displayable) == 0 {
		return fmt.Errorf("session %q has no messages to tag", lastSlice8(fullSID))
	}

	win, tagged := computeUntaggedWindow(displayable, msgs, existingSegs, chunkByteCap)
	if win.fullyTagged {
		fmt.Fprintf(w, "# session %s is already fully tagged\n", lastSlice8(fullSID))
		return nil
	}

	fmt.Fprintf(w, "# condensed session %s — one line per message: <uuid8> [<role>] <text>\n",
		lastSlice8(fullSID))
	fmt.Fprintf(w, "# split into contiguous TOPIC segments; feed back via: rawclaw tag-write %s\n",
		lastSlice8(fullSID))

	if win.start > 0 && tagged[win.start-1] {
		if prev := findPrevSegment(existingSegs, displayable, msgs, win.start-1); prev != nil {
			endRef := uuid8(prev.EndUUID)
			if endRef == "" {
				endRef = uuid8(prev.StartUUID)
			}
			fmt.Fprintf(w, "# previous topic: %q (ended at %s)\n", prev.Topic, endRef)
		}
	}

	for i := win.start; i <= win.end; i++ {
		fmt.Fprintln(w, condensedLine(displayable[i]))
	}

	if win.moreUntagged {
		fmt.Fprintf(w, "# untagged content remains beyond budget; rerun 'rawclaw tag-prep %s' after writing\n",
			lastSlice8(fullSID))
	}
	return nil
}

// computeUntaggedWindow finds the EARLIEST contiguous untagged stretch in
// displayable messages, capped at byteCap.
func computeUntaggedWindow(
	displayable []store.SessionMessage,
	msgs []store.SessionMessage,
	existingSegs []store.TopicSegment,
	byteCap int,
) (untaggedWindow, []bool) {
	if len(displayable) == 0 {
		return untaggedWindow{fullyTagged: true}, nil
	}

	uuidToDispIdx := make(map[string]int, len(displayable))
	for i, m := range displayable {
		uuidToDispIdx[m.UUID] = i
	}
	uuidToMsgID := make(map[string]int, len(msgs))
	for _, m := range msgs {
		uuidToMsgID[m.UUID] = m.ID
	}

	tagged := make([]bool, len(displayable))
	for _, seg := range existingSegs {
		st, stOK := uuidToDispIdx[seg.StartUUID]
		if !stOK {
			if id, hasID := uuidToMsgID[seg.StartUUID]; hasID {
				for i, dm := range displayable {
					if dm.ID >= id {
						st = i
						stOK = true
						break
					}
				}
			}
		}

		endUUID := seg.EndUUID
		if endUUID == "" {
			endUUID = seg.StartUUID
		}
		end, endOK := uuidToDispIdx[endUUID]
		if !endOK {
			if id, hasID := uuidToMsgID[endUUID]; hasID {
				for i := len(displayable) - 1; i >= 0; i-- {
					if displayable[i].ID <= id {
						end = i
						endOK = true
						break
					}
				}
			}
		}

		if stOK && endOK && st <= end && st < len(displayable) && end >= 0 {
			if st < 0 {
				st = 0
			}
			if end >= len(displayable) {
				end = len(displayable) - 1
			}
			for k := st; k <= end; k++ {
				tagged[k] = true
			}
		}
	}

	stretchStart := -1
	for i := range displayable {
		if !tagged[i] {
			stretchStart = i
			break
		}
	}
	if stretchStart < 0 {
		return untaggedWindow{fullyTagged: true}, tagged
	}

	stretchEnd := stretchStart
	for stretchEnd < len(displayable) && !tagged[stretchEnd] {
		stretchEnd++
	}
	stretchEnd--

	winStart := stretchStart
	winEnd := stretchStart
	totalBytes := 0
	moreUntagged := false

	for i := stretchStart; i <= stretchEnd; i++ {
		line := condensedLine(displayable[i])
		lineBytes := len(line) + 1
		if i > stretchStart && totalBytes+lineBytes > byteCap {
			moreUntagged = true
			break
		}
		winEnd = i
		totalBytes += lineBytes
	}

	if !moreUntagged {
		for i := winEnd + 1; i < len(displayable); i++ {
			if !tagged[i] {
				moreUntagged = true
				break
			}
		}
	}

	return untaggedWindow{
		start:        winStart,
		end:          winEnd,
		moreUntagged: moreUntagged,
		fullyTagged:  false,
	}, tagged
}

func findPrevSegment(
	existingSegs []store.TopicSegment,
	displayable []store.SessionMessage,
	msgs []store.SessionMessage,
	targetIdx int,
) *store.TopicSegment {
	uuidToDispIdx := make(map[string]int, len(displayable))
	for i, m := range displayable {
		uuidToDispIdx[m.UUID] = i
	}
	uuidToMsgID := make(map[string]int, len(msgs))
	for _, m := range msgs {
		uuidToMsgID[m.UUID] = m.ID
	}
	var bestSeg *store.TopicSegment
	bestStart := -1
	for i := range existingSegs {
		seg := &existingSegs[i]
		st, stOK := uuidToDispIdx[seg.StartUUID]
		if !stOK {
			if id, hasID := uuidToMsgID[seg.StartUUID]; hasID {
				for j, dm := range displayable {
					if dm.ID >= id {
						st = j
						stOK = true
						break
					}
				}
			}
		}
		endUUID := seg.EndUUID
		if endUUID == "" {
			endUUID = seg.StartUUID
		}
		end, endOK := uuidToDispIdx[endUUID]
		if !endOK {
			if id, hasID := uuidToMsgID[endUUID]; hasID {
				for j := len(displayable) - 1; j >= 0; j-- {
					if displayable[j].ID <= id {
						end = j
						endOK = true
						break
					}
				}
			}
		}
		if stOK && endOK && st <= targetIdx && targetIdx <= end {
			if st > bestStart {
				bestStart = st
				bestSeg = seg
			}
		}
	}
	return bestSeg
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
		retagAll    bool
	)
	cmd := &cobra.Command{
		Use:     "tag-write <session8>",
		Aliases: []string{"tag-floor"},
		Short:   "Write a tagging subagent's topic segments (JSON on STDIN) or routine verdict to the index",
		Long: "Read a JSON array of topic segments from STDIN and store them in the topic index used by " +
			"search/outline. Each segment: {\"start_uuid\":\"<uuid8 prefix>\",\"topic\":\"...\",\"summary\":\"...\"}, " +
			"in session order. start_uuid is prefix-resolved against the session's message uuids; each segment's " +
			"end is the message just before the next segment's start (the last message for the final segment).\n\n" +
			"Pass --retag-all to replace all existing topic segments for the session rather than inserting into " +
			"the earliest untagged window.\n\n" +
			"Pass --routine to mark the session with a routine verdict (trivial / low-signal; sorts down, never hidden). " +
			"Use the tag-floor alias with no session argument to sweep the consolidated corpus using the " +
			"deterministic math floor; it makes no LLM or API calls. rawclaw calls NO LLM — a tagging subagent " +
			"decides the segments or routine verdict and pipes/flags them here.",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.CalledAs() == "tag-floor" {
				return cobra.NoArgs(cmd, args)
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
			return runTagWriteCmd(cmd.OutOrStdout(), cmd.InOrStdin(), args[0], scope, more, routine, source, retagAll)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	f.BoolVar(&retagAll, "retag-all", false, "replace all topic segments for the session (re-tagging pass)")
	f.BoolVar(&routine, "routine", false, "mark the session with a routine verdict (trivial / low-signal; sorts down)")
	f.StringVar(&source, "source", store.VerdictSourceAgent, "verdict source (agent|floor)")
	return cmd
}

// runTagFloorCmd evaluates every session in the consolidated store. The
// consolidated messages are already parser output, so the floor uses the
// parser's substantive-human filter instead of the inflated sessions count.
// A per-session read error is unknown and is never written as routine.
func runTagFloorCmd(w io.Writer, scope []view.Scope) error {
	// This writes directly to the consolidated store, not through a
	// per-project fold — the ONE path in this file that touches
	// consolidated.db without going through SyncConsolidatedFrom (which
	// fences itself). A concurrent rebuild snapshots the live store, builds
	// a replacement beside it, and renames over it; an unfenced write here
	// can land on the live file just before that rename and be silently
	// discarded. AcquireConsolidatedFence is the same fence rebuild takes.
	fence, err := index.AcquireConsolidatedFence(context.Background())
	if err != nil {
		return fmt.Errorf("acquire consolidated store lock: %w", err)
	}
	defer func() { _ = fence.Close() }()

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
func runTagWriteCmd(w io.Writer, r io.Reader, session8 string, scope []view.Scope, more agentproto.ScopeFn, routine bool, source string, retagAll bool) error {
	dbp, fullSID, err := agentproto.LocateSessionGuarded(session8, scope, more)
	if err != nil {
		return err
	}

	// A session with no surviving per-project source (retained-but-purged,
	// or #22's carried-forward store-only history) resolves dbp to the
	// consolidated store itself — this write then lands directly on
	// consolidated.db with no per-project fold afterward to fence it. Same
	// hazard as runTagFloorCmd: fence only for that case, so the common
	// per-project write path pays no extra lock contention.
	var fence *index.ConsolidatedFence
	if dbp == index.ConsolidatedPath() {
		fence, err = index.AcquireConsolidatedFence(context.Background())
		if err != nil {
			return fmt.Errorf("acquire consolidated store lock: %w", err)
		}
		defer func() { _ = fence.Close() }()
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
		n, writeErr = runTagWrite(con, fullSID, r, nowUnix(), retagAll)
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
// and taggedAt timestamp. Existing topic segments are retained because a real
// topic segment demotes routine at read time.
func runTagWriteRoutine(con *sql.DB, fullSID, source string, taggedAt float64) error {
	if err := store.EnsureTopicSchema(con); err != nil {
		return fmt.Errorf("ensure topic schema: %w", err)
	}
	if source == "" {
		source = store.VerdictSourceAgent
	}
	if source == store.VerdictSourceFloor {
		var ok bool
		var err error
		source, ok, err = verdictSource(con, fullSID)
		if err != nil {
			return fmt.Errorf("read existing verdict: %w", err)
		}
		if !ok {
			source = store.VerdictSourceFloor
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
	return nil
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
// for the final segment), validate that all segments lie within the declared
// untagged window [winStart, winEnd], and write segments to topic_segment.
// Default write is INSERT-ONLY; with retagAll=true it replaces all segments.
// Returns the number of rows written. taggedAt is passed in so the populate
// logic stays free of a clock read.
func runTagWrite(con *sql.DB, fullSID string, r io.Reader, taggedAt float64, retagAll bool) (int, error) {
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

	var displayable []store.SessionMessage
	for _, m := range msgs {
		if view.IsDisplayable(m.Content) {
			displayable = append(displayable, m)
		}
	}
	if len(displayable) == 0 {
		return 0, fmt.Errorf("session %q has no messages to tag", lastSlice8(fullSID))
	}

	var winStart, winEnd int
	if retagAll {
		winStart = 0
		winEnd = len(displayable) - 1
	} else {
		existingSegs, err := store.TopicsForSession(con, fullSID)
		if err != nil {
			return 0, fmt.Errorf("load existing topics: %w", err)
		}
		win, _ := computeUntaggedWindow(displayable, msgs, existingSegs, chunkByteCap)
		if win.fullyTagged {
			return 0, fmt.Errorf("session %q is already fully tagged (use --retag-all to re-tag)", lastSlice8(fullSID))
		}
		winStart = win.start
		winEnd = win.end
	}

	var segs []rawSegment
	dec := json.NewDecoder(r)
	if err := dec.Decode(&segs); err != nil {
		return 0, fmt.Errorf("decode tag-write JSON (want an array of {start_uuid,topic,summary}): %w", err)
	}
	if len(segs) == 0 {
		return 0, fmt.Errorf("tag-write got an empty segment array — nothing to write")
	}

	startIndices := make([]int, len(segs))
	for i, seg := range segs {
		if strings.TrimSpace(seg.StartUUID) == "" {
			return 0, fmt.Errorf("segment %d: missing start_uuid", i)
		}
		if strings.TrimSpace(seg.Topic) == "" {
			return 0, fmt.Errorf("segment %d (start_uuid %q): missing topic", i, seg.StartUUID)
		}
		matchIdx := -1
		for j, dm := range displayable {
			if strings.HasPrefix(dm.UUID, seg.StartUUID) {
				if matchIdx >= 0 {
					return 0, fmt.Errorf("start_uuid %q is ambiguous (matches multiple messages)", seg.StartUUID)
				}
				matchIdx = j
			}
		}
		if matchIdx < 0 {
			return 0, fmt.Errorf("start_uuid %q matches no message in this session", seg.StartUUID)
		}
		if matchIdx < winStart || matchIdx > winEnd {
			return 0, fmt.Errorf("segment %d start_uuid %q (msg %s) is outside window [%s..%s]",
				i, seg.StartUUID, uuid8(displayable[matchIdx].UUID),
				uuid8(displayable[winStart].UUID), uuid8(displayable[winEnd].UUID))
		}
		if i > 0 && matchIdx <= startIndices[i-1] {
			return 0, fmt.Errorf("segment %d start is not after segment %d", i, i-1)
		}
		startIndices[i] = matchIdx
	}

	out := make([]store.TopicSegment, len(segs))
	for i, seg := range segs {
		st := startIndices[i]
		end := winEnd
		if i+1 < len(segs) {
			end = startIndices[i+1] - 1
		}
		out[i] = store.TopicSegment{
			SessionID: fullSID,
			StartUUID: displayable[st].UUID,
			EndUUID:   displayable[end].UUID,
			Topic:     seg.Topic,
			Summary:   seg.Summary,
			TaggedAt:  taggedAt,
		}
	}

	if retagAll {
		if err := store.ReplaceSessionSegments(con, fullSID, out); err != nil {
			return 0, err
		}
	} else {
		if err := store.InsertTopicSegments(con, out); err != nil {
			return 0, err
		}
	}
	return len(out), nil
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

// nowUnix is the CLI-runtime wall-clock stamp for tagged_at (seconds since the
// epoch). It lives at the command edge — runTagWrite takes the stamp as a
// parameter so the populate logic stays free of a clock read (and a test pins it).
func nowUnix() float64 {
	return float64(time.Now().Unix())
}
