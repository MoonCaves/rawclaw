// Package claudeweb is the Source adapter for Claude's account data-export —
// the "claude-web" source: cloud conversations (claude.ai, the Desktop app,
// Cowork) that live on the account rather than on local disk. There is no
// consumer chat-history API, so the export ZIP is the only extraction path; the
// adapter is therefore fed EXPLICITLY by `rawclaw import <zip|dir>` and is NOT
// auto-discovered on ordinary runs the way claude/codex re-scan a live
// transcript directory.
//
// The export ZIP carries a top-level conversations.json — an array of
// conversation objects, each with a chat_messages array. Discover yields one
// Container per conversation (keyed on the conversation's own uuid, like the
// claude/codex adapters key on their session uuid); Messages maps a
// conversation's chat_messages onto the normalized model.Message, reusing the
// shared internal/parse block handling so text/thinking/tool_use/tool_result
// content is flattened and capped exactly as the CLI transcripts are. The
// source name ("claude-web") travels as the index's source_tool column, not as
// an id prefix — so a read-ref's <session8> stays distinctive per conversation.
//
// Prior art (adopt-the-proven-shape): the sibling internal/source/codex adapter
// (Adapter/New/Discover/Messages shape, sender->role mapping); internal/parse
// ExtractText for content-block flattening; and, for the export format itself,
// public Claude/ChatGPT export importers — an export's conversations.json is a
// JSON array whose items carry chat_messages (tutti-os/tutti's Go Claude-export
// validator; elizaOS/eliza's import-conversations parsers).
package claudeweb

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
	"sync"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/source"
)

// ID is the stable source name stamped as each imported row's source_tool and
// used by the --source flag. It is NOT part of a session id (see the package
// doc): the conversation uuid alone identifies the session.
const ID = "claude-web"

// conversationsFile is the export member holding the conversation array.
const conversationsFile = "conversations.json"

// Adapter reads one Claude account data-export. It is constructed WITH the
// import path (a .zip export or an already-extracted directory) and parses that
// export's conversations.json once, caching the result so Discover and the
// per-container Messages calls share a single parse. Not safe for concurrent
// construction reuse across imports; construct one per `rawclaw import`.
type Adapter struct {
	path string // the import path: a .zip export or an extracted directory

	once    sync.Once
	byID    map[string]conversation // conversation.uuid -> conversation
	order   []string                // conversation.uuid in discovery order
	loadErr error
}

// pathKey is a container's synthetic, stable Path: a per-conversation identity
// keyed on the conversation uuid, NOT the transient export file. The export zip
// is one-shot and non-canonical (a re-export is a different file holding the
// same conversations), so a file-path watermark can't survive a re-import. The
// claude-web import path (index.ImportClaudeWeb) reconciles on conversation
// identity and never stats this value; it is stored as the row's source_path
// purely as a stable identity.
func pathKey(convUUID string) string { return ID + ":" + convUUID }

// Compile-time proof the adapter satisfies the ingest port.
var _ source.Source = (*Adapter)(nil)

// New returns an adapter bound to an import path (a .zip export or an
// already-extracted export directory). The path is read lazily on the first
// Discover/Messages call.
func New(importPath string) *Adapter { return &Adapter{path: importPath} }

// conversation is the subset of an export conversation object this adapter
// reads. Unknown fields are ignored (lenient parse), so a future schema tweak
// that adds fields does not break the import.
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
// is read — the opaque per-account identity used to scope re-import
// reconciliation. No email / name / phone from the export is ever read (PII).
type accountRef struct {
	UUID string `json:"uuid"`
}

// message is the subset of an export chat_message this adapter reads. content
// holds the typed blocks (text/thinking/tool_use/tool_result) fed to
// internal/parse; text is the export's flat convenience copy, used only as a
// fallback when content is absent/empty.
type message struct {
	UUID      string `json:"uuid"`
	Text      string `json:"text"`
	Content   []any  `json:"content"`
	Sender    string `json:"sender"` // "human" | "assistant"
	CreatedAt string `json:"created_at"`
}

// Discover parses the export's conversations.json and returns one Container per
// conversation: ID is the bare conversation uuid (the source name rides the
// index's source_tool column, not the id); CWD is "" because the export drops
// the working directory a Cowork/Code session ran in, so these cloud sessions
// cannot be project-scoped; they are never subagents or forks. Path is a stable
// synthetic per-conversation key (see pathKey), not the transient export file,
// so a re-import reconciles on conversation identity. A malformed or non-Claude
// export is a real error (see load); an empty conversation array is not — it
// yields zero containers.
func (a *Adapter) Discover() ([]source.Container, error) {
	if err := a.load(); err != nil {
		return nil, err
	}
	out := make([]source.Container, 0, len(a.order))
	for _, id := range a.order {
		out = append(out, source.Container{
			ID:   id,
			Path: pathKey(id),
			CWD:  "",
		})
	}
	return out, nil
}

// Account returns the export's owning account uuid — the axis that scopes
// re-import reconciliation so importing one account never touches another's
// conversations. A Claude data-export is single-account (every conversation
// carries the same account.uuid), so this returns the first non-empty account
// uuid found; "" when the export carries none. Only the opaque uuid is read,
// never the account's email / name / phone (PII).
func (a *Adapter) Account() (string, error) {
	if err := a.load(); err != nil {
		return "", err
	}
	for _, id := range a.order {
		if u := a.byID[id].Account.UUID; u != "" {
			return u, nil
		}
	}
	return "", nil
}

// NewestUpdatedAt returns the newest conversation updated_at across the export,
// as epoch seconds — the staleness signal the import's mirror-prune guard reads
// so an older re-export can never wipe conversations a newer one established. It
// falls back to created_at for a conversation with no updated_at, and returns 0
// for an empty export.
func (a *Adapter) NewestUpdatedAt() (float64, error) {
	if err := a.load(); err != nil {
		return 0, err
	}
	var newest float64
	for _, id := range a.order {
		c := a.byID[id]
		ts := parse.ISOToEpoch(c.UpdatedAt)
		if ts == 0 {
			ts = parse.ISOToEpoch(c.CreatedAt)
		}
		if ts > newest {
			newest = ts
		}
	}
	return newest, nil
}

// Messages maps one conversation's chat_messages onto normalized messages in
// created_at order: sender human->user, assistant->assistant; each message's
// text is the parse-flattened content blocks (falling back to the export's flat
// text when content is empty). Messages are deduplicated on their own uuid
// within the dump, and a textless message is dropped. An unknown container
// (not in this export) yields no messages and no error.
func (a *Adapter) Messages(c source.Container) ([]model.Message, error) {
	if err := a.load(); err != nil {
		return nil, err
	}
	conv, ok := a.byID[c.ID]
	if !ok {
		return nil, nil
	}

	ordered := append([]message(nil), conv.ChatMessages...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return parse.ISOToEpoch(ordered[i].CreatedAt) < parse.ISOToEpoch(ordered[j].CreatedAt)
	})

	out := make([]model.Message, 0, len(ordered))
	seen := make(map[string]struct{}, len(ordered))
	for _, m := range ordered {
		if m.UUID != "" {
			if _, dup := seen[m.UUID]; dup {
				continue // dedup on message.uuid within the dump
			}
			seen[m.UUID] = struct{}{}
		}
		text := messageText(m)
		if text == "" {
			continue // nothing searchable (e.g. an attachment-only turn)
		}
		out = append(out, model.Message{
			Role:  mapSender(m.Sender),
			Text:  text,
			TS:    parse.ISOToEpoch(m.CreatedAt),
			TSISO: m.CreatedAt,
			UUID:  m.UUID,
		})
	}
	return out, nil
}

// messageText flattens a chat_message's searchable text. The export's typed
// content blocks share the CLI transcript's block schema, so a synthetic
// {message:{content:[...]}} record feeds the SAME internal/parse.ExtractText
// path the CLI uses — inheriting its [THINKING]/[TOOL:...]/[TOOL_RESULT] tagging
// and BlockCap/ToolResCap size caps. The export's flat text field is the
// fallback for a message that carries no content blocks.
func messageText(m message) string {
	if len(m.Content) > 0 {
		if s := parse.ExtractText(map[string]any{
			"message": map[string]any{"content": m.Content},
		}); s != "" {
			return s
		}
	}
	return m.Text
}

// mapSender maps an export sender onto the messages-table role vocabulary
// (user|assistant|system|summary). An empty sender defaults to user so the
// row's role is never empty (the model.Message contract); an unrecognized
// sender passes through untouched, mirroring the codex adapter's role mapping.
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

// load parses the export's conversations.json exactly once, caching the
// conversations keyed by uuid (and their discovery order) so Discover and every
// Messages call share a single read. A missing conversations.json, an
// unreadable ZIP, or a body that is not a JSON array is a real error — a user
// who points import at the wrong file gets told, rather than a silent no-op.
func (a *Adapter) load() error {
	a.once.Do(func() { a.loadErr = a.doLoad() })
	return a.loadErr
}

func (a *Adapter) doLoad() error {
	a.byID = make(map[string]conversation)
	err := a.streamConversations(func(c conversation) error {
		if c.UUID == "" {
			return nil // a conversation with no uuid cannot be identified — skip it
		}
		if _, dup := a.byID[c.UUID]; dup {
			return nil // conversation-level dedup within the dump (and across batches)
		}
		a.byID[c.UUID] = c
		a.order = append(a.order, c.UUID)
		return nil
	})
	if err != nil {
		// A malformed member fails the WHOLE import atomically: reset any partial
		// state so no half-loaded export is ever observed (no partial write
		// downstream, since Discover errors before ImportClaudeWeb runs).
		a.byID, a.order = nil, nil
		return err
	}
	return nil
}

// batchMemberRe matches a multi-batch export member: "data-…-batch-NNNN.zip".
// A large account exports as several batch zips that are ONE logical import.
var batchMemberRe = regexp.MustCompile(`^(.*-batch-)\d+\.zip$`)

// streamConversations reads every conversations.json source in turn and streams
// each conversation object to fn WITHOUT loading a whole file into memory (a
// json.Decoder buffers only the current object). The sources are: one file for
// a directory import; or, for a zip import, all the batch-sibling zips (globbed
// when the given zip is part of a "…-batch-NNNN.zip" set) — so a multi-batch
// export ingests as one.
func (a *Adapter) streamConversations(fn func(conversation) error) error {
	info, statErr := os.Stat(a.path)
	if statErr != nil {
		return fmt.Errorf("%s: %w", ID, statErr)
	}
	if info.IsDir() {
		return streamFile(filepath.Join(a.path, conversationsFile), fn)
	}
	for _, zp := range batchSiblings(a.path) {
		if err := streamZip(zp, fn); err != nil {
			return err
		}
	}
	return nil
}

// batchSiblings returns every zip belonging to zipPath's multi-batch set (all
// "<prefix>-batch-*.zip" in the same directory), sorted; or just zipPath itself
// when it is not a batch member.
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
