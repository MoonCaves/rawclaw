package claudeweb

// Materialize turns a Claude account data-export into the raw transcript tree the
// adapter reads: one JSONL file per conversation under
// <root>/<account>/<conversation-uuid>.jsonl, in Claude record shape. Each write
// is STAGED in a temp dir and committed into the tree only after the whole export
// parses cleanly, so a malformed export — or a refused account-less import under
// mirror — leaves the tree untouched (no partial write). Content blocks are
// stored VERBATIM, so the rebuilt index is byte-identical.

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/parse"
)

// AccountImport is one account's materialization outcome.
type AccountImport struct {
	Account   string              // account dir name (sanitized account uuid; "unknown" if none)
	Dir       string              // <root>/<Account>
	ConvUUIDs map[string]struct{} // conversation uuids present in THIS export (for mirror-prune)
	Newest    float64             // newest updated_at across this account's conversations
	Written   int                 // transcript files written
	Messages  int                 // transcript records (messages) written
}

// MaterializeResult reports what an import wrote, keyed by account dir name.
type MaterializeResult struct {
	Accounts map[string]*AccountImport
}

// Materialize streams the export at exportPath and writes each conversation as a
// transcript under root. mirror=true refuses an account-less export: without an
// account, every account-less import shares one bucket, so mirror could prune
// another import's conversations. Returns per-account info for the caller's
// mirror-prune + reindex. Fail-closed: any parse error or refusal leaves the
// tree untouched.
func Materialize(exportPath, root string, mirror bool) (*MaterializeResult, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("%s: create transcript root: %w", ID, err)
	}
	staging, err := os.MkdirTemp(root, ".import-*")
	if err != nil {
		return nil, fmt.Errorf("%s: create staging dir: %w", ID, err)
	}
	// Cleaned up on every path: on error nothing was committed; on success the
	// staged files were renamed out, leaving an empty staging dir to remove.
	defer os.RemoveAll(staging)

	res := &MaterializeResult{Accounts: map[string]*AccountImport{}}
	staged := map[string]string{} // committed-dest path -> staged path

	streamErr := newExport(exportPath).streamConversations(func(c conversation) error {
		// Skip a conversation with no uuid, or a uuid that cannot safely become a
		// filename — a crafted or corrupt export must never steer a transcript
		// write outside the account tree (path traversal).
		if !safeConvUUID(c.UUID) {
			return nil
		}
		if c.Account.UUID == "" && mirror {
			return fmt.Errorf("%s: refusing an export with no account uuid under RAWCLAW_RETENTION=mirror — account-less imports share one bucket, so mirror could cross-prune unrelated conversations; import under the default (keep) instead", ID)
		}
		acct := accountDir(c.Account.UUID)
		ai := res.account(acct, root)
		if _, dup := ai.ConvUUIDs[c.UUID]; dup {
			return nil // conversation-level dedup within the dump / across batches
		}

		stagedPath := filepath.Join(staging, acct, c.UUID+transcriptExt)
		destPath := filepath.Join(ai.Dir, c.UUID+transcriptExt)
		nMsgs, err := mergeTranscriptFile(stagedPath, destPath, c)
		if err != nil {
			return err
		}
		staged[filepath.Join(ai.Dir, c.UUID+transcriptExt)] = stagedPath
		ai.ConvUUIDs[c.UUID] = struct{}{}
		ai.Written++
		ai.Messages += nMsgs
		if ts := c.newestUpdatedAt(); ts > ai.Newest {
			ai.Newest = ts
		}
		return nil
	})
	if streamErr != nil {
		return nil, streamErr // staging removed by defer — nothing committed
	}

	// Commit: rename each staged file into the real tree (atomic per file; same
	// filesystem since staging is under root). Only reached after a clean parse.
	// The staged files were already fsync'd (mergeTranscriptFile), so a crash
	// after the rename can't leave a renamed-but-empty transcript — the exact
	// "only copy" durability class. Each account dir is fsync'd after its renames
	// so the directory entries themselves survive a power loss.
	dirsToSync := map[string]struct{}{}
	for dest, src := range staged {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, fmt.Errorf("%s: create account dir: %w", ID, err)
		}
		if err := os.Rename(src, dest); err != nil {
			return nil, fmt.Errorf("%s: commit transcript: %w", ID, err)
		}
		dirsToSync[filepath.Dir(dest)] = struct{}{}
	}
	for dir := range dirsToSync {
		if err := fsyncDir(dir); err != nil {
			return nil, fmt.Errorf("%s: fsync account dir: %w", ID, err)
		}
	}
	return res, nil
}

// fsyncDir flushes a directory's entries to disk so a rename survives a crash.
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

// newestMarker holds an account's freshest imported updated_at (epoch), the
// staleness guard for the mirror prune. It is not a .jsonl file, so the adapter
// never reads it as a transcript.
const newestMarker = ".newest"

// PrunePlan reports the conversation uuids that mirror mode WOULD delete for this
// account: transcripts on disk that are ABSENT from the export just imported (a
// cloud-side deletion). It deletes NOTHING — the caller shows the plan and gets
// the user's approval (a y/N prompt, or --yes) before Commit removes them, since
// a silent delete would contradict the archive promise. The plan is empty unless
// mirror is on AND this export is at least as fresh as the last import for the
// account (a stale re-export can never propose a prune). Under the keep default
// the plan is always empty.
func PrunePlan(ai *AccountImport, mirror bool) []string {
	if !mirror {
		return nil
	}
	if ai.Newest < readNewestMarker(filepath.Join(ai.Dir, newestMarker)) {
		return nil // stale export — cannot propose deletions
	}
	files, _ := filepath.Glob(filepath.Join(ai.Dir, "*"+transcriptExt))
	var prune []string
	for _, f := range files {
		uuid := convUUIDFromFile(f)
		if _, present := ai.ConvUUIDs[uuid]; present {
			continue
		}
		prune = append(prune, uuid)
	}
	return prune
}

// Commit finalizes one account's import: it deletes exactly the APPROVED prune
// set (the uuids the caller confirmed — pass nil to delete nothing), then advances
// the freshness watermark to the newest seen. Removing a uuid whose file is
// already gone is not an error (idempotent).
func Commit(ai *AccountImport, approvedPrune []string) error {
	for _, uuid := range approvedPrune {
		f := filepath.Join(ai.Dir, uuid+transcriptExt)
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: mirror-prune %s: %w", ID, uuid, err)
		}
	}
	marker := filepath.Join(ai.Dir, newestMarker)
	if ai.Newest > readNewestMarker(marker) {
		return writeNewestMarker(marker, ai.Newest)
	}
	return nil
}

// readNewestMarker reads an account's freshness watermark (0 when absent/unreadable).
func readNewestMarker(path string) float64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return 0
	}
	return f
}

// writeNewestMarker stores an account's freshness watermark.
func writeNewestMarker(path string, v float64) error {
	if err := os.WriteFile(path, []byte(strconv.FormatFloat(v, 'f', -1, 64)), 0o644); err != nil {
		return fmt.Errorf("%s: write freshness marker: %w", ID, err)
	}
	return nil
}

// account returns (creating) the per-account import record.
func (r *MaterializeResult) account(acct, root string) *AccountImport {
	ai, ok := r.Accounts[acct]
	if !ok {
		ai = &AccountImport{Account: acct, Dir: filepath.Join(root, acct), ConvUUIDs: map[string]struct{}{}}
		r.Accounts[acct] = ai
	}
	return ai
}

// accountDir is the filesystem-safe directory segment for an account: the
// sanitized account uuid, or "unknown" when the export carries none. Slash/colon
// and any other non-[A-Za-z0-9._-] fold to '-', so the segment can never escape
// the tree or act as a path/ref separator.
func accountDir(account string) string {
	if account == "" {
		return "unknown"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, account)
}

// safeConvUUID reports whether a conversation uuid is safe to use as a transcript
// filename segment. Real uuids are hex + hyphens; anything else — a path
// separator, "..", an empty or "."/".." value — is rejected so a crafted or
// corrupt export can never steer the transcript write outside the account tree.
func safeConvUUID(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// mergeTranscriptFile writes one conversation's transcript to stagedPath, MERGED
// (union by record identity) with any EXISTING transcript at destPath — so a
// re-import NEVER drops a message a prior import wrote (the raw-archive
// keep-everything guarantee), even if a later export is smaller. Existing records
// always win on a collision (append-only); new records are appended. Content
// blocks are stored VERBATIM (a text fallback synthesized only when a message
// carries none), so the reindex flattens to identical text. Records are written
// in created_at order. Returns the total record count.
func mergeTranscriptFile(stagedPath, destPath string, c conversation) (int, error) {
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0o755); err != nil {
		return 0, fmt.Errorf("%s: create staging account dir: %w", ID, err)
	}

	existing, err := readTranscriptRecords(destPath)
	if err != nil {
		return 0, err
	}
	seen := make(map[string]struct{}, len(existing)+len(c.ChatMessages))
	merged := make([]recordLine, 0, len(existing)+len(c.ChatMessages))
	for _, r := range existing {
		id := recordIdentity(r)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		merged = append(merged, r)
	}
	// New records from this export (uuid-deduped within the conversation first).
	msgSeen := make(map[string]struct{}, len(c.ChatMessages))
	for _, m := range c.ChatMessages {
		if m.UUID != "" {
			if _, dup := msgSeen[m.UUID]; dup {
				continue
			}
			msgSeen[m.UUID] = struct{}{}
		}
		rec := transcriptRecord(m)
		id := recordIdentity(rec)
		if _, dup := seen[id]; dup {
			continue // already present (from a prior import) — never overwrite/drop
		}
		seen[id] = struct{}{}
		merged = append(merged, rec)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return parse.ISOToEpoch(merged[i].Timestamp) < parse.ISOToEpoch(merged[j].Timestamp)
	})

	f, err := os.Create(stagedPath)
	if err != nil {
		return 0, fmt.Errorf("%s: create transcript: %w", ID, err)
	}
	enc := json.NewEncoder(f)
	for _, r := range merged {
		if err := enc.Encode(r); err != nil {
			f.Close()
			return 0, fmt.Errorf("%s: write record: %w", ID, err)
		}
	}
	// fsync BEFORE the caller renames this staged file into the tree: the
	// contents must be durable before the rename, so a crash can't leave a
	// renamed-but-truncated transcript (the only copy of this conversation).
	if err := f.Sync(); err != nil {
		f.Close()
		return 0, fmt.Errorf("%s: fsync transcript: %w", ID, err)
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	return len(merged), nil
}

// readTranscriptRecords parses an existing transcript into records (nil when the
// file is absent — the first import of a conversation). A malformed line is
// skipped, never fatal (the merge is best-effort additive).
func readTranscriptRecords(path string) ([]recordLine, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: read existing transcript: %w", ID, err)
	}
	var out []recordLine
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r recordLine
		if json.Unmarshal([]byte(line), &r) == nil {
			out = append(out, r)
		}
	}
	return out, nil
}

// recordIdentity is a record's merge key: its message uuid when present, else a
// content hash (type + timestamp + content) — so a uuidless message dedups by
// content and never accumulates across re-imports.
func recordIdentity(r recordLine) string {
	if r.UUID != "" {
		return "u:" + r.UUID
	}
	blocks, _ := json.Marshal(r.Message.Content)
	sum := sha1.Sum([]byte(r.Type + "\x00" + r.Timestamp + "\x00" + string(blocks)))
	return "h:" + hex.EncodeToString(sum[:])
}

// transcriptRecord builds one Claude-shape JSONL record from an export message.
// type == role so the record is indexable; content is the verbatim export blocks
// (a synthesized text block when the message carried only the flat text field).
func transcriptRecord(m message) recordLine {
	role := mapSender(m.Sender)
	content := m.Content
	if len(content) == 0 && m.Text != "" {
		content = []any{map[string]any{"type": "text", "text": m.Text}}
	}
	return recordLine{
		Type:      role,
		UUID:      m.UUID,
		Timestamp: m.CreatedAt,
		Message:   recordMessage{Role: role, Content: content},
	}
}

// recordLine is a materialized transcript line (Claude record shape).
type recordLine struct {
	Type      string        `json:"type"`
	UUID      string        `json:"uuid,omitempty"`
	Timestamp string        `json:"timestamp,omitempty"`
	Message   recordMessage `json:"message"`
}

type recordMessage struct {
	Role    string `json:"role"`
	Content []any  `json:"content"`
}
