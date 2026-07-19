package live

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
)

// DefaultSessionTail is how many trailing messages a session render shows when
// the caller passes no tail — enough to see what the agent is doing right now
// without replaying the whole session.
const DefaultSessionTail = 40

// sessionDispCap caps one rendered message. Live peek is a glance, not a read;
// `rawclaw read` on the archive copy is the deep-dig path.
const sessionDispCap = 2000

// sessionJSON is the --json shape of one rendered session: identity header +
// the message tail.
type sessionJSON struct {
	SessionID     string        `json:"session_id"`
	Source        string        `json:"source"`
	Project       string        `json:"project"`
	CWD           string        `json:"cwd,omitempty"`
	LastActivity  string        `json:"last_activity"`
	AgeSeconds    int64         `json:"age_seconds"`
	TotalMessages int           `json:"total_messages"`
	Messages      []messageJSON `json:"messages"`
}

type messageJSON struct {
	Role  string `json:"role"`
	Text  string `json:"text"`
	TSISO string `json:"ts_iso,omitempty"`
	UUID  string `json:"uuid,omitempty"`
}

// ServeSession renders the current transcript of the top-level session whose
// id starts with prefix: a one-shot read of the live file, so messages written
// seconds ago are included. tail caps the rendered messages (<=0 = default);
// includeTools opts the human render into tool calls (stripped by default,
// matching the display posture of every other surface); jsonOut switches to
// the machine shape, which always carries the raw text. An unmatched or
// ambiguous prefix is a distinct, actionable error.
func ServeSession(w io.Writer, prefix string, tail int, includeTools, jsonOut bool) error {
	if tail <= 0 {
		tail = DefaultSessionTail
	}

	reg, c, err := resolvePrefix(prefix)
	if err != nil {
		return err
	}

	msgs, err := reg.New().Messages(c)
	if err != nil {
		return fmt.Errorf("read session %s: %w", c.ID, err)
	}
	total := len(msgs)
	if len(msgs) > tail {
		msgs = msgs[len(msgs)-tail:]
	}

	mtime, age := fileActivity(c.Path)
	if jsonOut {
		out := sessionJSON{
			SessionID:     c.ID,
			Source:        reg.ID,
			Project:       projectName(c.CWD),
			CWD:           c.CWD,
			LastActivity:  mtime,
			AgeSeconds:    age,
			TotalMessages: total,
			Messages:      make([]messageJSON, 0, len(msgs)),
		}
		for _, m := range msgs {
			out.Messages = append(out.Messages, messageJSON{
				Role:  m.Role,
				Text:  m.Text,
				TSISO: m.TSISO,
				UUID:  m.UUID,
			})
		}
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return fmt.Errorf("emit session json: %w", err)
		}
		return nil
	}

	fmt.Fprintf(w, "session %s · %s · %s · last activity %s\n\n",
		c.ID, orUnknown(projectName(c.CWD)), reg.ID, FormatAge(age))
	hidden := 0
	for _, m := range msgs {
		text := parse.Disp(m.Text, includeTools, sessionDispCap)
		if text == "" {
			// A turn that is nothing but tool calls strips to empty. Skip it —
			// the same empty-turn posture read takes — and count it, so a
			// tool-heavy tail reads as a working agent, not blank lines.
			if strings.TrimSpace(m.Text) != "" {
				hidden++
			}
			continue
		}
		fmt.Fprintf(w, "[%s %s] %s\n", clockOf(m), m.Role, text)
	}
	if hidden > 0 {
		noun := "turns"
		if hidden == 1 {
			noun = "turn"
		}
		fmt.Fprintf(w, "\n(%d tool-only %s hidden — --include-tools renders them)\n", hidden, noun)
	}
	if total > len(msgs) {
		fmt.Fprintf(w, "\n(showing the last %d of %d messages)\n", len(msgs), total)
	}
	return nil
}

// ambiguousListCap bounds how many candidate ids an ambiguous-prefix error
// lists — it is a message, not a dump.
const ambiguousListCap = 10

// resolvePrefix finds the single top-level session whose id starts with
// prefix, across every source. An empty prefix, zero matches, and multiple
// matches are distinct errors — the ambiguous one lists (a capped number of)
// candidates so the caller can narrow.
func resolvePrefix(prefix string) (source.Registration, source.Container, error) {
	if prefix == "" {
		return source.Registration{}, source.Container{}, fmt.Errorf(
			"session prefix is empty — drop it to list recent sessions instead")
	}
	type match struct {
		reg source.Registration
		c   source.Container
	}
	var matches []match
	for _, reg := range sources() {
		containers, err := reg.New().Discover()
		if err != nil {
			continue // an unreadable source can't hold the match; others still can
		}
		for _, c := range containers {
			if c.IsSubagent {
				continue
			}
			if strings.HasPrefix(c.ID, prefix) {
				matches = append(matches, match{reg, c})
			}
		}
	}
	switch len(matches) {
	case 0:
		return source.Registration{}, source.Container{}, fmt.Errorf(
			"no session on this machine matches %q — drop the prefix to list recent sessions", prefix)
	case 1:
		return matches[0].reg, matches[0].c, nil
	default:
		shown := len(matches)
		if shown > ambiguousListCap {
			shown = ambiguousListCap
		}
		ids := make([]string, 0, shown+1)
		for _, m := range matches[:shown] {
			ids = append(ids, m.c.ID)
		}
		if rest := len(matches) - shown; rest > 0 {
			ids = append(ids, fmt.Sprintf("… and %d more", rest))
		}
		return source.Registration{}, source.Container{}, fmt.Errorf(
			"%d sessions match %q — narrow it:\n  %s", len(matches), prefix, strings.Join(ids, "\n  "))
	}
}

// fileActivity returns (marked-UTC RFC3339 mtime, age seconds) for path,
// zero-valued when the file cannot be stat'd (it may have vanished between
// resolve and render). Rendering goes through the timefmt seam: live output is
// an agent-parsed surface, so the instant carries the explicit Z marker.
func fileActivity(path string) (string, int64) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0
	}
	age := int64(time.Since(info.ModTime()).Seconds())
	if age < 0 {
		age = 0
	}
	return timefmt.UTC(info.ModTime()), age
}

// clockOf renders a message's wall-clock (marked UTC, "HH:MM:SSZ" via the
// timefmt seam), "?" when the record carries no timestamp.
func clockOf(m model.Message) string {
	if m.TS <= 0 {
		return "?"
	}
	return timefmt.UTCClock(time.Unix(int64(m.TS), 0))
}

// FormatAge renders seconds as a compact "how long ago": 42s, 5m, 3h, 2d.
// Exported because the client half renders remote-computed ages with it.
func FormatAge(secs int64) string {
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds ago", secs)
	case secs < 60*60:
		return fmt.Sprintf("%dm ago", secs/60)
	case secs < 24*60*60:
		return fmt.Sprintf("%dh ago", secs/(60*60))
	default:
		return fmt.Sprintf("%dd ago", secs/(24*60*60))
	}
}

// orUnknown maps "" to "?" for display.
func orUnknown(s string) string {
	if s == "" {
		return "?"
	}
	return s
}
