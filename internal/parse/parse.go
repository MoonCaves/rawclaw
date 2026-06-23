// Package parse holds text-extraction and parse primitives — flattening one
// transcript JSON record into a searchable string, tool-stripping, role lookup,
// ISO-timestamp parsing, and display formatting.
package parse

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// Per-block / per-tool character ceilings.
const (
	BlockCap   = 8000 // per-block char ceiling (recall > leanness for a rare dig)
	ToolInCap  = 500
	ToolResCap = 1000
)

// IndexableTypes is the set of transcript record types that get indexed.
var IndexableTypes = []string{"user", "assistant", "summary", "system"}

// NoHumanMarker is the placeholder shown for a tool-only match.
const NoHumanMarker = "[tool-only match — use --include-tools to see it]"

// capRunes truncates s to at most n runes. The cap is rune-based, not
// byte-based, so multi-byte code points are never split.
func capRunes(s string, n int) string {
	if n < 0 {
		return s // negative = no cap (used to render a chosen/anchored message whole)
	}
	if len(s) <= n {
		// Fast path: byte length already within the rune cap.
		return s
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// asString returns (v, true) when v is a JSON string. Decoded JSON objects hold
// strings as the Go string type.
func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// asMap returns (v, true) when v is a decoded JSON object.
func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

// ExtractText flattens one decoded transcript record into a searchable string.
// Includes text, thinking, system, summary, and marker-tagged tool_use /
// tool_result so they are matchable. `obj` is a decoded JSON object.
func ExtractText(obj map[string]any) string {
	t, _ := asString(obj["type"])

	if t == "summary" {
		if s, ok := asString(obj["summary"]); ok {
			return "[SUMMARY] " + capRunes(s, BlockCap)
		}
	}
	if t == "system" {
		if s, ok := asString(obj["content"]); ok {
			return "[SYSTEM] " + capRunes(s, BlockCap)
		}
	}

	msg, ok := asMap(obj["message"])
	if !ok {
		// No message object: fall back to a top-level string content.
		if c, ok := asString(obj["content"]); ok {
			label := t
			if label == "" {
				label = "MISC"
			}
			return "[" + strings.ToUpper(label) + "] " + capRunes(c, BlockCap)
		}
		return ""
	}

	// message.content may be a plain string.
	if c, ok := asString(msg["content"]); ok {
		return capRunes(c, BlockCap)
	}

	// message.content may be a list of typed blocks.
	blocks, ok := msg["content"].([]any)
	if !ok {
		return ""
	}

	parts := make([]string, 0, len(blocks))
	for _, raw := range blocks {
		b, ok := asMap(raw)
		if !ok {
			continue
		}
		parts = appendBlock(parts, b)
	}

	// Drop empty parts, then join the rest on a single space.
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, " ")
}

// appendBlock appends the flattened text for one content block, dispatching on
// the block's type (text, thinking, tool_use, tool_result).
func appendBlock(parts []string, b map[string]any) []string {
	bt, _ := asString(b["type"])
	switch bt {
	case "text":
		if s, ok := asString(b["text"]); ok {
			parts = append(parts, capRunes(s, BlockCap))
		}
	case "thinking":
		if s, ok := asString(b["thinking"]); ok {
			parts = append(parts, "[THINKING] "+capRunes(s, BlockCap))
		}
	case "tool_use":
		name := "?"
		if n, ok := asString(b["name"]); ok {
			name = n
		}
		input := b["input"]
		if input == nil {
			input = map[string]any{}
		}
		parts = append(parts, "[TOOL:"+name+"] "+capRunes(jsonDump(input), ToolInCap))
	case "tool_result":
		parts = appendToolResult(parts, b["content"])
	}
	return parts
}

// appendToolResult flattens a tool_result block: a string content is tagged
// directly; a list yields one tagged part per object block that carries a text
// string.
func appendToolResult(parts []string, content any) []string {
	if s, ok := asString(content); ok {
		return append(parts, "[TOOL_RESULT] "+capRunes(s, ToolResCap))
	}
	list, ok := content.([]any)
	if !ok {
		return parts
	}
	for _, raw := range list {
		x, ok := asMap(raw)
		if !ok {
			continue
		}
		if s, ok := asString(x["text"]); ok {
			parts = append(parts, "[TOOL_RESULT] "+capRunes(s, ToolResCap))
		}
	}
	return parts
}

// jsonDump renders a decoded JSON value for the tool-input haystack with a space
// after every ':' and ',' separator. Go's encoder emits no separator spaces, so we
// re-insert them with a structural pass that skips string literals. Two properties
// are accepted for this haystack text (capped to ToolInCap, used only for
// matching): object keys come out sorted, and non-ASCII runes are kept as UTF-8
// rather than escaped to \uXXXX.
func jsonDump(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep <, >, & literal in the haystack
	if err := enc.Encode(v); err != nil {
		return ""
	}
	// Encoder appends a trailing newline; drop it.
	compact := strings.TrimRight(buf.String(), "\n")
	return respaceJSON(compact)
}

// respaceJSON inserts a space after structural ':' and ',' (outside string
// literals), producing ", " and ": " separators in the rendered JSON.
func respaceJSON(s string) string {
	var out strings.Builder
	out.Grow(len(s) + len(s)/8)
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		out.WriteByte(c)
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case ':', ',':
			out.WriteByte(' ')
		}
	}
	return out.String()
}

// Tool-marker boundary scanning:
//
// A run starts at "[TOOL" or "[TOOL_RESULT" immediately followed by ':' or ']',
// and extends (across newlines) until just before " [<UPPERCASE>" or end of
// string. We scan manually rather than with a regex so the until-boundary is a
// pure lookahead that does not consume the next marker.

// StripTools removes [TOOL...]/[TOOL_RESULT...] runs and collapses whitespace.
func StripTools(text string) string {
	stripped := removeToolRuns(text)
	return collapseSpaces(stripped)
}

// removeToolRuns deletes every [TOOL...]/[TOOL_RESULT...] run, matching each run
// non-greedily up to the next marker boundary.
func removeToolRuns(text string) string {
	var out strings.Builder
	i := 0
	for i < len(text) {
		start, hdrLen := toolMarkerAt(text, i)
		if start < 0 {
			out.WriteByte(text[i])
			i++
			continue
		}
		// Found a tool marker at i. Advance past the header, then consume until
		// the boundary " [<UPPERCASE>" or end of string.
		j := i + hdrLen
		end := toolRunEnd(text, j)
		i = end
	}
	return out.String()
}

// toolMarkerAt reports whether a tool marker begins at position i. It returns
// the start index (i if matched, -1 otherwise) and the header length consumed
// (the "[TOOL:" / "[TOOL]" / "[TOOL_RESULT:" / "[TOOL_RESULT]" prefix).
func toolMarkerAt(text string, i int) (int, int) {
	const (
		toolRes = "[TOOL_RESULT"
		tool    = "[TOOL"
	)
	// Test the longer prefix first so "[TOOL_RESULT" is not mis-matched as "[TOOL".
	if strings.HasPrefix(text[i:], toolRes) {
		if n := len(toolRes); i+n < len(text) && (text[i+n] == ':' || text[i+n] == ']') {
			return i, n + 1
		}
	}
	if strings.HasPrefix(text[i:], tool) {
		if n := len(tool); i+n < len(text) && (text[i+n] == ':' || text[i+n] == ']') {
			return i, n + 1
		}
	}
	return -1, 0
}

// toolRunEnd scans from j (just past a tool header) to the index of the boundary
// " [<UPPERCASE>" (whitespace + '[' + ASCII upper), or len(text) if none. The
// boundary char is NOT consumed, so the returned index points at the whitespace
// before the next bracketed marker.
func toolRunEnd(text string, j int) int {
	for k := j; k < len(text); k++ {
		if !isSpace(text[k]) {
			continue
		}
		// k is whitespace; need '[' then an uppercase ASCII letter after it.
		if k+2 < len(text) && text[k+1] == '[' && isUpper(text[k+2]) {
			return k
		}
	}
	return len(text)
}

// isSpace reports whether b is a whitespace byte that appears in transcript text:
// space, tab, newline, carriage return, form feed, or vertical tab.
func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}

func isUpper(b byte) bool { return b >= 'A' && b <= 'Z' }

// collapseSpaces replaces every run of whitespace with a single space and trims
// leading/trailing spaces.
func collapseSpaces(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	inSpace := false
	for i := 0; i < len(s); i++ {
		if isSpace(s[i]) {
			inSpace = true
			continue
		}
		if inSpace && out.Len() > 0 {
			out.WriteByte(' ')
		}
		inSpace = false
		out.WriteByte(s[i])
	}
	return out.String()
}

// TitleCap is the rune ceiling for a derived session title.
const TitleCap = 80

// titleFields are the raw record keys that carry a Claude-Code-native session
// title, checked in preference order before falling back to derivation. A
// "summary"-type record stores its recap text under "summary", so this set also
// captures native summaries.
var titleFields = []string{"ai_title", "summary", "custom_title", "title"}

// slashCommands is the set of bare slash-commands that count as low-signal when
// they are the WHOLE message (a warmup / control verb, not a substantive turn).
var slashCommands = map[string]struct{}{
	"/clear": {}, "/compact": {}, "/resume": {}, "/help": {},
	"/exit": {}, "/quit": {}, "/init": {}, "/cost": {}, "/review": {},
}

// greetings is the set of bare greeting/warmup tokens that count as low-signal
// when one of them is the WHOLE message (case-insensitive).
var greetings = map[string]struct{}{
	"hi": {}, "hey": {}, "hello": {}, "yo": {}, "sup": {},
	"thanks": {}, "thx": {}, "ok": {}, "okay": {}, "ty": {},
}

// IsLowSignal reports whether text is a low-signal message: one that should be
// skipped for previews and title derivation. Low-signal = empty/whitespace, a
// bare greeting/warmup token, a bare slash-command, a command-envelope markup
// line (<command-name>/<command-message>/<local-command-*>), or a pure
// tool/JSON/XML opener ('{', '[', '<'). The session is NEVER dropped on this
// predicate — only the individual message is filtered.
func IsLowSignal(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return true
	}

	lower := strings.ToLower(t)

	// Bare greeting/warmup: the WHOLE message is a single greeting token.
	if _, ok := greetings[lower]; ok {
		return true
	}

	// Bare slash-command: the first whitespace-delimited word is a known
	// control verb and nothing substantive follows beyond it.
	if t[0] == '/' {
		head := lower
		if i := strings.IndexFunc(lower, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '\r'
		}); i >= 0 {
			head = lower[:i]
		}
		if _, ok := slashCommands[head]; ok {
			return true
		}
	}

	// Command-envelope markup: Claude Code wraps slash invocations in
	// <command-name>…/<command-message>…/<local-command-stdout>… tags.
	if strings.HasPrefix(lower, "<command-name") ||
		strings.HasPrefix(lower, "<command-message") ||
		strings.HasPrefix(lower, "<command-args") ||
		strings.HasPrefix(lower, "<local-command-") {
		return true
	}

	// Pure tool/JSON/XML opener: a message that begins with a structural
	// bracket is machine markup, not a human turn worth previewing.
	switch t[0] {
	case '{', '[', '<':
		return true
	}

	return false
}

// IsSubstantive is the negation of IsLowSignal: true when text is a real human
// turn worth using for a title or preview.
func IsSubstantive(text string) bool {
	return !IsLowSignal(text)
}

// titleClean normalizes a candidate title to a single trimmed line capped at
// TitleCap runes: collapse all whitespace (so embedded newlines become spaces),
// trim, then rune-cap.
func titleClean(s string) string {
	return capRunes(collapseSpaces(s), TitleCap)
}

// DeriveTitle derives a single-line session title from a session's records, in
// preference order:
//
//  1. A record's own native title field (ai_title / summary / custom_title /
//     title) — the first record (in iteration order) that carries one wins.
//  2. The first user message whose flattened text IsSubstantive.
//
// It returns (title, true) when either path yields a non-empty title, else
// ("", false). The title is collapsed to one line, trimmed, and capped to
// ~TitleCap runes. records are decoded JSONL maps in transcript order.
func DeriveTitle(records []map[string]any) (string, bool) {
	// Pass 1: prefer a native title field carried on any record.
	for _, rec := range records {
		if rec == nil {
			continue
		}
		for _, key := range titleFields {
			if s, ok := asString(rec[key]); ok {
				if cleaned := titleClean(s); cleaned != "" {
					return cleaned, true
				}
			}
		}
	}

	// Pass 2: first substantive USER message.
	for _, rec := range records {
		if rec == nil {
			continue
		}
		if MsgRole(rec) != "user" {
			continue
		}
		text := ExtractText(rec)
		if IsSubstantive(text) {
			if cleaned := titleClean(text); cleaned != "" {
				return cleaned, true
			}
		}
	}

	return "", false
}

// MsgUUID returns the stable per-message identifier for a decoded record: the
// top-level "uuid" string when present and non-empty, else "leafUuid" (which
// "summary" records carry instead of their own uuid), else "". This id is
// Claude Code's own per-message handle — it survives reindex and append, so it
// anchors the external read-ref. A record with no uuid is still indexed and
// searchable; it is simply not returned as a primary read anchor.
func MsgUUID(obj map[string]any) string {
	if u, ok := asString(obj["uuid"]); ok && u != "" {
		return u
	}
	if u, ok := asString(obj["leafUuid"]); ok && u != "" {
		return u
	}
	return ""
}

// MsgRole returns the role for a decoded record: message.role when present and
// non-empty, otherwise the record type. An empty/missing role falls through to
// type; an empty string is the result when neither yields a non-empty value.
func MsgRole(obj map[string]any) string {
	if msg, ok := asMap(obj["message"]); ok {
		if role, ok := asString(msg["role"]); ok && role != "" {
			return role
		}
	}
	if t, ok := asString(obj["type"]); ok {
		return t
	}
	return ""
}

// ISOToEpoch parses an ISO-8601 string to epoch seconds, treating tz-naive as
// UTC; returns 0.0 on empty/invalid.
func ISOToEpoch(s string) float64 {
	if s == "" {
		return 0.0
	}
	// Normalize a trailing 'Z' (and any other 'Z') to an explicit +00:00 offset.
	normalized := strings.ReplaceAll(s, "Z", "+00:00")

	// Try offset-bearing layouts first, then naive (defaulted to UTC).
	withZone := []string{
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
	}
	for _, layout := range withZone {
		if dt, err := time.Parse(layout, normalized); err == nil {
			return epochSeconds(dt)
		}
	}

	naive := []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range naive {
		if dt, err := time.ParseInLocation(layout, normalized, time.UTC); err == nil {
			return epochSeconds(dt)
		}
	}
	return 0.0
}

// epochSeconds returns fractional Unix seconds for dt.
func epochSeconds(dt time.Time) float64 {
	return float64(dt.Unix()) + float64(dt.Nanosecond())/1e9
}

// Disp normalizes content for display: optionally tool-stripped, whitespace
// collapsed, capped to `cap` runes.
func Disp(content string, includeTools bool, cap int) string {
	t := content
	if !includeTools {
		t = StripTools(content)
	}
	return capRunes(collapseSpaces(t), cap)
}
