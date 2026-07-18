package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
	"github.com/spf13/cobra"
)

// condenseCap caps each message's collapsed one-line content (in runes) in the
// condensed view dumped for a tagging subagent — enough to name the topic, not
// the full text.
const condenseCap = 200

// uuid8Len is the prefix length of a message uuid printed by tag-prep and
// resolved by tag-write — mirrors the <uuid8> refs the search/read path uses.
const uuid8Len = 8

// tagMsg is one message row loaded for tagging: id + uuid + role + content, in
// session order (id ascending).
type tagMsg struct {
	ID      int
	UUID    string
	Role    string
	Content string
}

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
			return runTagPrepCmd(cmd.OutOrStdout(), args[0], verbScope(thisProject, dir))
		},
	}
	f := cmd.Flags()
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	return cmd
}

// runTagPrepCmd resolves session8 → db + full id, opens the db read-only, loads
// the session's messages, and prints the condensed dump. Thin wrapper around the
// testable runTagPrep core.
func runTagPrepCmd(w io.Writer, session8 string, scope []view.Scope) error {
	con, fullSID, err := openSessionRO(session8, scope)
	if err != nil {
		return err
	}
	defer con.Close()
	return runTagPrep(w, con, fullSID)
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

	fmt.Fprintf(w, "# condensed session %s — one line per message: <uuid8> [<role>] <text>\n",
		lastSlice8(fullSID))
	fmt.Fprintf(w, "# split into contiguous TOPIC segments; feed back via: rawclaw tag-write %s\n",
		lastSlice8(fullSID))
	for _, m := range msgs {
		fmt.Fprintln(w, condensedLine(m))
	}
	return nil
}

// ── verb: tag-write ────────────────────────────────────────────────────────────

// newTagWriteCmd wires `rawclaw tag-write <session8>`: read a JSON array of
// topic segments from STDIN (as decided by a tagging subagent) and upsert them
// into the topic_segment index. rawclaw calls no LLM — this is the dumb write-back
// half of the prep/write pair.
func newTagWriteCmd() *cobra.Command {
	var (
		thisProject bool
		dir         string
	)
	cmd := &cobra.Command{
		Use:   "tag-write <session8>",
		Short: "Write a tagging subagent's topic segments (JSON on STDIN) to the index",
		Long: "Read a JSON array of topic segments from STDIN and store them in the topic index used by " +
			"search/outline. Each segment: {\"start_uuid\":\"<uuid8 prefix>\",\"topic\":\"...\",\"summary\":\"...\"}, " +
			"in session order. start_uuid is prefix-resolved against the session's message uuids; each segment's " +
			"end is the message just before the next segment's start (the last message for the final segment). " +
			"rawclaw calls NO LLM — a tagging subagent decides the segments (see `tag-prep`) and pipes them here.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagWriteCmd(cmd.OutOrStdout(), cmd.InOrStdin(), args[0], verbScope(thisProject, dir))
		},
	}
	f := cmd.Flags()
	f.BoolVar(&thisProject, "this-project", false, "limit to this project (default: all projects)")
	f.StringVar(&dir, "dir", cwd(), "project working dir for --this-project")
	return cmd
}

// runTagWriteCmd resolves session8 → db + full id, opens the db read-write,
// ensures the topic schema, and runs the populate pass reading JSON from r with a
// now() timestamp. Thin wrapper around the testable runTagWrite core.
func runTagWriteCmd(w io.Writer, r io.Reader, session8 string, scope []view.Scope) error {
	dbp, fullSID, err := agentproto.LocateSession(session8, scope)
	if err != nil {
		return err
	}

	con, err := openRW(dbp)
	if err != nil {
		return fmt.Errorf("open %q read-write: %w", dbp, err)
	}
	defer con.Close()

	n, err := runTagWrite(con, fullSID, r, nowUnix())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote %d topic segments for %s\n", n, lastSlice8(fullSID))
	return nil
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

	var segs []rawSegment
	dec := json.NewDecoder(r)
	if err := dec.Decode(&segs); err != nil {
		return 0, fmt.Errorf("decode tag-write JSON (want an array of {start_uuid,topic,summary}): %w", err)
	}
	if len(segs) == 0 {
		return 0, fmt.Errorf("tag-write got an empty segment array — nothing to write")
	}

	return writeSegments(con, fullSID, msgs, segs, taggedAt)
}

// ── shared helpers ────────────────────────────────────────────────────────────

// openSessionRO resolves session8 → db + full id and opens the db read-only —
// the load path for tag-prep (a pure dump, no writes).
func openSessionRO(session8 string, scope []view.Scope) (*sql.DB, string, error) {
	dbp, fullSID, err := agentproto.LocateSession(session8, scope)
	if err != nil {
		return nil, "", err
	}
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return nil, "", fmt.Errorf("open %q read-only: %w", dbp, err)
	}
	return con, fullSID, nil
}

// loadSessionMessages reads a session's messages in id order (id ascending) — the
// chronological spine the dump and the segment-range mapping both walk.
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

// condensedLine renders one message as `<uuid8> [<role>] <text>`, collapsing the
// content to a single capped line via parse.Disp (tools stripped). uuid8 is the
// ref a tagging subagent echoes back in tag-write's start_uuid.
func condensedLine(m tagMsg) string {
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

// writeSegments maps each subagent segment to a topic_segment row and upserts it.
// start_uuid is prefix-resolved to a message's full uuid; end_uuid is the full
// uuid of the message just BEFORE the next segment's start (the session's last
// message for the final segment). A segment missing start_uuid/topic, or whose
// start_uuid resolves to no/ambiguous message, returns a clear error. Returns the
// number of rows written.
func writeSegments(con *sql.DB, fullSID string, msgs []tagMsg, segs []rawSegment, taggedAt float64) (int, error) {
	if len(msgs) == 0 {
		return 0, nil
	}
	lastUUID := msgs[len(msgs)-1].UUID

	// First pass: validate each row and resolve its start_uuid → message index, so
	// the end-boundary computation can look at the next segment's resolved index.
	startIdx := make([]int, len(segs))
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
	}

	written := 0
	for i, seg := range segs {
		startUUID := msgs[startIdx[i]].UUID

		// end_uuid = the uuid of the message just before the NEXT segment's start.
		endUUID := lastUUID
		if i+1 < len(segs) {
			nextIdx := startIdx[i+1]
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

// resolveStartUUID resolves a uuid prefix against the session's message uuids,
// mirroring the read path's uuid8 resolution: exactly one match wins; zero
// matches and more-than-one match are both clear errors. Returns the matching
// message's index in msgs.
func resolveStartUUID(msgs []tagMsg, prefix string) (int, error) {
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
