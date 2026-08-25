// Package source defines the ingest PORT: a small interface each runtime's
// transcript reader implements, so the index can ingest Claude Code, Codex, and
// future runtimes without knowing the on-disk format underneath. It is the
// ingest-side parallel to internal/embed's vector ports — defined ahead of its
// adapters, and consumed by the index, which never learns a source's format.
//
// Adapters live in subpackages (internal/source/claude, internal/source/codex)
// and import this package; this package imports none of them. The explicit
// internal/sources composition root wires adapters with Register — never via
// init()-time self-registration (implicit ordering, no error path, breaks test
// isolation).
package source

import (
	"strings"

	"github.com/MoonCaves/rawclaw/internal/model"
)

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

// ResetForTesting restores the registry slice to a specified state (for test isolation).
func ResetForTesting(regs []Registration) {
	registry = make([]Registration, len(regs))
	copy(registry, regs)
}

// Register adds a source. Call it once at explicit wire-up, never from init().
// If a source with the same ID already exists, it is replaced idempotently.
func Register(r Registration) {
	for i, existing := range registry {
		if existing.ID == r.ID {
			registry[i] = r
			return
		}
	}
	registry = append(registry, r)
}

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

// ResumeArgv returns the CLI argument vector to resume a session given its source
// tool and session ID.
func ResumeArgv(sourceTool, sessionID string) []string {
	switch sourceTool {
	case "codex":
		return []string{"codex", "resume", sessionID}
	case "antigravity":
		return []string{"agy", "--conversation", sessionID}
	case "goose":
		return []string{"goose", "session", "--resume", "--session-id", sessionID}
	case "claude":
		return []string{"claude", "--resume", sessionID}
	default:
		return []string{"claude", "--resume", sessionID}
	}
}

// shellSafe reports whether r can appear unquoted in a POSIX shell word without
// changing its meaning. This is an ALLOWLIST on purpose: a denylist of "dangerous"
// metacharacters is a standing invitation to miss one, and missing one here means
// emitting a command that silently runs somewhere else — or runs something else.
func shellSafe(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("/._-+:=,@%", r)
}

// shellQuote renders s as a single POSIX shell word. Single quotes are the only
// shell quoting that suppresses EVERY expansion — double quotes still perform
// parameter expansion ($HOME) and command substitution (`cmd`), so Go's
// strconv.Quote is the wrong tool here despite looking right: it emits Go string
// syntax, not shell syntax. A literal single quote cannot be escaped inside single
// quotes, so it is closed, backslash-escaped, and reopened — the standard four-byte
// idiom, spelled out in the test table rather than here because gofmt rewrites a
// doubled apostrophe in a doc comment into a typographic quote. Paths made only of
// shell-safe bytes are returned bare, keeping the common case copy-pasteable.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !shellSafe(r) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ResumeCommand formats the complete shell command line string to resume a session,
// optionally prefixing a `cd <cwd> &&` if cwd is non-empty. The cwd is shell-quoted:
// it comes out of a transcript, so it is untrusted input being pasted into a shell.
func ResumeCommand(sourceTool, sessionID, cwd string) string {
	cmd := strings.Join(ResumeArgv(sourceTool, sessionID), " ")
	if cwd != "" {
		return "cd " + shellQuote(cwd) + " && " + cmd
	}
	return cmd
}
