package codex

import (
	"bufio"
	"encoding/json"
	"os"
)

// meta is the lineage + cwd read from a rollout's own session_meta header (the
// file's first session_meta record — its OWN id, not a replayed parent's).
type meta struct {
	id             string
	cwd            string
	threadSource   string // "user" for a root session; "subagent" etc. for a child
	parentThreadID string // "" when absent/null
	forkedFromID   string // "" when absent/null
}

// isChild reports whether this thread is a subagent or fork (anything that isn't
// a root user session). Such threads are indexed is_subagent=1 and hidden from
// default search, mirroring Claude subagents.
func (m meta) isChild() bool {
	return (m.threadSource != "" && m.threadSource != "user") ||
		m.forkedFromID != "" ||
		m.parentThreadID != ""
}

// parent returns the parent session id for lineage collapse: the spawning
// parent_thread_id if present, else the forked_from_id.
func (m meta) parent() string {
	if m.parentThreadID != "" {
		return m.parentThreadID
	}
	return m.forkedFromID
}

// readMeta returns the first session_meta header in path. It scans a small
// prefix (headers sit at the very top) and takes the FIRST session_meta — the
// file's own — never a replayed parent's that may follow. ok=false when no
// usable header with a non-empty id is found.
func readMeta(path string) (meta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return meta{}, false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // headers embed the full system prompt
	const scanLimit = 8
	for i := 0; sc.Scan() && i < scanLimit; i++ {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			continue
		}
		if t, _ := rec["type"].(string); t != "session_meta" {
			continue
		}
		p, ok := rec["payload"].(map[string]any)
		if !ok {
			continue
		}
		m := meta{}
		m.id, _ = p["id"].(string)
		if m.id == "" {
			return meta{}, false
		}
		m.cwd, _ = p["cwd"].(string)
		m.threadSource, _ = p["thread_source"].(string)
		m.parentThreadID, _ = p["parent_thread_id"].(string) // null -> ""
		m.forkedFromID, _ = p["forked_from_id"].(string)     // null -> ""
		return m, true
	}
	return meta{}, false
}
