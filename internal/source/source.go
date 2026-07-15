// Package source defines the ingest PORT: a small interface each runtime's
// transcript reader implements, so the index can ingest Claude Code, Codex, and
// future runtimes without knowing the on-disk format underneath. It is the
// ingest-side parallel to internal/embed's vector ports — defined ahead of its
// adapters, and consumed by the index, which never learns a source's format.
//
// Adapters live in subpackages (internal/source/claude, internal/source/codex)
// and import this package; this package imports none of them. Wire adapters
// explicitly with Register at start-up — never via init()-time self-registration
// (implicit ordering, no error path, breaks test isolation).
package source

import "github.com/MoonCaves/rawclaw/internal/model"

// Container is one ingestable session: the unit the index watermarks, reindexes,
// and prunes. A source yields one Container per session it can see, already
// carrying the lineage the index needs to tag subagents and collapse forks.
type Container struct {
	ID         string   // unique session id, already lineage-namespaced by the source
	Path       string   // backing file — the file_index watermark key
	CWD        string   // working dir recorded in the transcript ("" if unknown)
	IsSubagent bool     // subagent / forked child: hidden from default search
	ParentID   string   // parent session id for lineage collapse ("" = root → SQL NULL)
	ResumeArgv []string // argv that resumes this session, e.g. {"claude","--resume",id}
}

// Source reads one runtime's transcripts. Discover enumerates every session the
// source can see; Messages returns one session's messages in transcript order,
// already normalized and (where a format duplicates history, e.g. Codex forks)
// deduplicated. Seeing nothing is not an error — Discover returns (nil, nil) for
// an empty or absent corpus, mirroring the ship-empty rule of the embed ports.
type Source interface {
	Discover() ([]Container, error)
	Messages(c Container) ([]model.Message, error)
}

// Registration is a source's selection metadata, kept OFF the behavioral
// interface (the image.RegisterFormat / database/sql.Register split): Detect
// reports whether a path belongs to this source (for --source auto-detection),
// New constructs a ready adapter. ID is the stable source name ("claude",
// "codex") used by the --source flag and for namespacing its cache.
type Registration struct {
	ID     string
	Detect func(path string) bool
	New    func() Source
}

// registry holds the explicitly-registered sources, in registration order.
var registry []Registration

// Register adds a source. Call it once at wire-up (cli/main), never from init().
func Register(r Registration) { registry = append(registry, r) }

// Registered returns the registered sources in registration order. The returned
// slice is a copy — callers may not mutate the registry through it.
func Registered() []Registration {
	out := make([]Registration, len(registry))
	copy(out, registry)
	return out
}

// DetectID returns the ID of the first registered source whose Detect matches
// path, or "" if none do. Used to auto-attribute a path to its runtime.
func DetectID(path string) string {
	for _, r := range registry {
		if r.Detect != nil && r.Detect(path) {
			return r.ID
		}
	}
	return ""
}
