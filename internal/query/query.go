// Package query holds pure query-layer string functions: term parsing, FTS5
// query sanitizing, stopword stripping, boolean-operator translation, and the
// path / min-messages predicates. No dependencies beyond the standard library.
//
// The FTS5 sanitizer neutralizes SQLite FTS5 operators so a plain-English query
// can't accidentally invoke FTS5 syntax.
package query

import (
	"regexp"
	"strings"
)

// Stopwords are common words + FTS5 boolean keywords dropped from natural-
// language queries. A set for O(1) membership.
var Stopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "of": {}, "to": {}, "in": {}, "on": {},
	"for": {}, "and": {}, "or": {}, "not": {}, "near": {}, "is": {}, "are": {},
	"was": {}, "were": {}, "be": {}, "it": {}, "its": {}, "this": {}, "that": {},
	"with": {}, "as": {}, "at": {}, "by": {}, "we": {}, "i": {}, "you": {},
	"do": {}, "did": {}, "what": {}, "where": {}, "when": {}, "how": {}, "our": {},
	"your": {}, "from": {}, "about": {},
}

// sentinel is the NUL byte ("\x00") used to stand in for a protected quoted
// phrase while structural transforms run. One sentinel == one phrase.
const sentinel = "\x00"

// Precompiled patterns used across the query transforms.
var (
	reQuotedNonEmpty = regexp.MustCompile(`"([^"]+)"`)                 // phrase with non-empty body
	reQuotedAny      = regexp.MustCompile(`"[^"]*"`)                   // any quoted run (may be empty)
	reFTS5Structural = regexp.MustCompile(`[+(){}"^]`)                 // FTS5 structural characters
	reLeadStar       = regexp.MustCompile(`(^|\s)\*`)                  // leading bare prefix-star
	reRunStar        = regexp.MustCompile(`\*{2,}`)                    // runaway star run
	reLeadBool       = regexp.MustCompile(`(?i)^\s*(AND|OR|NOT)\b\s*`) // leading boolean keyword
	reTrailBool      = regexp.MustCompile(`(?i)\s+(AND|OR|NOT)\s*$`)   // trailing boolean keyword
	reDottedID       = regexp.MustCompile(`\b(\w+(?:[._-]\w+)+)\b`)    // dotted/hyphenated identifier
	// reProtect matches, in textual order, the runs SanitizeFTS5Query sets aside
	// as one sentinel each: a quoted run (may be empty) OR a path-like token (one
	// containing '/', e.g. ~/.claude/projects). Path tokens stop at whitespace and
	// the NUL sentinel so a path can't swallow an already-protected phrase.
	reProtect    = regexp.MustCompile(`"[^"]*"|[^\s\x00]*/[^\s\x00]*`)
	reWhitespace = regexp.MustCompile(`\s+`)           // snippet whitespace collapse
	reBoolAnd    = regexp.MustCompile(`\s*&&\s*`)      // && operator
	reBoolOr     = regexp.MustCompile(`\s*\|\|\s*`)    // || operator
	reInfixNot   = regexp.MustCompile(`\S\s+NOT\s+\S`) // an infix, uppercase NOT (FTS5 only treats uppercase as the operator; excludes a leading/trailing-only NOT)
)

// ParseTerms splits a query into lowercased phrases (quoted) + bare terms,
// dropping AND/OR/NOT boolean keywords and trailing '*' wildcards.
func ParseTerms(q string) []string {
	// Quoted phrases first: lowercased, only if non-empty after trimming.
	phrases := []string{}
	for _, m := range reQuotedNonEmpty.FindAllStringSubmatch(q, -1) {
		body := m[1]
		if strings.TrimSpace(body) != "" {
			phrases = append(phrases, strings.ToLower(body))
		}
	}

	// Remainder: quoted runs replaced by a space, then split on whitespace.
	rest := reQuotedNonEmpty.ReplaceAllString(q, " ")
	bares := []string{}
	for _, t := range strings.Fields(rest) {
		if isBoolKeyword(t) {
			continue
		}
		bares = append(bares, strings.TrimRight(strings.ToLower(t), "*"))
	}

	// phrases + bares, dropping any empty result.
	out := make([]string, 0, len(phrases)+len(bares))
	for _, t := range phrases {
		if t != "" {
			out = append(out, t)
		}
	}
	for _, t := range bares {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// MakeSnippet rebuilds a human-readable snippet from matched (tool-stripped)
// text, highlighting `terms` with >>>...<<<. Returns ("", false) if no term is
// present in the human text (the match was tool-only) — the bool reports
// presence.
func MakeSnippet(text string, terms []string) (snippet string, ok bool) {
	if text == "" {
		return "", false
	}
	low := strings.ToLower(text)

	// pos = the minimum match offset over terms, considering only present
	// non-empty terms. strings.Index returns a byte index in the lowercased
	// string; because ToLower can change byte length for some runes, we slice
	// `text` against `low` carefully below. For ASCII (the corpus norm) they
	// align.
	pos := -1
	for _, t := range terms {
		if t == "" {
			continue
		}
		i := strings.Index(low, t)
		if i < 0 {
			continue
		}
		if pos < 0 || i < pos {
			pos = i
		}
	}
	if pos < 0 {
		return "", false
	}

	// Window: text[max(0,pos-90) : pos+170], indexed by code point against the
	// rune slice of the ORIGINAL text. `pos` (a byte index into `low`) is
	// converted to a rune index for slicing.
	textRunes := []rune(text)
	posRune := runeIndexAtByte(low, pos)
	s := posRune - 90
	if s < 0 {
		s = 0
	}
	end := posRune + 170
	if end > len(textRunes) {
		end = len(textRunes)
	}
	window := string(textRunes[s:end])

	// Highlight each unique term, longest first (so a longer term wins over a
	// substring), case-insensitively.
	for _, t := range uniqueByLenDesc(terms) {
		if t == "" {
			continue
		}
		re := regexp.MustCompile(`(?i)(` + regexp.QuoteMeta(t) + `)`)
		window = re.ReplaceAllString(window, ">>>$1<<<")
	}

	return strings.TrimSpace(reWhitespace.ReplaceAllString(window, " ")), true
}

// SanitizeFTS5Query neutralizes FTS5 syntax hazards in a plain-English query:
// protects quoted phrases, drops structural chars, tames wildcards, strips a
// leading/trailing boolean keyword, and quotes path-like / dotted-hyphenated
// identifiers so FTS5 keeps each as one searchable phrase.
func SanitizeFTS5Query(q string) string {
	// 1) Set aside (in textual order) each protected run — a quoted phrase, or a
	// path-like token with a '/'. Each becomes one sentinel byte; a path is
	// quoted so FTS5 reads it as an adjacency phrase of its tokens (which matches
	// the path) instead of treating '/' as a token separator and dropping it.
	phrases := make([]string, 0)
	s := reProtect.ReplaceAllStringFunc(q, func(run string) string {
		if strings.HasPrefix(run, `"`) {
			phrases = append(phrases, run) // already-quoted: kept verbatim
		} else {
			phrases = append(phrases, `"`+strings.ReplaceAll(run, `"`, "")+`"`) // path-like: quote it
		}
		return sentinel
	})

	// 2) Drop FTS5 structural characters.
	s = reFTS5Structural.ReplaceAllString(s, " ")

	// 3) Collapse runaway '*' …
	s = reRunStar.ReplaceAllString(s, "*")
	// … and remove a bare leading prefix-'*' (illegal in FTS5).
	s = reLeadStar.ReplaceAllString(s, "$1")

	// 4) A boolean keyword that leads … or trails is meaningless — strip it.
	s = reLeadBool.ReplaceAllString(strings.TrimSpace(s), "")
	s = reTrailBool.ReplaceAllString(strings.TrimSpace(s), "")

	// 5) Quote session_id / a.b.c so FTS5 keeps them one token.
	s = reDottedID.ReplaceAllString(s, `"$1"`)

	// 6) Restore the protected runs (quoted phrases + path-likes), in order.
	if len(phrases) > 0 {
		s = restorePhrases(s, phrases)
	}
	return strings.TrimSpace(s)
}

// StripStopwords drops Stopwords tokens from a query while preserving quoted
// phrases untouched. A quoted phrase may carry an attached prefix-'*' (e.g.
// SanitizeFTS5Query turns `self-update*` into `"self-update"*`), so the sentinel
// can be embedded in a token rather than standing alone — it is restored in
// place after the stopword filter, never dropped as a bare NUL.
func StripStopwords(q string) string {
	quoted := reQuotedAny.FindAllString(q, -1)
	holes := reQuotedAny.ReplaceAllString(q, sentinel)

	out := []string{}
	for _, t := range strings.Fields(holes) {
		// A token carrying a sentinel is a (possibly decorated) quoted phrase —
		// never a stopword. Keep it; the phrase is restored after the loop.
		if strings.Contains(t, sentinel) {
			out = append(out, t)
			continue
		}
		key := strings.Trim(strings.ToLower(t), "*")
		if _, isStop := Stopwords[key]; isStop {
			continue
		}
		out = append(out, t)
	}
	joined := strings.Join(out, " ")
	if len(quoted) > 0 {
		joined = restorePhrases(joined, quoted)
	}
	return strings.TrimSpace(joined)
}

// HasSearchableToken reports whether the query has any token that is not a bare
// AND/OR/NOT keyword.
func HasSearchableToken(q string) bool {
	for _, t := range strings.Fields(q) {
		if !isBoolKeyword(t) {
			return true
		}
	}
	return false
}

// BooleanToFTS5 translates human boolean operators (&&→AND, ||→OR, !term→NOT
// term, quoted phrases, parentheses) into an FTS5 MATCH expression.
//
// CRUCIAL INVARIANT: if `raw` contains NO boolean operators, the returned expr
// is EXACTLY SanitizeFTS5Query(raw) and usedOperators is false (plain queries
// stay byte-identical).
func BooleanToFTS5(raw string) (fts5Expr string, usedOperators bool) {
	if !hasBoolOps(raw) {
		// No boolean operators → plain path, byte-identical to sanitize.
		return SanitizeFTS5Query(raw), false
	}

	// Protect quoted phrases so operators inside them aren't mangled.
	phrases := reQuotedAny.FindAllString(raw, -1)
	s := reQuotedAny.ReplaceAllString(raw, sentinel)

	// Translate operators on the non-phrase remainder.
	s = reBoolAnd.ReplaceAllString(s, " AND ")
	s = reBoolOr.ReplaceAllString(s, " OR ")
	s = subBoolNot(s)

	// Restore quoted phrases.
	if len(phrases) > 0 {
		s = restorePhrases(s, phrases)
	}

	// Neutralize residual FTS5 hazards while keeping inserted AND/OR/NOT.
	return SanitizeFTS5Query(s), true
}

// PathPredicate returns a predicate filtering a transcript cwd string against
// optional include/exclude regexes. A bad regex degrades to literal substring
// containment (never panics). Pass "" for include or exclude to skip that
// filter.
func PathPredicate(include, exclude string) func(cwd string) bool {
	// compile returns (compiled, literalFallback, present). An empty pattern
	// string here means "filter absent" (present=false).
	compile := func(pat string) (*regexp.Regexp, string, bool) {
		if pat == "" {
			return nil, "", false
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, pat, true // bad regex → literal substring fallback
		}
		return re, "", true
	}

	incRe, incLit, incOn := compile(include)
	excRe, excLit, excOn := compile(exclude)

	return func(cwd string) bool {
		s := cwd
		if incOn {
			if incRe != nil {
				if !incRe.MatchString(s) {
					return false
				}
			} else if !strings.Contains(s, incLit) {
				return false
			}
		}
		if excOn {
			if excRe != nil {
				if excRe.MatchString(s) {
					return false
				}
			} else if strings.Contains(s, excLit) {
				return false
			}
		}
		return true
	}
}

// MinMessagesOK reports whether messageCount >= minimum.
func MinMessagesOK(messageCount, minimum int) bool {
	return messageCount >= minimum
}

// ── internal helpers ─────────────────────────────────────────────────────────

// isBoolKeyword reports whether t (case-insensitively, via uppercase compare)
// is one of the bare FTS5 boolean keywords AND, OR, or NOT.
func isBoolKeyword(t string) bool {
	switch strings.ToUpper(t) {
	case "AND", "OR", "NOT":
		return true
	default:
		return false
	}
}

// restorePhrases walks s rune-by-rune, replacing each sentinel with the next
// protected phrase in order.
func restorePhrases(s string, phrases []string) string {
	var b strings.Builder
	i := 0
	for _, ch := range s {
		if ch == '\x00' && i < len(phrases) {
			b.WriteString(phrases[i])
			i++
			continue
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// uniqueByLenDesc dedups terms preserving uniqueness, then sorts by length
// descending. The only behavior the snippet relies on is longest-first, so
// ties are immaterial to the highlight result.
func uniqueByLenDesc(terms []string) []string {
	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(terms))
	for _, t := range terms {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		uniq = append(uniq, t)
	}
	// Stable insertion sort by rune-length descending (counting code points,
	// not bytes).
	for i := 1; i < len(uniq); i++ {
		for j := i; j > 0 && runeLen(uniq[j]) > runeLen(uniq[j-1]); j-- {
			uniq[j], uniq[j-1] = uniq[j-1], uniq[j]
		}
	}
	return uniq
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// runeIndexAtByte converts a byte offset into s to a rune (code-point) index.
func runeIndexAtByte(s string, byteOffset int) int {
	idx := 0
	for b := range s {
		if b >= byteOffset {
			return idx
		}
		idx++
	}
	return idx
}

// hasBoolOps reports whether s contains a boolean operator: "&&", "||", or a
// '!' that is NOT preceded by a word char (\w = [A-Za-z0-9_]). RE2 lacks
// lookbehind, so the `!` clause is checked manually.
func hasBoolOps(s string) bool {
	if strings.Contains(s, "&&") || strings.Contains(s, "||") {
		return true
	}
	// A spelled-out infix NOT ("deploy NOT staging") is exclusion, documented in
	// the search skill. Recognize it here so it routes through the boolean path
	// (a raw FTS5 expr) BEFORE StripStopwords — which would otherwise drop "not"
	// as a stopword and silently no-op the exclusion.
	if reInfixNot.MatchString(s) {
		return true
	}
	for i := 0; i < len(s); i++ {
		if s[i] != '!' {
			continue
		}
		if i == 0 || !isWordByte(s[i-1]) {
			return true
		}
	}
	return false
}

// subBoolNot rewrites negation shorthand: a '!' at a non-word boundary followed
// by one or more word chars becomes "NOT <word>". RE2 has no lookbehind, so the
// boundary is enforced manually.
func subBoolNot(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '!' && (i == 0 || !isWordByte(s[i-1])) {
			// Collect following \w+ (must be at least one to match).
			j := i + 1
			for j < len(s) && isWordByte(s[j]) {
				j++
			}
			if j > i+1 {
				b.WriteString("NOT ")
				b.WriteString(s[i+1 : j])
				i = j
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// isWordByte reports whether c is in \w = [A-Za-z0-9_] (ASCII). The
// boolean-operator inputs handled here are ASCII identifier text.
func isWordByte(c byte) bool {
	return c == '_' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9')
}
