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

// chunkByteCap is the total size ceiling (in bytes) for a single tag-prep pass.
// Sessions exceeding this budget are processed incrementally in chunked passes,
// keeping each pass in the reliably-attended context band (~15k tokens / 60KB).
// var, not const, so a test can shrink it to force chunking without huge sessions.
var chunkByteCap = 60_000

// dumpByteCap is retained for backward compatibility / tests that set it.
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

// messageRange represents a contiguous index range [start, end] (inclusive)
// in the displayable messages slice.
type messageRange struct {
	start int
	end   int
}

// tagChunk holds the untagged ranges and sized pass messages for a session.
type tagChunk struct {
	displayable  []store.SessionMessage
	msgs         []store.SessionMessage
	ranges       []messageRange
	prevSegs     map[int]*store.TopicSegment
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

// runTagPrep is the testable core: load a session's messages, compute untagged ranges,
// and print the condensed dump for the current pass.
func runTagPrep(w io.Writer, con *sql.DB, fullSID string) error {
	msgs, err := loadSessionMessages(con, fullSID)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return fmt.Errorf("session %q has no messages to tag", lastSlice8(fullSID))
	}

	existingSegs, err := store.TopicsForSession(con, fullSID)
	if err != nil {
		return fmt.Errorf("load existing topics: %w", err)
	}

	chunk := computeTagChunk(msgs, existingSegs, chunkByteCap)
	if len(chunk.displayable) == 0 {
		return fmt.Errorf("session %q has no messages to tag", lastSlice8(fullSID))
	}
	if chunk.fullyTagged {
		fmt.Fprintf(w, "# session %s is already fully tagged\n", lastSlice8(fullSID))
		return nil
	}

	fmt.Fprintf(w, "# condensed session %s — one line per message: <uuid8> [<role>] <text>\n",
		lastSlice8(fullSID))
	fmt.Fprintf(w, "# split into contiguous TOPIC segments; feed back via: rawclaw tag-write %s\n",
		lastSlice8(fullSID))

	for rIdx, r := range chunk.ranges {
		if prev, ok := chunk.prevSegs[rIdx]; ok && prev != nil {
			endRef := uuid8(prev.EndUUID)
			if endRef == "" {
				endRef = uuid8(prev.StartUUID)
			}
			fmt.Fprintf(w, "# previous topic: %q (ended at %s)\n", prev.Topic, endRef)
		}
		for i := r.start; i <= r.end; i++ {
			fmt.Fprintln(w, condensedLine(chunk.displayable[i]))
		}
	}

	if chunk.moreUntagged {
		fmt.Fprintf(w, "# untagged content remains beyond budget; rerun 'rawclaw tag %s' after writing\n",
			lastSlice8(fullSID))
	}
	return nil
}

// computeTagChunk calculates untagged ranges over displayable messages by
// inspecting existing topic_segment rows, and packs untagged messages into
// a pass capped at byteCap.
func computeTagChunk(msgs []store.SessionMessage, existingSegs []store.TopicSegment, byteCap int) tagChunk {
	var displayable []store.SessionMessage
	for _, m := range msgs {
		if view.IsDisplayable(m.Content) {
			displayable = append(displayable, m)
		}
	}
	if len(displayable) == 0 {
		return tagChunk{fullyTagged: true}
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
		startDispIdx, startOK := uuidToDispIdx[seg.StartUUID]
		if !startOK {
			if startID, hasID := uuidToMsgID[seg.StartUUID]; hasID {
				for i, dm := range displayable {
					if dm.ID >= startID {
						startDispIdx = i
						startOK = true
						break
					}
				}
			}
		}

		endUUID := seg.EndUUID
		if endUUID == "" {
			endUUID = seg.StartUUID
		}
		endDispIdx, endOK := uuidToDispIdx[endUUID]
		if !endOK {
			if endID, hasID := uuidToMsgID[endUUID]; hasID {
				for i := len(displayable) - 1; i >= 0; i-- {
					if displayable[i].ID <= endID {
						endDispIdx = i
						endOK = true
						break
					}
				}
			}
		}

		if startOK && endOK && startDispIdx <= endDispIdx && startDispIdx < len(displayable) && endDispIdx >= 0 {
			if startDispIdx < 0 {
				startDispIdx = 0
			}
			if endDispIdx >= len(displayable) {
				endDispIdx = len(displayable) - 1
			}
			for k := startDispIdx; k <= endDispIdx; k++ {
				tagged[k] = true
			}
		}
	}

	var allUntagged []messageRange
	for i := 0; i < len(displayable); {
		if !tagged[i] {
			start := i
			for i < len(displayable) && !tagged[i] {
				i++
			}
			allUntagged = append(allUntagged, messageRange{start: start, end: i - 1})
		} else {
			i++
		}
	}

	if len(allUntagged) == 0 {
		return tagChunk{
			displayable: displayable,
			fullyTagged: true,
		}
	}

	var (
		passMsgs     []store.SessionMessage
		passRanges   []messageRange
		prevSegs     = make(map[int]*store.TopicSegment)
		totalBytes   int
		moreUntagged bool
	)

	for _, uRange := range allUntagged {
		if moreUntagged {
			break
		}

		var currentSubRange *messageRange
		for msgIdx := uRange.start; msgIdx <= uRange.end; msgIdx++ {
			m := displayable[msgIdx]
			line := condensedLine(m)
			lineBytes := len(line) + 1

			if len(passMsgs) > 0 && totalBytes+lineBytes > byteCap {
				moreUntagged = true
				break
			}

			passMsgs = append(passMsgs, m)
			totalBytes += lineBytes

			if currentSubRange == nil {
				currentSubRange = &messageRange{start: msgIdx, end: msgIdx}
				pRangeIdx := len(passRanges)
				if msgIdx > 0 && tagged[msgIdx-1] {
					if prev := findSegmentCovering(existingSegs, uuidToDispIdx, uuidToMsgID, displayable, msgIdx-1); prev != nil {
						prevCopy := *prev
						prevSegs[pRangeIdx] = &prevCopy
					}
				}
			} else {
				currentSubRange.end = msgIdx
			}
		}

		if currentSubRange != nil {
			passRanges = append(passRanges, *currentSubRange)
		}
	}

	return tagChunk{
		displayable:  displayable,
		msgs:         passMsgs,
		ranges:       passRanges,
		prevSegs:     prevSegs,
		moreUntagged: moreUntagged,
		fullyTagged:  false,
	}
}

func findSegmentCovering(
	existingSegs []store.TopicSegment,
	uuidToDispIdx map[string]int,
	uuidToMsgID map[string]int,
	displayable []store.SessionMessage,
	targetDispIdx int,
) *store.TopicSegment {
	var bestSeg *store.TopicSegment
	bestStart := -1

	for i := range existingSegs {
		seg := &existingSegs[i]
		startDispIdx, startOK := uuidToDispIdx[seg.StartUUID]
		if !startOK {
			if startID, hasID := uuidToMsgID[seg.StartUUID]; hasID {
				for j, dm := range displayable {
					if dm.ID >= startID {
						startDispIdx = j
						startOK = true
						break
					}
				}
			}
		}

		endUUID := seg.EndUUID
		if endUUID == "" {
			endUUID = seg.StartUUID
		}
		endDispIdx, endOK := uuidToDispIdx[endUUID]
		if !endOK {
			if endID, hasID := uuidToMsgID[endUUID]; hasID {
				for j := len(displayable) - 1; j >= 0; j-- {
					if displayable[j].ID <= endID {
						endDispIdx = j
						endOK = true
						break
					}
				}
			}
		}

		if startOK && endOK && startDispIdx <= targetDispIdx && targetDispIdx <= endDispIdx {
			if startDispIdx > bestStart {
				bestStart = startDispIdx
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

	existingSegs, err := store.TopicsForSession(con, fullSID)
	if err != nil {
		return 0, fmt.Errorf("load existing topics: %w", err)
	}

	chunk := computeTagChunk(msgs, existingSegs, chunkByteCap)
	if len(chunk.displayable) == 0 {
		return 0, fmt.Errorf("session %q has no messages to tag", lastSlice8(fullSID))
	}
	if chunk.fullyTagged {
		// All messages are already tagged, so an explicit write is a re-tagging pass.
		chunk.msgs = chunk.displayable
		chunk.ranges = []messageRange{{start: 0, end: len(chunk.displayable) - 1}}
	}

	var segs []rawSegment
	dec := json.NewDecoder(r)
	if err := dec.Decode(&segs); err != nil {
		return 0, fmt.Errorf("decode tag-write JSON (want an array of {start_uuid,topic,summary}): %w", err)
	}
	if len(segs) == 0 {
		return 0, fmt.Errorf("tag-write got an empty segment array — nothing to write")
	}

	return writeSegments(con, fullSID, chunk, existingSegs, segs, taggedAt)
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

// writeSegments maps the subagent's segments to topic_segment rows, resolves
// start_uuid and end_uuid against the current pass chunk, clamps boundaries
// to the dumped ranges, and updates topic_segment rows: it deletes only prior
// segments overlapping the dumped ranges and inserts the new ones, preserving
// prior segments outside those ranges.
//
// Returns the number of rows written.
func writeSegments(con *sql.DB, fullSID string, chunk tagChunk, existingSegs []store.TopicSegment, segs []rawSegment, taggedAt float64) (int, error) {
	if len(chunk.msgs) == 0 {
		return 0, nil
	}

	startDispIdx := make([]int, len(segs))
	rangeCovered := make([]bool, len(chunk.ranges))

	for i, seg := range segs {
		if strings.TrimSpace(seg.StartUUID) == "" {
			return 0, fmt.Errorf("segment %d: missing start_uuid", i)
		}
		if strings.TrimSpace(seg.Topic) == "" {
			return 0, fmt.Errorf("segment %d (start_uuid %q): missing topic", i, seg.StartUUID)
		}
		dispIdx, err := resolveStartUUIDInChunk(chunk, seg.StartUUID)
		if err != nil {
			return 0, fmt.Errorf("segment %d: %w", i, err)
		}
		startDispIdx[i] = dispIdx
		for rIdx, r := range chunk.ranges {
			if dispIdx >= r.start && dispIdx <= r.end {
				rangeCovered[rIdx] = true
				break
			}
		}
	}

	if len(chunk.ranges) > 1 {
		for rIdx, covered := range rangeCovered {
			if !covered {
				return 0, fmt.Errorf("dump contained multiple ranges but range %d (%s..%s) has no segment starting in it — segments must cover ALL ranges shown in the dump",
					rIdx,
					uuid8(chunk.displayable[chunk.ranges[rIdx].start].UUID),
					uuid8(chunk.displayable[chunk.ranges[rIdx].end].UUID))
			}
		}
	}

	out := make([]store.TopicSegment, len(segs))
	for i, seg := range segs {
		st := startDispIdx[i]
		rEnd := chunk.ranges[len(chunk.ranges)-1].end
		for _, r := range chunk.ranges {
			if st >= r.start && st <= r.end {
				rEnd = r.end
				break
			}
		}

		endDisp := rEnd
		if i+1 < len(segs) {
			nextSt := startDispIdx[i+1]
			if nextSt > st && nextSt <= rEnd {
				endDisp = nextSt - 1
			}
		}

		out[i] = store.TopicSegment{
			SessionID: fullSID,
			StartUUID: chunk.displayable[st].UUID,
			EndUUID:   chunk.displayable[endDisp].UUID,
			Topic:     seg.Topic,
			Summary:   seg.Summary,
			TaggedAt:  taggedAt,
		}
	}

	uuidToDispIdx := make(map[string]int, len(chunk.displayable))
	for i, m := range chunk.displayable {
		uuidToDispIdx[m.UUID] = i
	}
	uuidToMsgID := make(map[string]int, len(chunk.displayable))
	for _, m := range chunk.displayable {
		uuidToMsgID[m.UUID] = m.ID
	}

	var deleteStartUUIDs []string
	for _, prior := range existingSegs {
		priorStart, startOK := uuidToDispIdx[prior.StartUUID]
		if !startOK {
			if startID, hasID := uuidToMsgID[prior.StartUUID]; hasID {
				for j, dm := range chunk.displayable {
					if dm.ID >= startID {
						priorStart = j
						startOK = true
						break
					}
				}
			}
		}
		endUUID := prior.EndUUID
		if endUUID == "" {
			endUUID = prior.StartUUID
		}
		priorEnd, endOK := uuidToDispIdx[endUUID]
		if !endOK {
			if endID, hasID := uuidToMsgID[endUUID]; hasID {
				for j := len(chunk.displayable) - 1; j >= 0; j-- {
					if chunk.displayable[j].ID <= endID {
						priorEnd = j
						endOK = true
						break
					}
				}
			}
		}

		if startOK && endOK && priorStart <= priorEnd {
			overlaps := false
			for _, r := range chunk.ranges {
				if !(priorEnd < r.start || priorStart > r.end) {
					overlaps = true
					break
				}
			}
			if overlaps {
				deleteStartUUIDs = append(deleteStartUUIDs, prior.StartUUID)
			}
		}
	}

	if err := store.ReplaceSessionRangeSegments(con, fullSID, deleteStartUUIDs, out); err != nil {
		return 0, err
	}
	return len(out), nil
}

// resolveStartUUIDInChunk resolves a uuid prefix against the current pass's
// message uuids.
func resolveStartUUIDInChunk(chunk tagChunk, prefix string) (int, error) {
	matchDispIdx := -1
	for _, m := range chunk.msgs {
		if strings.HasPrefix(m.UUID, prefix) {
			if matchDispIdx >= 0 {
				return 0, fmt.Errorf("start_uuid %q is ambiguous (matches multiple messages)", prefix)
			}
			for i, dm := range chunk.displayable {
				if dm.UUID == m.UUID {
					matchDispIdx = i
					break
				}
			}
		}
	}
	if matchDispIdx < 0 {
		return 0, fmt.Errorf("start_uuid %q matches no message in this session", prefix)
	}
	return matchDispIdx, nil
}

// nowUnix is the CLI-runtime wall-clock stamp for tagged_at (seconds since the
// epoch). It lives at the command edge — runTagWrite takes the stamp as a
// parameter so the populate logic stays free of a clock read (and a test pins it).
func nowUnix() float64 {
	return float64(time.Now().Unix())
}
