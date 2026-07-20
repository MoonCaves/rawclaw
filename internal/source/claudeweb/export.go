package claudeweb

// The export reader: streams a Claude account data-export (a .zip export, an
// already-extracted directory, or a multi-batch set of zips) one conversation at
// a time, without loading a whole conversations.json into memory. It is used
// ONLY by materialize.go to write the raw transcript tree; the Source adapter
// (claudeweb.go) reads that tree, never the export.

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/MoonCaves/rawclaw/internal/parse"
)

// conversationsFile is the export member holding the conversation array.
const conversationsFile = "conversations.json"

// export is a one-shot reader bound to an import path.
type export struct {
	path string
}

func newExport(importPath string) *export { return &export{path: importPath} }

// conversation is the subset of an export conversation object read here. Unknown
// fields are ignored (lenient parse), so a future schema tweak that adds fields
// does not break the import.
type conversation struct {
	UUID         string     `json:"uuid"`
	Name         string     `json:"name"`
	Summary      string     `json:"summary"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
	Account      accountRef `json:"account"`
	ChatMessages []message  `json:"chat_messages"`
}

// accountRef is the owning account carried on every conversation. Only its uuid
// is read — the opaque per-account identity. No email / name / phone (PII).
type accountRef struct {
	UUID string `json:"uuid"`
}

// message is the subset of an export chat_message read here. content holds the
// typed blocks (text/thinking/tool_use/tool_result); text is the flat fallback.
type message struct {
	UUID      string `json:"uuid"`
	Text      string `json:"text"`
	Content   []any  `json:"content"`
	Sender    string `json:"sender"` // "human" | "assistant"
	CreatedAt string `json:"created_at"`
}

// newestUpdatedAt returns a conversation's newest timestamp (updated_at, else
// created_at) as epoch seconds — the staleness signal for the mirror guard.
func (c conversation) newestUpdatedAt() float64 {
	if ts := parse.ISOToEpoch(c.UpdatedAt); ts != 0 {
		return ts
	}
	return parse.ISOToEpoch(c.CreatedAt)
}

// batchMemberRe matches a multi-batch export member: "data-…-batch-NNNN.zip".
var batchMemberRe = regexp.MustCompile(`^(.*-batch-)\d+\.zip$`)

// streamConversations reads every conversations.json source in turn and streams
// each conversation object to fn WITHOUT loading a whole file into memory (a
// json.Decoder buffers only the current object). Sources: one file for a
// directory import; or, for a zip import, all the batch-sibling zips (globbed
// when the given zip is part of a "…-batch-NNNN.zip" set) — a multi-batch export
// ingests as one.
func (e *export) streamConversations(fn func(conversation) error) error {
	info, statErr := os.Stat(e.path)
	if statErr != nil {
		return fmt.Errorf("%s: %w", ID, statErr)
	}
	if info.IsDir() {
		return streamFile(filepath.Join(e.path, conversationsFile), fn)
	}
	for _, zp := range batchSiblings(e.path) {
		if err := streamZip(zp, fn); err != nil {
			return err
		}
	}
	return nil
}

// batchSiblings returns every zip belonging to zipPath's multi-batch set (all
// "<prefix>-batch-*.zip" in the same directory), sorted; or just zipPath when it
// is not a batch member.
func batchSiblings(zipPath string) []string {
	m := batchMemberRe.FindStringSubmatch(filepath.Base(zipPath))
	if m == nil {
		return []string{zipPath}
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(zipPath), m[1]+"*.zip"))
	if err != nil || len(matches) == 0 {
		return []string{zipPath}
	}
	sort.Strings(matches)
	return matches
}

// streamFile streams a directory-import's conversations.json.
func streamFile(p string, fn func(conversation) error) error {
	f, err := os.Open(p)
	if err != nil {
		return fmt.Errorf("%s: read %s: %w", ID, conversationsFile, err)
	}
	defer f.Close()
	return decodeConversationArray(bufio.NewReader(f), fn)
}

// streamZip streams conversations.json out of one zip export, matching the
// member by exact name or basename at any nesting depth. A missing member is a
// clear "not a Claude data-export" error.
func streamZip(zipPath string, fn func(conversation) error) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("%s: open %s as a ZIP export: %w", ID, filepath.Base(zipPath), err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != conversationsFile && path.Base(f.Name) != conversationsFile {
			continue
		}
		rc, openErr := f.Open()
		if openErr != nil {
			return fmt.Errorf("%s: open %s in %s: %w", ID, conversationsFile, filepath.Base(zipPath), openErr)
		}
		err := decodeConversationArray(rc, fn)
		_ = rc.Close()
		return err
	}
	return fmt.Errorf("%s: %s not found in %s — not a Claude data-export", ID, conversationsFile, filepath.Base(zipPath))
}

// decodeConversationArray streams a top-level JSON array of conversation objects
// from r, decoding ONE object at a time (bounded memory, 100 MB+ safe). It
// requires the body to open with '['; a non-array or a malformed object is a
// clear error. Prior art: the encoding/json Decoder.Token + Decode-in-a-loop
// streaming-array pattern.
func decodeConversationArray(r io.Reader, fn func(conversation) error) error {
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: %s is not a valid conversation array (not a Claude data-export?): %w", ID, conversationsFile, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return fmt.Errorf("%s: %s is not a JSON array (not a Claude data-export?)", ID, conversationsFile)
	}
	for dec.More() {
		var c conversation
		if err := dec.Decode(&c); err != nil {
			return fmt.Errorf("%s: malformed conversation in %s: %w", ID, conversationsFile, err)
		}
		if err := fn(c); err != nil {
			return err
		}
	}
	return nil
}

// mapSender maps an export sender onto the messages-table role vocabulary. An
// empty sender defaults to user (never an empty role); an unrecognized sender
// passes through untouched, mirroring the codex adapter.
func mapSender(sender string) string {
	switch sender {
	case "human", "":
		return "user"
	case "assistant":
		return "assistant"
	default:
		return sender
	}
}
