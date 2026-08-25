# Ponytail Review Findings

Fenced packages under review:
- `internal/parse/**`
- `internal/source/codex/**`
- `internal/source/antigravity/**`

## Findings

### `internal/parse`
- `internal/parse/parse.go:L48-57`: `yagni`: Single-line wrappers `asString` and `asMap`. Inline native Go type assertions `.(string)` and `.(map[string]any)`.
- `internal/parse/parse.go:L109-116`: `shrink`: Two-pass slice filtering for `parts` and `nonEmpty`. Only append non-empty parts in `appendBlock`/`appendToolResult`, eliminating the second allocation and loop.
- `internal/parse/parse.go:L308-324`: `stdlib`: 17-line hand-rolled `collapseSpaces` loop and state machine. `strings.Join(strings.Fields(s), " ")`.
- `internal/parse/parse.go:L372-379`: `stdlib`: Manual rune scanner in `IsLowSignal` to extract first slash command word. `strings.Fields(lower)[0]`.
- `internal/parse/parse.go:L491-524`: `stdlib`: 34-line multi-layout timestamp parser with redundant layout tables. Standard `time.RFC3339Nano`, `time.DateTime`, and `time.DateOnly` with space normalization.
- `internal/parse/parse.go:L533-539`: `shrink`: Hand-rolled logic in `Disp` duplicating `DispWith`. `DispWith(content, includeTools, true, cap)`.

### `internal/source/codex`
- `internal/source/codex/meta.go:L63-73`: `shrink`: Multi-step struct field assignments on `m := meta{}` in `readMeta`. Inline into struct literal.
- `internal/source/codex/codex.go:L217-224`: `shrink`: Verbose `actionQuery` map check. `if m, ok := v.(map[string]any); ok { q, _ := m["query"].(string); return q }`.
- `internal/source/codex/codex.go:L264-293`: `shrink`: Dupl clone site between `contentText` block walking and `summaryText`/`outputText`. Unify text block extraction.

### `internal/source/antigravity`
- `internal/source/antigravity/antigravity.go:L526-530`: `delete`: Redundant unreachable `if sourceVal == "MODEL" || sourceVal == "SYSTEM"` checking the same empty `content`. Replacement: nothing.
- `internal/source/antigravity/antigravity.go:L560-575`: `shrink`: 15 lines of repetitive `if` statements in `formatToolArgs`. Loop over key slice `[]string{"CommandLine", "query", "Query", "AbsolutePath", "TargetFile"}`.
- `internal/source/antigravity/antigravity.go:L362-417`: `shrink`: Triplicated tag extraction (`<subagent_reminder>`, `<user_information>`, `<USER_REQUEST>`). Extract small `tagContent(s, startTag, endTag string) string` helper and use `strings.Cut` for `->`.
- `internal/source/antigravity/antigravity.go:L280-347`: `shrink`: Duplicate JSON unmarshaling of transcript lines and manual colon splitting in `inspectSessionHeaderAndSubagents`. Single unmarshal and `strings.Cut`.
- `internal/source/antigravity/antigravity.go:L298-305`: `shrink`: Dupl clone site between `formatToolArgs` string/map decoding and `inspectSessionHeaderAndSubagents` tool args extraction.

## Cross-Package Opportunities (Report Only — Excluded From Current Fence)
- `internal/source/codex/codex_test.go` and `internal/source/antigravity/antigravity_test.go` define identical `writeJSONL` test helpers. A shared test fixture package under `internal/testutil` or `internal/source/testutil` would eliminate repeated fixture setup across all source adapters (`claude`, `codex`, `antigravity`, `goose`).
- `internal/parse/parse_test.go` and `internal/source/codex/codex_test.go` define identical `decode(t, json)` test helpers.
- `internal/parse/parse_test.go:L465-489` vs `internal/parse/parse_test.go:L579-631`: Repeated test harness structures (table test runner pattern across test files is report-only per supervisor directive).

## Scoring
net: -125 lines possible.
