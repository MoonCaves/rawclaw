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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
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
	backing string                  // real, stat-able file the containers watermark on
	byID    map[string]conversation // conversation.uuid -> conversation
	order   []string                // conversation.uuid in discovery order
	loadErr error
}

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
	UUID         string    `json:"uuid"`
	Name         string    `json:"name"`
	Summary      string    `json:"summary"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
	ChatMessages []message `json:"chat_messages"`
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
// cannot be project-scoped; they are never subagents or forks. Path is the real
// export artifact, so the index's file-watermark stat succeeds. A malformed or
// non-Claude export is a real error (see load); an empty conversation array is
// not — it yields zero containers.
func (a *Adapter) Discover() ([]source.Container, error) {
	if err := a.load(); err != nil {
		return nil, err
	}
	out := make([]source.Container, 0, len(a.order))
	for _, id := range a.order {
		out = append(out, source.Container{
			ID:   id,
			Path: a.backing,
			CWD:  "",
		})
	}
	return out, nil
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
	data, backing, err := a.readConversations()
	if err != nil {
		return err
	}
	a.backing = backing

	var convs []conversation
	if err := json.Unmarshal(data, &convs); err != nil {
		return fmt.Errorf("%s: %s is not a valid conversation array (not a Claude data-export?): %w", ID, conversationsFile, err)
	}

	a.byID = make(map[string]conversation, len(convs))
	a.order = make([]string, 0, len(convs))
	for _, c := range convs {
		if c.UUID == "" {
			continue // a conversation with no uuid cannot be identified — skip it
		}
		if _, dup := a.byID[c.UUID]; dup {
			continue // conversation-level dedup within the dump
		}
		a.byID[c.UUID] = c
		a.order = append(a.order, c.UUID)
	}
	return nil
}

// readConversations returns the raw conversations.json bytes plus a real,
// stat-able backing path for the watermark. A directory import reads the file
// directly; a file import is opened as a ZIP export and the member is inflated.
func (a *Adapter) readConversations() (data []byte, backing string, err error) {
	info, statErr := os.Stat(a.path)
	if statErr != nil {
		return nil, "", fmt.Errorf("%s: %w", ID, statErr)
	}
	if info.IsDir() {
		p := filepath.Join(a.path, conversationsFile)
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil, "", fmt.Errorf("%s: read %s: %w", ID, conversationsFile, readErr)
		}
		return b, p, nil
	}
	b, zipErr := readZipMember(a.path, conversationsFile)
	if zipErr != nil {
		return nil, "", zipErr
	}
	// The ZIP itself is the stable, real backing file for the watermark; the
	// member is transient inside it.
	return b, a.path, nil
}

// readZipMember inflates a single named member from a ZIP, matching either the
// exact name or the basename at any nesting depth (an export may wrap its
// members under a top-level directory). A missing member is reported as "not a
// Claude data-export" rather than a bare not-found.
func readZipMember(zipPath, member string) ([]byte, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("%s: open %s as a ZIP export: %w", ID, filepath.Base(zipPath), err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != member && path.Base(f.Name) != member {
			continue
		}
		rc, openErr := f.Open()
		if openErr != nil {
			return nil, fmt.Errorf("%s: open %s in %s: %w", ID, member, filepath.Base(zipPath), openErr)
		}
		b, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("%s: read %s in %s: %w", ID, member, filepath.Base(zipPath), readErr)
		}
		return b, nil
	}
	return nil, fmt.Errorf("%s: %s not found in %s — not a Claude data-export", ID, member, filepath.Base(zipPath))
}
