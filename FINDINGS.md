# Ponytail Review Findings

Fenced packages under review:
- `internal/parse/**`
- `internal/source/codex/**`
- `internal/source/antigravity/**`

## Behavior Regressions (Ruled FIX-RESTORE — Restored to 43cb92f Semantics)

The following changes were previously introduced under the guise of slimming but introduced semantic regressions. All seven have been restored to exact 43cb92f semantics:

1. **`internal/parse/parse.go` — Empty Thinking and Tool Result Marker Emission**
   - *Regression*: Dropping the two-pass filtering and adding `s != ""` guards caused empty `thinking` blocks (`[THINKING] `) and empty `tool_result` blocks (`[TOOL_RESULT] `) to be omitted completely rather than outputting marker records with trailing spaces.
   - *Fix-Restore*: Restored marker output for empty thinking and tool-result strings with non-empty filtering in `ExtractText`.

2. **`internal/parse/parse.go` — Tool Name Fallback Semantics**
   - *Regression*: Checking `name == ""` converted present-but-empty tool names (`name: ""`) into `[TOOL:?]`. The original 43cb92f semantics distinguish present empty names (`[TOOL:]`) from missing/non-string tool names (`[TOOL:?]`).
   - *Fix-Restore*: Restored type-assertion presence check so `[TOOL:]` is emitted for empty string names and `?` only when absent or non-string.

3. **`internal/parse/parse.go` — Six-ASCII-Byte Whitespace Collapsing**
   - *Regression*: Replacing `collapseSpaces` with `strings.Join(strings.Fields(s), " ")` collapsed Unicode whitespace characters (e.g., non-breaking space `\u00A0`, em-space `\u2003`), violating byte-exact transcript whitespace collapsing.
   - *Fix-Restore*: Restored the byte loop with the exact 6-ASCII-byte `isSpace` definition (`' '`, `'\t'`, `'\n'`, `'\r'`, `'\f'`, `'\v'`).

4. **`internal/parse/parse.go` — Slash-Command Delimiter Tokenization**
   - *Regression*: Replacing manual `IndexFunc` ASCII scanning in `IsLowSignal` with `strings.Fields` treated vertical tabs (`\v`) and other Unicode whitespace as word delimiters, falsely flagging substantive commands (such as `/clear\vargument`) as bare low-signal slash-commands.
   - *Fix-Restore*: Restored ASCII-only whitespace scanning (`' '`, `'\t'`, `'\n'`, `'\r'`) in `IsLowSignal`.

5. **`internal/source/codex/codex.go` — ContentText Block Array vs String Rejection**
   - *Regression*: Allowing `contentText` to accept plain strings caused message content strings to be parsed rather than ignored. In Codex, message records derive UUIDs from deterministic ordinals of accepted normalize items; indexing string messages shifted ordinals and caused durable identity corruption. In addition, map-shaped tool outputs with string content produced text rather than marker-only output.
   - *Fix-Restore*: Restored `contentText` to strictly reject non-array content, and restored separated `summaryText` for reasoning blocks.

6. **`internal/source/antigravity/antigravity.go` — Empty `<USER_REQUEST>` Tag Handling**
   - *Regression*: Changing `parseUserRequest` to use `tagContent` fallback returned the literal raw string `"<USER_REQUEST></USER_REQUEST>"` when the tag pair was empty, indexing raw tag syntax instead of skipping the empty request.
   - *Fix-Restore*: Restored `parseUserRequest` to return `""` when tags are present but empty, allowing `normalize` to properly skip the step.

7. **`internal/source/antigravity/antigravity.go` — Subagent Lineage Scan Detection & Parsing**
   - *Regression*: Removing the raw-literal `strings.Contains(line, "INVOKE_SUBAGENT")` guard and parsing `conversationId` using multi-step `strings.Cut` deviated from the line-scan structure and first-colon splitting behavior of 43cb92f.
   - *Fix-Restore*: Restored the raw-literal `INVOKE_SUBAGENT` check on transcript lines and first-colon splitting for `conversationId` extraction.

## Accepted Slimming & Refactoring Deviations

The following behavior-neutral improvements have been retained:

### `internal/parse`
- `internal/parse/parse.go:L48-57`: Inlined direct Go type assertions `.(string)` and `.(map[string]any)`, eliminating redundant single-line wrappers `asString` and `asMap`.
- `internal/parse/parse.go:L491-524`: Modernized `ISOToEpoch` using standard `time.RFC3339Nano` and `time.DateOnly` with space normalization, reducing redundant layout tables.
- `internal/parse/parse.go:L533-539`: Delegated `Disp` to `DispWith(content, includeTools, true, cap)`.

### `internal/source/codex`
- `internal/source/codex/meta.go:L63-73`: Inlined struct field assignments into a single `meta{...}` struct literal.
- `internal/source/codex/codex.go:L217-224`: Streamlined `actionQuery` map extraction with a single `if m, ok := v.(map[string]any); ok` guard.

### `internal/source/antigravity`
- `internal/source/antigravity/antigravity.go:L526-530`: Removed unreachable duplicate `sourceVal == "MODEL" || sourceVal == "SYSTEM"` condition in `normalize`.
- `internal/source/antigravity/antigravity.go:L298-305`: Extracted reusable `decodeArgsMap` helper shared between `inspectSessionHeaderAndSubagents` and `formatToolArgs`.
- `internal/source/antigravity/antigravity.go:L560-575`: Streamlined `formatToolArgs` by iterating over known argument keys.
- `internal/source/antigravity/antigravity.go:L362-417`: Extracted `tagContent` helper for `<subagent_reminder>` and `<user_information>` parsing and utilized `strings.Cut` for `->`.

## Cross-Package Opportunities (Report Only — Excluded From Current Fence)
- `internal/source/codex/codex_test.go` and `internal/source/antigravity/antigravity_test.go` define identical `writeJSONL` test helpers. A shared test fixture package under `internal/testutil` or `internal/source/testutil` would eliminate repeated fixture setup across all source adapters (`claude`, `codex`, `antigravity`, `goose`).
- `internal/parse/parse_test.go` and `internal/source/codex/codex_test.go` define identical `decode(t, json)` test helpers.
- `internal/parse/parse_test.go:L465-489` vs `internal/parse/parse_test.go:L579-631`: Repeated test harness structures (table test runner pattern across test files is report-only per supervisor directive).

## Scoring
- Total net change across fenced packages: slimming retained where behavior-neutral; all 7 behavioral regressions restored to exact 43cb92f semantics.
