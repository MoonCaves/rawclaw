// Package durable is rawclaw's transcript vault: the raw, rawclaw-OWNED copy of
// every session it indexes, plus the small sidecar of index facts that copy
// cannot carry by itself. It exists so the index databases are a rebuildable
// CACHE rather than the only place retained history lives — today a session
// whose source file was purged upstream survives solely as rows in a db, which
// makes deleting that db a silent, unrecoverable history loss.
//
// The on-disk shape is the one the claude-web import already writes: one JSONL
// file per session in Claude record shape, staged in a temp file and committed
// by rename so a crash mid-write can never leave a half-transcript in the tree.
// Content blocks are stored VERBATIM wherever the source is already in that
// shape (a byte copy), so a rebuilt index is byte-identical to the live one.
package durable

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/paths"
)

// transcriptExt / metaExt are the two files one vaulted session occupies. They
// are siblings rather than one combined file so the transcript stays a plain,
// readable Claude-shape JSONL that any other tool can consume unchanged.
const (
	transcriptExt = ".jsonl"
	metaExt       = ".meta.json"
)

// Root is the vault directory. Resolved per call rather than cached, because a
// test (and a user moving XDG_DATA_HOME) must be able to redirect it.
func Root() string { return paths.TranscriptsRoot() }

// Meta is what the transcript itself cannot carry: where the session came from,
// which db columns were derived outside the file, and the retention watermark.
//
// SourcePath/SourceMTime/SourceSize/SourceFP are recorded so a rebuild can
// restore the file_index watermark keyed on the ORIGINAL source path. Keying it
// on the vault path instead would be actively harmful: the next live pass
// compares file_index paths against the source walk, so a vault-keyed row reads
// as "source absent" and would stamp missing_since on a perfectly live session.
type Meta struct {
	ID            string  `json:"id"`
	Source        string  `json:"source"`
	Origin        string  `json:"origin,omitempty"`
	Project       string  `json:"project,omitempty"`
	CWD           string  `json:"cwd,omitempty"`
	IsSubagent    bool    `json:"is_subagent,omitempty"`
	ParentID      string  `json:"parent_id,omitempty"`
	SourcePath    string  `json:"source_path,omitempty"`
	SourceMTime   float64 `json:"source_mtime,omitempty"`
	SourceSize    int64   `json:"source_size,omitempty"`
	SourceFP      string  `json:"source_fp,omitempty"`
	OnlyCopySince float64 `json:"only_copy_since,omitempty"`
	StoredAt      float64 `json:"stored_at,omitempty"`
}

// UnmarshalJSON unmarshals Meta, supporting legacy sidecars carrying missing_since.
func (m *Meta) UnmarshalJSON(data []byte) error {
	type Alias Meta
	aux := struct {
		*Alias
		LegacyMissingSince float64 `json:"missing_since,omitempty"`
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if m.OnlyCopySince == 0 && aux.LegacyMissingSince != 0 {
		m.OnlyCopySince = aux.LegacyMissingSince
	}
	return nil
}

// Session is one vaulted session: its meta plus the transcript's absolute path,
// so a caller enumerating the vault can read the transcript without rebuilding
// the path from the id.
type Session struct {
	Meta
	Transcript string
}

// StoreFile vaults a source file that is ALREADY in Claude record shape, by
// copying its bytes verbatim. Verbatim rather than re-serialized because the
// index is built by re-parsing this file: any normalization we applied here
// would show up as a diff between the live index and the rebuilt one.
// A source whose fingerprint already matches the vaulted copy is not rewritten,
// only un-flagged: a full reindex re-offers every file rawclaw has ever seen,
// and copying an unchanged corpus byte for byte on each one is pure cost. The
// un-flag is not optional though — reaching here means the file was just read
// off disk, so a stale "its source is gone" watermark in the sidecar would
// outlive the truth and mislabel a live session after the next rebuild.
func StoreFile(m Meta, src string) error {
	if unchanged(m) {
		return SetOnlyCopySince(m.ID, 0)
	}
	body, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("durable: read source %s: %w", src, err)
	}
	return write(m, body)
}

// unchanged reports whether the vault already holds this exact source file,
// judged by the same (path, size, fingerprint) triple the index watermarks on.
// An empty fingerprint or size never matches, so an unfingerprintable source
// always rewrites rather than being assumed current.
func unchanged(m Meta) bool {
	if m.SourcePath == "" || m.SourceFP == "" || m.SourceSize == 0 {
		return false
	}
	tp, err := PathFor(m.ID)
	if err != nil {
		return false
	}
	if _, err := os.Stat(tp); err != nil {
		return false
	}
	prev, err := readMeta(metaPathOf(tp))
	if err != nil {
		return false
	}
	return prev.SourcePath == m.SourcePath && prev.SourceSize == m.SourceSize && prev.SourceFP == m.SourceFP
}

// StoreMessages vaults a session whose source is NOT Claude-shaped (a Codex
// rollout, say) by rendering its already-flattened messages INTO that shape.
// One format for the whole vault, so the rebuild has exactly one reader.
func StoreMessages(m Meta, msgs []model.Message) error {
	body, err := render(m, msgs)
	if err != nil {
		return err
	}
	return write(m, body)
}

// record / recordMessage are one materialized transcript line in Claude record
// shape — the same struct claude-web's importer writes, so the vault holds a
// single format regardless of which adapter fed it.
type record struct {
	Type      string        `json:"type"`
	UUID      string        `json:"uuid,omitempty"`
	Timestamp string        `json:"timestamp,omitempty"`
	CWD       string        `json:"cwd,omitempty"`
	Message   recordMessage `json:"message"`
}

type recordMessage struct {
	Role    string `json:"role"`
	Content []any  `json:"content"`
}

// render turns flattened messages into Claude-shape JSONL.
//
// The record "type" must be one the indexer indexes, or the rebuild would drop
// the line; a role outside that set is therefore carried as a "system" record
// whose message.role keeps the true value, which is where the reader looks for
// the role anyway. The session's cwd rides on every line because the rebuild
// reads cwd out of the transcript, and a vault file should stand on its own
// even if its sidecar is lost.
func render(m Meta, msgs []model.Message) ([]byte, error) {
	var b strings.Builder
	for _, msg := range msgs {
		iso := msg.TSISO
		if iso == "" && msg.TS != 0 {
			iso = time.Unix(0, int64(msg.TS*1e9)).UTC().Format("2006-01-02T15:04:05.000Z")
		}
		r := record{
			Type:      recordType(msg.Role),
			UUID:      msg.UUID,
			Timestamp: iso,
			CWD:       m.CWD,
			Message: recordMessage{
				Role:    msg.Role,
				Content: []any{map[string]any{"type": "text", "text": msg.Text}},
			},
		}
		line, err := json.Marshal(r)
		if err != nil {
			return nil, fmt.Errorf("durable: encode record for %s: %w", m.ID, err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// recordType maps a message role onto an indexable record type, falling back to
// "system" for roles the indexer does not recognize (see render).
func recordType(role string) string {
	for _, t := range parse.IndexableTypes {
		if role == t {
			return role
		}
	}
	return "system"
}

// write commits a transcript and its sidecar into the vault. The transcript is
// staged and renamed first, then the sidecar, then the directory is fsynced —
// so a crash can strand a transcript without meta (List degrades gracefully),
// but never a meta pointing at a transcript that was never written.
func write(m Meta, body []byte) error {
	tp, err := PathFor(m.ID)
	if err != nil {
		return err
	}
	dir := filepath.Dir(tp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("durable: create %s: %w", dir, err)
	}
	m.StoredAt = float64(time.Now().UnixNano()) / 1e9
	metaBytes, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("durable: encode meta for %s: %w", m.ID, err)
	}
	if err := commit(tp, body); err != nil {
		return err
	}
	if err := commit(metaPathOf(tp), append(metaBytes, '\n')); err != nil {
		return err
	}
	return fsyncDir(dir)
}

// commit writes data to a temp file in dst's own directory, fsyncs it, and
// renames it over dst. Same-directory staging is what makes the rename atomic;
// the fsync before it is what makes the CONTENT durable, not just the name.
func commit(dst string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(dst), ".rawclaw-stage-*")
	if err != nil {
		return fmt.Errorf("durable: stage %s: %w", dst, err)
	}
	tmp := f.Name()
	defer func() {
		_ = os.Remove(tmp) // best-effort cleanup; no-op once rename succeeds
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("durable: write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("durable: sync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("durable: close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("durable: commit %s: %w", dst, err)
	}
	return nil
}

// fsyncDir flushes a directory entry so a rename survives a crash. Failure is
// non-fatal on filesystems that refuse to open a directory for sync.
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer d.Close()
	_ = d.Sync()
	return nil
}

// PathFor is the vault transcript path for a session id. A subagent id is
// lineage-namespaced ("parent/child"), so its slashes nest as directories the
// same way the source tools nest their own trees.
func PathFor(id string) (string, error) {
	rel, err := relFor(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(Root(), rel), nil
}

func relFor(id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", errors.New("durable: empty session id")
	}
	segs := strings.Split(id, "/")
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		out = append(out, sanitize(s))
	}
	return filepath.Join(out...) + transcriptExt, nil
}

// sanitize makes one id segment safe as a path component: anything outside the
// portable set folds to "-", and a segment that is only dots is prefixed, since
// "." and ".." would address the vault's own directories instead of a file.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := b.String()
	if strings.Trim(out, ".") == "" {
		return "_" + out
	}
	return out
}

func metaPathOf(transcript string) string {
	return strings.TrimSuffix(transcript, transcriptExt) + metaExt
}

// Has reports whether a session is already vaulted.
func Has(id string) bool {
	tp, err := PathFor(id)
	if err != nil {
		return false
	}
	_, err = os.Stat(tp)
	return err == nil
}

// SetOnlyCopySince records (since > 0) or clears (since == 0) the retention
// watermark on a vaulted session, indicating RawClaw is now the only copy after
// the CLI deleted its transcript. A session that was never vaulted
// is not an error: there is simply no durable copy to annotate.
func SetOnlyCopySince(id string, since float64) error {
	tp, err := PathFor(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(tp); err != nil {
		return nil
	}
	m, err := readMeta(metaPathOf(tp))
	if err != nil {
		return err
	}
	if m.ID == "" {
		m.ID = id
	}
	if m.OnlyCopySince == since {
		return nil
	}
	m.OnlyCopySince = since
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("durable: encode meta for %s: %w", id, err)
	}
	if err := commit(metaPathOf(tp), append(b, '\n')); err != nil {
		return err
	}
	return fsyncDir(filepath.Dir(tp))
}

// Remove deletes a session's durable copy. This is the delete path for a real
// user delete (an explicit tombstone): retention prunes the row, and the vault
// must not hold a copy that a later rebuild would resurrect.
func Remove(id string) error {
	tp, err := PathFor(id)
	if err != nil {
		return err
	}
	for _, p := range []string{tp, metaPathOf(tp)} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("durable: remove %s: %w", p, err)
		}
	}
	pruneEmptyDirs(filepath.Dir(tp))
	return nil
}

// pruneEmptyDirs walks up from dir removing now-empty directories, stopping at
// the vault root so a removed subagent lineage leaves no empty shell behind.
func pruneEmptyDirs(dir string) {
	root := filepath.Clean(Root())
	for cur := filepath.Clean(dir); strings.HasPrefix(cur, root) && cur != root; cur = filepath.Dir(cur) {
		if err := os.Remove(cur); err != nil {
			return
		}
	}
}

// List enumerates every vaulted session, sorted by id so a rebuild is
// deterministic. A transcript whose sidecar is missing or unreadable is still
// returned, with the id recovered from its path — a stranded transcript is
// history, and dropping it would defeat the point of the vault.
func List() ([]Session, error) {
	root := Root()
	var out []Session
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, transcriptExt) {
			return nil
		}
		m, merr := readMeta(metaPathOf(p))
		if merr != nil || m.ID == "" {
			rel, rerr := filepath.Rel(root, p)
			if rerr != nil {
				return nil
			}
			m.ID = filepath.ToSlash(strings.TrimSuffix(rel, transcriptExt))
		}
		out = append(out, Session{Meta: m, Transcript: p})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("durable: walk %s: %w", root, err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// readMeta loads a sidecar. A missing sidecar yields a zero Meta with no error,
// so callers can treat "no meta yet" and "meta says nothing" identically.
func readMeta(p string) (Meta, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Meta{}, nil
		}
		return Meta{}, fmt.Errorf("durable: read meta %s: %w", p, err)
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return Meta{}, fmt.Errorf("durable: parse meta %s: %w", p, err)
	}
	return m, nil
}
