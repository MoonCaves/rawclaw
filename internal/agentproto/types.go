// Package agentproto is the search→narrow→bounded-read protocol for LLM agents:
// an agent recalls prior conversations WITHOUT pasting whole transcripts — it
// gets ranked conversation refs, then reads BOUNDED excerpts on demand.
//
// Three verbs: Search (ranked refs), Read (bounded excerpt around a ref),
// Outline (a session's goal→resolution arc).
package agentproto

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/text"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// Protocol constants.
const (
	DefaultSearchLimit = 8    // top-N conversations to surface (matches the human --limit default)
	DefaultReadBudget  = 4000 // chars; the ceiling a bare --budget uses (no longer a default cap — reads are whole by default, #3)
	OutlineBookend     = 4    // messages each end for the arc summary
	ReadWindow         = 8    // ±messages around the anchor for Read
)

// readBookend is the number of bookend messages included at each end of the
// read window.
const readBookend = 3

// truncateMarker is appended to the last in-budget message when it overflows
// (note the leading space).
const truncateMarker = " …[truncated]"

// SearchRef is one ranked conversation ref. The ReadRef token
// ("<session8>:<uuid8>") is what an agent passes to Read.
type SearchRef struct {
	Project   string `json:"project"`
	SessionID string `json:"session_id"`
	ISO       string `json:"iso"`
	Snippet   string `json:"snippet"`
	ReadRef   string `json:"read_ref"`
	Topic     string `json:"topic,omitempty"`
	Last      string `json:"last,omitempty"`
	Missing   bool   `json:"missing,omitempty"`
	Routine   bool   `json:"routine,omitempty"`
}

// Scope status values for incompleteness-as-data (#6).
const (
	ScopeSearched        = "searched"       // indexed fresh + searched
	ScopeEmpty           = "empty"          // searched fresh, zero rows
	ScopeSkippedError    = "skipped_error"  // index/open failed — not searched
	ScopeStaleFallback   = "stale_fallback" // busy-lock → searched a possibly-stale cached index
	ScopeNotConsolidated = "not_consolidated"
)

// Store values for SearchEnvelope.Store — which index actually answered.
const (
	StoreConsolidated = "consolidated" // one database, one query
	StorePerProject   = "per-project"  // the fan-out: one database per project
)

// ScopeReport records how one project scope fared during a search, so an agent
// reads a partial result AS partial instead of mistaking it for complete (#6).
type ScopeReport struct {
	Project string `json:"project"`
	Dir     string `json:"dir"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
}

// Warning codes.
const (
	WarnRecencySkew         = "recency_skew"
	WarnBroadQuery          = "broad_query"
	WarnCurrentTurnExcluded = "current_turn_excluded"
	WarnScopeIncomplete     = "scope_incomplete"
	WarnNotConsolidated     = "not_consolidated"
	WarnStoreFallback       = "store_fallback"
	WarnProjectSpread       = "project_spread"
	WarnVectorGap           = "vector_gap"
	WarnRawHistory          = "raw_history"
	WarnIncludePathNoMatch  = "include_path_no_match"
)

// Warning is one advisory carried as data rather than prose.
type Warning struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Facts   map[string]any `json:"facts,omitempty"`
}

// VectorCoverage records candidate embedding coverage across the searched corpus.
type VectorCoverage struct {
	Ran           bool `json:"ran"`
	CandidateMsgs int  `json:"candidate_msgs"`
	VectoredMsgs  int  `json:"vectored_msgs"`
	MissingMsgs   int  `json:"missing_msgs"`
}

// SearchEnvelope wraps the ranked results with the per-scope completeness report.
type SearchEnvelope struct {
	Results             []SearchRef    `json:"results"`
	Scopes              []ScopeReport  `json:"scopes_report"`
	Complete            bool           `json:"complete"`
	Count               int            `json:"count"`
	TotalMatches        int            `json:"total_matches"`
	TotalIsLowerBound   bool           `json:"total_is_lower_bound,omitempty"`
	HasMore             bool           `json:"has_more"`
	NextCommand         string         `json:"next_command,omitempty"`
	VectorCoverage      VectorCoverage `json:"vector_coverage"`
	Warnings            []Warning      `json:"warnings,omitempty"`
	ExcludedCurrentTurn int            `json:"excluded_current_turn,omitempty"`
	Store               string         `json:"store,omitempty"`
	StoreNote           string         `json:"store_note,omitempty"`
}

// SubagentInfo is one child subagent session of a parent session.
type SubagentInfo struct {
	SessionID    string `json:"session_id"`
	MessageCount int    `json:"message_count"`
	Title        string `json:"title,omitempty"`
}

// ReadResult is a bounded excerpt around a ref.
type ReadResult struct {
	Project      string         `json:"project"`
	SessionID    string         `json:"session_id"`
	AnchorID     int            `json:"anchor_id"`
	FocusSnippet string         `json:"focus_snippet"`
	CharBudget   *int           `json:"char_budget"`
	ReadRef      string         `json:"read_ref"`
	Truncated    bool           `json:"truncated"`
	TrimmedChars int            `json:"trimmed_chars,omitempty"`
	TrimmedMsgs  int            `json:"trimmed_msgs,omitempty"`
	NextCommand  string         `json:"next_command,omitempty"`
	Subagents    []SubagentInfo `json:"subagents,omitempty"`
	*view.AnchoredView
}

// SearchOpts groups the optional search filters.
type SearchOpts struct {
	Limit            int
	Offset           int
	Role             string
	Sort             string
	IncludeTools     bool
	IncludeSubagents bool
	Since            string
	Before           string
	MinMessages      int
	IncludePath      string
	ExcludePath      string
	Oneline          bool
	CurrentSession   string
	Project          string
	Projects         []string
	ProjectDir       string
	Source           string
	ScopeFallback    func() []view.Scope
}

// fmtRef builds "<session8>:<uuid8>".
func fmtRef(sessionID, uuid string) string {
	return sid8(sessionID) + ":" + uuid8(uuid)
}

// sid8 delegates to text.Sid8.
func sid8(sessionID string) string {
	return text.Sid8(sessionID)
}

// uuid8 returns the first 8 hex chars of a message uuid.
func uuid8(uuid string) string {
	r := []rune(uuid)
	if len(r) > 8 {
		return string(r[:8])
	}
	return string(r)
}

var reNumericRef = regexp.MustCompile(`^[0-9]+$`)
var reHexPrefix = regexp.MustCompile(`^[0-9a-f]+$`)

func normalizeRefArg(ref string) string {
	return strings.TrimPrefix(ref, "ref=")
}

func normalizeSessionArg(s string) string {
	out := normalizeRefArg(s)
	if i := strings.IndexByte(out, ':'); i >= 0 {
		out = out[:i]
	}
	if out == "" {
		return s
	}
	return out
}

func NormalizeSessionArg(s string) string {
	return normalizeSessionArg(s)
}

func resolveRef(ref string) (string, string, error) {
	parts := strings.Split(normalizeRefArg(ref), ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("bad ref %q — expected <session8>:<uuid8> (e.g. a1b2c3d4:9f3e1c20)", ref)
	}
	session8, uuidPrefix := parts[0], strings.ToLower(parts[1])
	if uuidPrefix == "" {
		return "", "", fmt.Errorf("bad ref %q — expected <session8>:<uuid8> (e.g. a1b2c3d4:9f3e1c20)", ref)
	}
	if reNumericRef.MatchString(uuidPrefix) {
		return "", "", fmt.Errorf("ref %q looks like an old numeric ref; re-run search to get a uuid ref", ref)
	}
	if !reHexPrefix.MatchString(uuidPrefix) {
		return "", "", fmt.Errorf("bad ref %q — uuid8 must be hex [0-9a-f] (e.g. a1b2c3d4:9f3e1c20)", ref)
	}
	return session8, uuidPrefix, nil
}

func allScope() []view.Scope {
	return scopes.All(context.Background(), "", false)
}

type ScopeFn func() []view.Scope

func resolveScope(fn ScopeFn) []view.Scope {
	if fn == nil {
		return allScope()
	}
	return fn()
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

func runeSlice(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if n >= len(r) {
		return s
	}
	return string(r[:n])
}

func emit(w io.Writer, obj any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(obj); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
