package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/tagger"
	"github.com/MoonCaves/rawclaw/internal/view"
	"github.com/spf13/cobra"
)

// condenseCap caps each message's collapsed one-line content (in runes) in the
// condensed view fed to the tagger — enough to name the topic, not the full text.
const condenseCap = 200

// windowChars is the soft ceiling on a single condensed window's size (chars). A
// session whose condensed view exceeds this is split into sequential windows,
// each tagged separately; the returned start_ids are real message ids, so the
// segments from every window remain valid and just concatenate.
const windowChars = 120_000

// tagMsg is one message row loaded for tagging: id + uuid + role + content, in
// session order (id ascending).
type tagMsg struct {
	ID      int
	UUID    string
	Role    string
	Content string
}

// newTagCmd wires `rawclaw tag <session8>`: tag a session's messages into topic
// segments via the configured chat endpoint, populating the topic_segment table.
// Thin wrapper — resolution + open + a now() stamp, then runTag does the work.
func newTagCmd() *cobra.Command {
	var (
		thisProject bool
		dir         string
	)
	cmd := &cobra.Command{
		Use:   "tag <session8>",
		Short: "Tag a session's topics (needs RAWCLAW_TAG_ENDPOINT)",
		Long: "Label WHERE topics were discussed in a session, populating the topic index used by search/outline.\n\n" +
			"Tagging is opt-in: set RAWCLAW_TAG_ENDPOINT (an OpenAI-compatible chat URL, e.g. a LiteLLM " +
			"/v1/chat/completions) and RAWCLAW_TAG_MODEL. Takes the 8-char session id from a search hit.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			tg := tagger.GetTagger()
			if tg == nil {
				return fmt.Errorf("tagging disabled — set RAWCLAW_TAG_ENDPOINT (OpenAI-compatible chat URL) and RAWCLAW_TAG_MODEL")
			}
			return runTagCmd(cmd.OutOrStdout(), args[0], verbScope(thisProject, dir), tg)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	return cmd
}

// runTagCmd resolves session8 → db + full id, opens the db read-write, ensures
// the topic schema, and runs the populate pass with a now() timestamp. Kept thin
// so the testable core (runTag) takes an already-open db + resolved session id.
func runTagCmd(w io.Writer, session8 string, scope []view.Scope, tg tagger.Tagger) error {
	dbp, fullSID, err := agentproto.LocateSession(session8, scope)
	if err != nil {
		return err
	}

	con, err := openRW(dbp)
	if err != nil {
		return fmt.Errorf("open %q read-write: %w", dbp, err)
	}
	defer con.Close()

	n, err := runTag(con, fullSID, tg, nowUnix())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Tagged %s — %d topic segment(s).\n", lastSlice8(fullSID), n)
	return nil
}

// runTag is the testable core: load a session's messages, build the condensed
// view, tag it (windowing if needed), and upsert one topic_segment row per
// returned segment. Returns the number of segments written. taggedAt is passed in
// (the CLI uses time.Now; a test passes a fixed stamp) — this keeps the populate
// logic free of a wall-clock read.
func runTag(con *sql.DB, fullSID string, tg tagger.Tagger, taggedAt float64) (int, error) {
	if err := index.EnsureTopicSchema(con); err != nil {
		return 0, fmt.Errorf("ensure topic schema: %w", err)
	}

	msgs, err := loadSessionMessages(con, fullSID)
	if err != nil {
		return 0, err
	}
	if len(msgs) == 0 {
		return 0, fmt.Errorf("session %q has no messages to tag", lastSlice8(fullSID))
	}

	segs, err := tagWindows(tg, msgs)
	if err != nil {
		return 0, err
	}

	return writeSegments(con, fullSID, msgs, segs, taggedAt)
}

// loadSessionMessages reads a session's messages in id order (id ascending) — the
// chronological spine the tagger and the segment-range mapping both walk.
func loadSessionMessages(con *sql.DB, fullSID string) ([]tagMsg, error) {
	rows, err := con.Query(
		"SELECT id, uuid, role, content FROM messages WHERE session_id=? ORDER BY id",
		fullSID)
	if err != nil {
		return nil, fmt.Errorf("load session messages: %w", err)
	}
	defer rows.Close()

	var out []tagMsg
	for rows.Next() {
		var m tagMsg
		if err := rows.Scan(&m.ID, &m.UUID, &m.Role, &m.Content); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// condensedLine renders one message as `[#<id> <role>] <text>`, collapsing the
// content to a single capped line via parse.Disp (tools stripped).
func condensedLine(m tagMsg) string {
	text := parse.Disp(m.Content, false, condenseCap)
	return fmt.Sprintf("[#%d %s] %s", m.ID, m.Role, text)
}

// tagWindows builds the condensed view and tags it. When the joined view exceeds
// windowChars it is split into sequential windows (whole messages only) and each
// is tagged separately; every window's start_ids are real message ids, so the
// returned segments stay valid and simply concatenate in session order.
func tagWindows(tg tagger.Tagger, msgs []tagMsg) ([]tagger.Segment, error) {
	var all []tagger.Segment
	var b strings.Builder

	flush := func() error {
		if b.Len() == 0 {
			return nil
		}
		segs, err := tg.TagSession(strings.TrimRight(b.String(), "\n"))
		if err != nil {
			return err
		}
		all = append(all, segs...)
		b.Reset()
		return nil
	}

	for _, m := range msgs {
		line := condensedLine(m)
		// Start a new window before this line would push the current one over the
		// ceiling (but never split below one message — a lone oversized message
		// still goes in its own window).
		if b.Len() > 0 && b.Len()+len(line)+1 > windowChars {
			if err := flush(); err != nil {
				return nil, err
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return all, nil
}

// writeSegments maps each tagger Segment to a topic_segment row and upserts it.
// start_id resolves to its message's uuid; end_uuid is the uuid of the message
// just BEFORE the next segment's start_id (the session's last message for the
// final segment). A segment whose start_id matches no loaded message is skipped
// (the model occasionally invents an id). Returns the number of rows written.
func writeSegments(con *sql.DB, fullSID string, msgs []tagMsg, segs []tagger.Segment, taggedAt float64) (int, error) {
	if len(msgs) == 0 {
		return 0, nil
	}
	// id → message index, for resolving a segment's start_id to its uuid and for
	// finding "the message just before the next segment's start_id".
	idIndex := make(map[int]int, len(msgs))
	for i, m := range msgs {
		idIndex[m.ID] = i
	}
	lastUUID := msgs[len(msgs)-1].UUID

	written := 0
	for i, seg := range segs {
		startIdx, ok := idIndex[seg.StartID]
		if !ok {
			continue // model invented a start_id with no backing message — skip it
		}
		startUUID := msgs[startIdx].UUID

		endUUID := lastUUID
		// end_uuid = the uuid of the message just before the NEXT segment's start.
		if nextIdx, found := nextStartIndex(idIndex, segs, i); found {
			if nextIdx > 0 {
				endUUID = msgs[nextIdx-1].UUID
			} else {
				endUUID = msgs[nextIdx].UUID
			}
		}

		if err := index.UpsertTopicSegment(con, fullSID, startUUID, endUUID, seg.Topic, seg.Summary, taggedAt); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// nowUnix is the CLI-runtime wall-clock stamp for tagged_at (seconds since the
// epoch). It lives at the command edge — runTag takes the stamp as a parameter so
// the populate logic stays free of a clock read (and a test pins it).
func nowUnix() float64 {
	return float64(time.Now().Unix())
}

// nextStartIndex finds the message index of the NEXT segment (after i) whose
// start_id resolves to a real message, so a skipped/invalid intervening segment
// doesn't break the end-boundary computation. Returns (index, true) when found.
func nextStartIndex(idIndex map[int]int, segs []tagger.Segment, i int) (int, bool) {
	for j := i + 1; j < len(segs); j++ {
		if idx, ok := idIndex[segs[j].StartID]; ok {
			return idx, true
		}
	}
	return 0, false
}
