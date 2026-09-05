# Steal code, verbatim (2026-09-05)

Exact source pulled through Sourcegraph, unmodified except where a line says `// rawclaw:`.
Each block names the origin file and the RawClaw file it replaces or joins. Licenses are recorded as facts only.
For the full fork-by-fork reference map see `decision-references.md`.

## 1. Twin FTS5 tables — ccrider `internal/core/db/schema.go` L86–122 (MIT)

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
	text_content, content=messages, content_rowid=id, tokenize='porter unicode61'
);
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts_code USING fts5(
	text_content, content=messages, content_rowid=id, tokenize='unicode61'
);
CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
	INSERT INTO messages_fts(rowid, text_content) VALUES (new.id, new.text_content);
	INSERT INTO messages_fts_code(rowid, text_content) VALUES (new.id, new.text_content);
END;
CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
	INSERT INTO messages_fts(messages_fts, rowid, text_content) VALUES ('delete', old.id, old.text_content);
	INSERT INTO messages_fts_code(messages_fts_code, rowid, text_content) VALUES ('delete', old.id, old.text_content);
END;
CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
	INSERT INTO messages_fts(messages_fts, rowid, text_content) VALUES ('delete', old.id, old.text_content);
	INSERT INTO messages_fts(rowid, text_content) VALUES (new.id, new.text_content);
	INSERT INTO messages_fts_code(messages_fts_code, rowid, text_content) VALUES ('delete', old.id, old.text_content);
	INSERT INTO messages_fts_code(rowid, text_content) VALUES (new.id, new.text_content);
END;
-- backfill an existing store:
INSERT INTO messages_fts_code(messages_fts_code) VALUES('rebuild');
```

## 2. Code-aware tokenizer + router — CASS `tests/pages_fts.rs` L174, `src/pages/fts.rs` L84–150

```sql
tokenize="unicode61 tokenchars '-_./:@#$%\\'"
```

```rust
pub fn detect_search_mode(query: &str) -> Fts5SearchMode {
    let has_code_chars = query.contains('_') || query.contains('.') || query.contains('/')
        || query.contains('\\') || query.contains("::") || query.contains('#')
        || query.contains('@') || query.contains('$') || query.contains('%');
    let has_code_patterns = has_camel_case(query) || has_kebab_case(query);
    let is_code_query = has_code_chars || has_code_patterns;
    let words: Vec<&str> = query.split_whitespace().collect();
    let lower = query.to_lowercase();
    let has_prose_indicators = words.len() > 3
        || lower.starts_with("how ") || lower.starts_with("what ") || lower.starts_with("why ")
        || lower.starts_with("when ") || lower.starts_with("where ")
        || lower.contains(" the ") || lower.contains(" is ") || lower.contains(" are ")
        || lower.contains(" was ") || lower.contains(" were ");
    if is_code_query && !has_prose_indicators { Fts5SearchMode::Code }
    else if has_prose_indicators && !is_code_query { Fts5SearchMode::NaturalLanguage }
    else if is_code_query { Fts5SearchMode::Code }
    else { Fts5SearchMode::NaturalLanguage }
}
fn has_kebab_case(s: &str) -> bool {
    let c: Vec<char> = s.chars().collect();
    (2..c.len()).any(|i| c[i-1] == '-' && c[i-2].is_alphabetic() && c[i].is_alphabetic())
}
fn has_camel_case(s: &str) -> bool {
    let c: Vec<char> = s.chars().collect();
    (1..c.len()).any(|i| c[i-1].is_lowercase() && c[i].is_uppercase())
}
```

## 3. Query sanitizer — zk `internal/util/fts5/fts5.go` L6–104 (GPL-3)

```go
func ConvertQuery(query string) string {
	var out strings.Builder
	passthroughTokens := map[string]bool{"AND": true, "OR": true, "NOT": true}
	termSeparators := map[rune]bool{' ': true, '\t': true, '\n': true, '(': true, ')': true}
	inQuote := false
	term := ""
	closeTerm := func() {
		if term == "" {
			return
		}
		if !inQuote && passthroughTokens[term] {
			out.WriteString(term)
		} else {
			isPrefixToken := !inQuote && strings.HasSuffix(term, "*")
			if isPrefixToken {
				term = strings.TrimSuffix(term, "*")
			}
			out.WriteString(`"` + term + `"`)
			if isPrefixToken {
				out.WriteString("*")
			}
		}
		term = ""
	}
	for _, c := range query {
		switch {
		case c == '"':
			if inQuote {
				closeTerm()
			}
			inQuote = !inQuote
		case term == "" && (c == '^' || c == '*'):
			out.WriteString(string(c))
		case !inQuote && c == ':':
			out.WriteString(term + string(c))
			term = ""
		case c == '-' && term == "":
			out.WriteString(" NOT ")
		case !inQuote && c == '|':
			closeTerm()
			out.WriteString(" OR ")
		case !inQuote && c == '+' && term == "":
			break
		case !inQuote && termSeparators[c]:
			closeTerm()
			out.WriteString(string(c))
		default:
			term += string(c)
		}
	}
	closeTerm()
	return out.String()
}
```

`// rawclaw:` the `col:` branch conflicts with `std::io` style tokens on the tokenchars table; skip it when routing to the exact table.

## 4. Exact-word boost in one bm25 query — navidrome `persistence/sql_search_fts.go` L32–43, L113–188 (GPL-3)

```go
var fts5SpecialChars = regexp.MustCompile(`[^\p{L}\p{N}\s*"\x00]`)
var fts5Operators = regexp.MustCompile(`(?i)\b(AND|OR|NOT|NEAR)\b`)
var fts5LeadingStar = regexp.MustCompile(`(^|[\s])\*+`)

func buildFTS5Query(userInput string) (string, bool) {
	q := strings.TrimSpace(userInput)
	if q == "" || q == `""` {
		return "", false
	}
	var phrases []string
	result := q
	for {
		start := strings.Index(result, `"`)
		if start == -1 {
			break
		}
		end := strings.Index(result[start+1:], `"`)
		if end == -1 {
			result = result[:start] + result[start+1:]
			break
		}
		end += start + 1
		phrases = append(phrases, result[start:end+1])
		result = result[:start] + fmt.Sprintf("\x00PHRASE%d\x00", len(phrases)-1) + result[end+1:]
	}
	result = fts5Operators.ReplaceAllStringFunc(result, strings.ToLower)
	result = fts5SpecialChars.ReplaceAllString(result, " ")
	result = fts5LeadingStar.ReplaceAllString(result, "$1")
	tokens := strings.Fields(result)
	prefixTokens := make([]string, len(tokens))
	wrappedTokens := make([]string, len(tokens))
	for i, t := range tokens {
		if strings.HasPrefix(t, "\x00") || strings.HasSuffix(t, "*") {
			prefixTokens[i], wrappedTokens[i] = t, t
			continue
		}
		prefixTokens[i] = t + "*"
		wrappedTokens[i] = "(" + t + " OR " + t + "*)"
	}
	prefixQuery := strings.Join(prefixTokens, " ")
	result = strings.Join(wrappedTokens, " AND ")
	for i, phrase := range phrases {
		placeholder := fmt.Sprintf("\x00PHRASE%d\x00", i)
		prefixQuery = strings.ReplaceAll(prefixQuery, placeholder, phrase)
		result = strings.ReplaceAll(result, placeholder, phrase)
	}
	return result, ftsQueryDegraded(userInput, prefixQuery)
}
```

`// rawclaw:` original also runs `sanitize.Accents` and `processPunctuatedWords` (L56–89) between phrase extraction and operator lowering.

## 5. RRF over N lists — grepai `search/hybrid.go` L57–89 (MIT)

```go
func ReciprocalRankFusion(k float32, limit int, lists ...[]store.SearchResult) []store.SearchResult {
	scores := make(map[string]float32)
	chunkMap := make(map[string]store.Chunk)
	for _, list := range lists {
		for rank, result := range list {
			id := result.Chunk.ID
			scores[id] += 1.0 / (k + float32(rank) + 1)
			chunkMap[id] = result.Chunk
		}
	}
	results := make([]store.SearchResult, 0, len(scores))
	for id, score := range scores {
		results = append(results, store.SearchResult{Chunk: chunkMap[id], Score: score})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}
```

`// rawclaw:` key by message uuid; add a secondary sort key so equal scores are deterministic.

## 6. Table choice as a flag — calibre `src/calibre/db/fts/connect.py` L164–165, L193–195 (GPL-3)

```python
fts_table = 'books_fts' + ('_stemmed' if use_stemming else '')
query += f' "{fts_table}" MATCH ?'
query += f' ORDER BY {fts_table}.rank '
```

## 7. Rotation / truncation / growth — hpcloud/tail `watch/polling.go` L83–109 (MIT)

```go
if !os.SameFile(origFi, fi) {
	changes.NotifyDeleted()
	return
}
fw.Size = fi.Size()
if prevSize > 0 && prevSize > fw.Size {
	changes.NotifyTruncated()
	prevSize = fw.Size
	continue
}
if prevSize > 0 && prevSize < fw.Size {
	changes.NotifyModified()
	prevSize = fw.Size
	continue
}
prevSize = fw.Size
modTime := fi.ModTime()
if modTime != prevModTime {
	prevModTime = modTime
	changes.NotifyModified()
}
```

## 8. Retry policy — grepai `embedder/retry.go` (MIT)

```go
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{BaseDelay: time.Second, Multiplier: 2.0, MaxDelay: 32 * time.Second, MaxAttempts: 5}
}
func (p RetryPolicy) Calculate(attempt int) time.Duration {
	delay := float64(p.BaseDelay) * math.Pow(p.Multiplier, float64(attempt))
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	return time.Duration(delay + rand.Float64()*delay)
}
func IsRetryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || (statusCode >= 500 && statusCode < 600)
}
```

## 9. gh JSON flag trio — cli/cli `pkg/cmdutil/json_flags.go` L1–90 (MIT)

```go
func AddJSONFlags(cmd *cobra.Command, exportTarget *Exporter, fields []string) {
	f := cmd.Flags()
	addJsonFlag(f)
	addJqFlag(f, "q")
	addTemplateFlag(f, "t")
	setupJsonFlags(cmd, exportTarget, fields)
}
func addJsonFlag(f *pflag.FlagSet) {
	f.StringSlice("json", nil, "Output JSON with the specified `fields`")
}
func addJqFlag(f *pflag.FlagSet, shorthand string) {
	f.StringP("jq", shorthand, "", "Filter JSON output using a jq `expression`")
}
func addTemplateFlag(f *pflag.FlagSet, shorthand string) {
	f.StringP("template", shorthand, "", "Format JSON output using a Go template; see \"gh help formatting\"")
}
```

Engines: `github.com/cli/go-gh/v2/pkg/jq`, `github.com/cli/go-gh/v2/pkg/template`.

## 10. Sanitizer alternative with tests — ccrider `internal/core/search/search.go` L356–425 (MIT)

```go
func escapeFTS5Query(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return query
	}
	if strings.HasPrefix(query, "\"") && strings.HasSuffix(query, "\"") && len(query) > 2 {
		inner := query[1 : len(query)-1]
		return "\"" + strings.ReplaceAll(inner, "\"", "\"\"") + "\""
	}
	tokens := strings.Fields(query)
	var escaped []string
	for _, token := range tokens {
		hasWildcard := strings.HasSuffix(token, "*")
		if hasWildcard {
			token = token[:len(token)-1]
		}
		token = strings.ReplaceAll(token, "\"", "\"\"")
		if hasWildcard {
			escaped = append(escaped, "\""+token+"\"*")
		} else {
			escaped = append(escaped, "\""+token+"\"")
		}
	}
	return strings.Join(escaped, " ")
}
```

## Order of paste

1. Block 1 with block 2's tokenizer line; additive migration; `'rebuild'` on the new table only.
2. Block 3 or 10, tests included.
3. Block 2 router in Go plus block 6 as `--exact`.
4. Ranking, measured three ways: exact-first fallback; block 5 RRF over both tables; block 4's `(t OR t*)` on the exact table alone.
5. Five queries, all modes, copy of the store, numbers in the handoff.
6. Blocks 7, 8, 9 are independent lanes.
