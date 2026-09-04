package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
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

type metaCacheEntry struct {
	mtime int64
	size  int64
	meta  meta
	ok    bool
}

var (
	metaCacheMu sync.RWMutex
	metaCache   = make(map[string]metaCacheEntry)
)

// readMeta returns the first session_meta header in path. It scans a small
// prefix (headers sit at the very top) and takes the FIRST session_meta — the
// file's own — never a replayed parent's that may follow. ok=false when no
// usable header with a non-empty id is found. Uses an mtime+size cache to avoid
// opening unchanged files.
func readMeta(path string) (meta, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return meta{}, false
	}

	metaCacheMu.RLock()
	if e, exists := metaCache[path]; exists && e.mtime == fi.ModTime().UnixNano() && e.size == fi.Size() {
		metaCacheMu.RUnlock()
		return e.meta, e.ok
	}
	metaCacheMu.RUnlock()

	f, err := os.Open(path)
	if err != nil {
		return meta{}, false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	sc.Buffer(buf, 1024*1024)
	const scanLimit = 8
	var found meta
	var ok bool
	for i := 0; sc.Scan() && i < scanLimit; i++ {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			continue
		}
		if t, _ := rec["type"].(string); t != "session_meta" {
			continue
		}
		p, pok := rec["payload"].(map[string]any)
		if !pok {
			continue
		}
		id, _ := p["id"].(string)
		if id == "" {
			break
		}
		cwd, _ := p["cwd"].(string)
		threadSource, _ := p["thread_source"].(string)
		parentThreadID, _ := p["parent_thread_id"].(string) // null -> ""
		forkedFromID, _ := p["forked_from_id"].(string)     // null -> ""
		found = meta{
			id:             id,
			cwd:            cwd,
			threadSource:   threadSource,
			parentThreadID: parentThreadID,
			forkedFromID:   forkedFromID,
		}
		ok = true
		break
	}

	metaCacheMu.Lock()
	metaCache[path] = metaCacheEntry{
		mtime: fi.ModTime().UnixNano(),
		size:  fi.Size(),
		meta:  found,
		ok:    ok,
	}
	metaCacheMu.Unlock()

	return found, ok
}
