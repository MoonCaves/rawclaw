package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// tagQueueFile is the queue's filename under the session-search state dir: one
// full session id per line, appended by the SessionEnd hook when a session
// finishes and drained as tagging happens (tag-write removes its session; an
// agent can `tag-queue remove` one that won't resolve). Plain text so a human
// can read or repair it with nothing but cat.
const tagQueueFile = "tag-queue"

// tagQueuePath returns the queue file's location in the session-search state
// dir (alongside the index dbs and the machine-id file).
func tagQueuePath() string {
	return filepath.Join(store.CacheDir(), tagQueueFile)
}

// validQueueID gates what the add path will store: hook input is parsed with a
// tolerant sed scan on the far side, so a malformed payload must not be able to
// smuggle arbitrary bytes (newlines, shell noise) into the queue. Session ids
// from Claude Code / Codex are uuid-shaped; letters, digits, `-` and `_` cover
// them.
func validQueueID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		ok := r == '-' || r == '_' ||
			(r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z')
		if !ok {
			return false
		}
	}
	return true
}

// readTagQueue returns the queue's session ids in file order, deduplicated
// (first occurrence wins). A missing file is an empty queue, not an error.
func readTagQueue() ([]string, error) {
	b, err := os.ReadFile(tagQueuePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", tagQueuePath(), err)
	}
	seen := make(map[string]bool)
	var ids []string
	for _, line := range strings.Split(string(b), "\n") {
		id := strings.TrimSpace(line)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

// tagQueueAdd appends id to the queue unless an entry for it already exists.
// Two SessionEnd hooks racing can in theory both pass the read check and
// append twice; the read path deduplicates, so the worst case is a redundant
// line, never a double listing.
func tagQueueAdd(id string) error {
	if !validQueueID(id) {
		return fmt.Errorf("invalid session id %q (want letters, digits, - or _)", id)
	}
	ids, err := readTagQueue()
	if err != nil {
		return err
	}
	for _, have := range ids {
		if have == id {
			return nil
		}
	}
	f, err := os.OpenFile(tagQueuePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", tagQueuePath(), err)
	}
	defer f.Close()
	if _, err := f.WriteString(id + "\n"); err != nil {
		return fmt.Errorf("append to %s: %w", tagQueuePath(), err)
	}
	return nil
}

// tagQueueRemove drops every queue entry matching id — exactly, or by prefix
// in either direction, so the 8-char form a listing (or tag-write) works with
// removes the full id the hook stored. Returns whether anything was removed.
// An emptied queue deletes the file rather than leaving a zero-byte husk.
func tagQueueRemove(id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, fmt.Errorf("empty session id")
	}
	ids, err := readTagQueue()
	if err != nil {
		return false, err
	}
	var kept []string
	removed := false
	for _, have := range ids {
		if strings.HasPrefix(have, id) || strings.HasPrefix(id, have) {
			removed = true
			continue
		}
		kept = append(kept, have)
	}
	if !removed {
		return false, nil
	}
	if len(kept) == 0 {
		if err := os.Remove(tagQueuePath()); err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("remove %s: %w", tagQueuePath(), err)
		}
		return true, nil
	}
	tmp := tagQueuePath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(kept, "\n")+"\n"), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, tagQueuePath()); err != nil {
		return false, fmt.Errorf("rename %s: %w", tmp, err)
	}
	return true, nil
}
