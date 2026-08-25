# Go Modernization Scan Report

**Date:** 2026-08-26  
**Target Codebase:** RawClaw (`github.com/MoonCaves/rawclaw`)  
**Toolchain Version:** Go 1.26.3 (`darwin/arm64`)  
**Module Directive:** `go 1.24.0` in `go.mod`  
**Linter:** `golangci-lint` v2.12.2 (`modernize` linter enabled)  
**Skill Guide:** the samber/cc-skills-golang `golang-modernize` skill  
**Ignore File (`.modernize`):** None present (clean scan)  
**Mode:** REPORT ONLY — zero `.go` files modified.

---

## Executive Summary

RawClaw is currently built with Go 1.24+ idioms in mind and already conforms to several key modernization patterns:
- All benchmark suites (`BenchmarkFTS5Search`, `BenchmarkCosineKNN`, `BenchmarkAntigravityDiscover`, `BenchmarkConnectionPragmas`, etc.) already use Go 1.24 `b.Loop()`.
- Zero loop variable shadow copies (`v := v`) exist in the codebase.
- No deprecated crypto algorithms (`crypto/elliptic`, `crypto/cipher.NewOFB`/`NewCFB`, `golang.org/x/crypto/sha3`) are imported.

However, significant modernization opportunities remain across the codebase:
1. **High Priority (Safety & Correctness):** 2 call sites using legacy `math/rand` instead of `math/rand/v2`, 14 call sites using legacy `errors.As` that can adopt Go 1.26 type-safe `errors.AsType[T]()`, and opportunities to encapsulate directory traversal with `os.OpenRoot` (Go 1.24+).
2. **Medium Priority (Readability & Maintainability):** 1 legacy `interface{}` instance, 6 manual min/max clamp patterns replaceable with built-in `min`/`max`, 38 standard 3-clause index loops replaceable with `range` over integer, 41 sorting and slice operations replaceable with standard `slices` / `cmp`, 4 environment fallback patterns replaceable with `cmp.Or`, 2 singleton initializations cleanly expressible via `sync.OnceValue`, 7 concurrent worker groups ready for `sync.WaitGroup.Go` (Go 1.25+), and 180+ test context call sites ready for `t.Context()` (Go 1.24+).
3. **Lower Priority (Gradual Improvement & Tooling):** 13 iterator-range modernizations (`strings.SplitSeq`, `strings.FieldsSeq`), 8 `strings.Cut`/`CutPrefix` simplifications, 5 `fmt.Appendf` zero-allocation byte slice formatters, and `go.mod` tool directive adoption.

---

## Migration Priority Scan Results

### 1. High Priority (Safety and Correctness)

#### 1.1 Remove Loop Variable Shadow Copies *(Go 1.22+)*
- **Status:** Clean. Zero instances of `v := v` shadow copies detected in the codebase.

#### 1.2 Replace `math/rand` with `math/rand/v2` *(Go 1.22+)*
- `internal/cli/cmd_ingest.go:8`: `import "math/rand"` &rarr; `import "math/rand/v2"`
- `internal/cli/cmd_ingest.go:288`: `+ time.Duration(rand.Intn(20))*time.Millisecond` &rarr; `+ time.Duration(rand.IntN(20))*time.Millisecond`
- `internal/semantic/semantic_bench_test.go:7`: `import "math/rand"` &rarr; `import "math/rand/v2"`
- `internal/semantic/semantic_bench_test.go:31`: `rng := rand.New(rand.NewSource(42))` &rarr; `rng := rand.New(rand.NewPCG(42, 0))`

#### 1.3 Use `os.Root` for User-Supplied File Paths *(Go 1.24+)*
- `internal/lifecycle/lifecycle.go:354`: `entries, err := os.ReadDir(root)` &rarr; `r, err := os.OpenRoot(root); if err == nil { defer r.Close(); entries, err = r.FS().ReadDir(".") }`
- `internal/paths/paths.go:327`: `entries, err := os.ReadDir(catDir)` &rarr; `r, err := os.OpenRoot(catDir); if err == nil { defer r.Close(); entries, err = r.FS().ReadDir(".") }`
- `internal/source/antigravity/antigravity.go:123`: `entries, err := os.ReadDir(brainDir)` &rarr; `r, err := os.OpenRoot(brainDir); if err == nil { defer r.Close(); entries, err = r.FS().ReadDir(".") }`

#### 1.4 Run `govulncheck` *(Go 1.22+)*
- **Status:** Not currently integrated into repo CI / Makefile. Recommend pinning `golang.org/x/vuln/cmd/govulncheck` in `go.mod` via a `tool` directive and running during CI audit checks.

#### 1.5 Use `errors.Is`/`errors.As` and `errors.AsType` *(Go 1.13+ / Go 1.26+)*
- Direct error comparisons (`== os.ErrNotExist`, etc.) are already using `errors.Is` throughout the codebase.
- The following `errors.As` call sites can be modernized to generic `errors.AsType[T]()` in Go 1.26+:
  - `cmd/rawclaw/main.go:22`: `var ee *cli.ExitError; if errors.As(err, &ee) {` &rarr; `if ee, ok := errors.AsType[*cli.ExitError](err); ok {`
  - `internal/agentproto/agentproto_test.go:640`: `var amb *AmbiguousSessionError; if !errors.As(err, &amb) {` &rarr; `if amb, ok := errors.AsType[*AmbiguousSessionError](err); !ok {`
  - `internal/agentproto/agentproto_test.go:660`: `var nf *NotFoundError; if !errors.As(err, &nf) {` &rarr; `if nf, ok := errors.AsType[*NotFoundError](err); !ok {`
  - `internal/agentproto/agentproto_test.go:761`: `var amb *AmbiguousSessionError; if !errors.As(err, &amb) {` &rarr; `if amb, ok := errors.AsType[*AmbiguousSessionError](err); !ok {`
  - `internal/agentproto/samesession_test.go:89`: `var amb *AmbiguousSessionError; if errors.As(err, &amb) {` &rarr; `if amb, ok := errors.AsType[*AmbiguousSessionError](err); ok {`
  - `internal/agentproto/samesession_test.go:142`: `var amb *AmbiguousSessionError; if !errors.As(err, &amb) {` &rarr; `if amb, ok := errors.AsType[*AmbiguousSessionError](err); !ok {`
  - `internal/agentproto/samesession_test.go:181`: `var nf *NotFoundError; if !errors.As(err, &nf) {` &rarr; `if nf, ok := errors.AsType[*NotFoundError](err); !ok {`
  - `internal/cli/apply.go:40`: `var re *remoteError; if err != nil && errors.As(err, &re) {` &rarr; `if re, ok := errors.AsType[*remoteError](err); err != nil && ok {`
  - `internal/cli/cmd_root_ux_test.go:375`: `var xe *ExitError; if !errors.As(err, &xe) || xe.Code != 2 {` &rarr; `if xe, ok := errors.AsType[*ExitError](err); !ok || xe.Code != 2 {`
  - `internal/cli/cmd_upgrade_test.go:537`: `var ee *ExitError; if !errors.As(err, &ee) || ee.Code != exitUpdateAvailable {` &rarr; `if ee, ok := errors.AsType[*ExitError](err); !ok || ee.Code != exitUpdateAvailable {`
  - `internal/cli/cmd_upgrade_test.go:568`: `var ee *ExitError; if !errors.As(err, &ee) || ee.Code != 2 {` &rarr; `if ee, ok := errors.AsType[*ExitError](err); !ok || ee.Code != 2 {`
  - `internal/cli/watchdog_child_test.go:41`: `var ee *exec.ExitError; if !errors.As(err, &ee) || ee.ExitCode() != 124 {` &rarr; `if ee, ok := errors.AsType[*exec.ExitError](err); !ok || ee.ExitCode() != 124 {`
  - `internal/live/client.go:154`: `var ee *exec.ExitError; if errors.As(err, &ee) {` &rarr; `if ee, ok := errors.AsType[*exec.ExitError](err); ok {`
  - `internal/live/client_test.go:182`: `var ee *exec.ExitError; if !errors.As(err, &ee) {` &rarr; `if ee, ok := errors.AsType[*exec.ExitError](err); !ok {`

#### 1.6 Migrate Deprecated Crypto Packages *(Go 1.24+)*
- **Status:** Clean. No deprecated `crypto/elliptic` curve functions, `crypto/cipher.NewOFB`/`NewCFB`, `golang.org/x/crypto/sha3`, `hkdf`, or `pbkdf2` packages are used.

---

### 2. Medium Priority (Readability and Maintainability)

#### 2.1 Replace `interface{}` with `any` *(Go 1.18+)*
- `internal/store/triage_test.go:139`: `origin interface{}` &rarr; `origin any`

#### 2.2 Use Built-in `min` and `max` *(Go 1.21+)*
- `internal/agentproto/agentproto.go:484`: `if fetch < 30 { fetch = 30 }` &rarr; `fetch = max(fetch, 30)`
- `internal/agentproto/agentproto.go:640`: `if wider < 20 { wider = 20 }` &rarr; `wider = max(wider, 20)`
- `internal/agentproto/agentproto.go:1679`: `if s < 0 { s = 0 }` &rarr; `s = max(s, 0)`
- `internal/live/session.go:170`: `if shown > ambiguousListCap { shown = ambiguousListCap }` &rarr; `shown = min(shown, ambiguousListCap)`
- `internal/query/query.go:129`: `if end > len(textRunes) { end = len(textRunes) }` &rarr; `end = min(end, len(textRunes))`
- `internal/retrieve/retrieve.go:379`: `if n > limit { n = limit }` &rarr; `n = min(n, limit)`

#### 2.3 Use `range` over Integers *(Go 1.22+)*
- `internal/agentproto/agentproto_scope_test.go:179`: `for i := 0; i < 5; i++ {` &rarr; `for i := range 5 {`
- `internal/agentproto/agentproto_store_test.go:23`: `for i := 0; i < nmsg; i++ {` &rarr; `for i := range nmsg {`
- `internal/agentproto/agentproto_store_test.go:317`: `for i := 0; i < 6; i++ {` &rarr; `for i := range 6 {`
- `internal/agentproto/agentproto_store_test.go:342`: `for i := 0; i < 6; i++ {` &rarr; `for i := range 6 {`
- `internal/agentproto/agentproto_test.go:816`: `for i := 0; i < n; i++ {` &rarr; `for i := range n {`
- `internal/agentproto/currentturn_test.go:240`: `for i := 0; i < deepMachineryTail; i++ {` &rarr; `for i := range deepMachineryTail {`
- `internal/cli/autosync_test.go:73`: `for i := 0; i < 2; i++ {` &rarr; `for range 2 {`
- `internal/cli/cmd_ingest.go:275`: `for attempt := 0; attempt < maxAttempts; attempt++ {` &rarr; `for attempt := range maxAttempts {`
- `internal/cli/cmd_ingest_test.go:307`: `for i := 0; i < numSessions; i++ {` &rarr; `for i := range numSessions {`
- `internal/cli/cmd_ingest_test.go:337`: `for i := 0; i < 16; i++ {` &rarr; `for i := range 16 {`
- `internal/cli/cmd_lifecycle_test.go:46`: `for i := 0; i < nLines; i++ {` &rarr; `for i := range nLines {`
- `internal/cli/cmd_upgrade.go:224`: `for i := 0; i < 3; i++ {` &rarr; `for range 3 {`
- `internal/index/incremental_test.go:270`: `for i := 0; i < 4; i++ {` &rarr; `for i := range 4 {`
- `internal/index/index_bench_test.go:36`: `for i := 0; i < sessionCount; i++ {` &rarr; `for i := range sessionCount {`
- `internal/index/tail_edge_test.go:690`: `for i := 0; i < 20; i++ {` &rarr; `for i := range 20 {`
- `internal/index/tail_edge_test.go:798`: `for i := 0; i < 5; i++ {` &rarr; `for i := range 5 {`
- `internal/index/tail_edge_test.go:836`: `for i := 0; i < 4; i++ {` &rarr; `for i := range 4 {`
- `internal/index/tail_edge_test.go:855`: `for i := 0; i < 3; i++ {` &rarr; `for i := range 3 {`
- `internal/index/tail_edge_test.go:1041`: `for i := 0; i < 3; i++ {` &rarr; `for range 3 {`
- `internal/lifecycle/floor_test.go:14`: `for i := 0; i < 20; i++ {` &rarr; `for i := range 20 {`
- `internal/live/session_test.go:257`: `for i := 0; i < 15; i++ {` &rarr; `for i := range 15 {`
- `internal/provenance/provenance_test.go:44`: `for i := 0; i < 40; i++ {` &rarr; `for range 40 {`
- `internal/provenance/provenance_test.go:45`: `for b := 0; b < 256; b++ {` &rarr; `for b := range 256 {`
- `internal/retrieve/retrieve.go:385`: `for i := 0; i < n; i++ {` &rarr; `for i := range n {`
- `internal/semantic/semantic.go:248`: `for i := 0; i < numWorkers; i++ {` &rarr; `for range numWorkers {`
- `internal/semantic/semantic_bench_test.go:47`: `for i := 0; i < tc.count; i++ {` &rarr; `for i := range tc.count {`
- `internal/semantic/semantic_test.go:548`: `for i := 0; i < n; i++ {` &rarr; `for i := range n {`
- `internal/semantic/semantic_test.go:724`: `for i := 0; i < 16; i++ {` &rarr; `for i := range 16 {`
- `internal/source/antigravity/antigravity_bench_test.go:19`: `for i := 0; i < 50; i++ {` &rarr; `for i := range 50 {`
- `internal/source/antigravity/antigravity_test.go:382`: `for i := 0; i < 24; i++ {` &rarr; `for i := range 24 {`
- `internal/source/antigravity/antigravity_test.go:393`: `for i := 0; i < 19; i++ {` &rarr; `for i := range 19 {`
- `internal/source/source_test.go:79`: `for g := 0; g < goroutines; g++ {` &rarr; `for g := range goroutines {`
- `internal/source/source_test.go:83`: `for i := 0; i < iterations; i++ {` &rarr; `for range iterations {`
- `internal/source/source_test.go:96`: `for i := 0; i < iterations; i++ {` &rarr; `for range iterations {`
- `internal/source/source_test.go:104`: `for i := 0; i < iterations; i++ {` &rarr; `for range iterations {`
- `internal/source/source_test.go:115`: `for i := 0; i < iterations; i++ {` &rarr; `for range iterations {`
- `internal/store/connect_bench_test.go:39`: `for i := 0; i < sessionCount; i++ {` &rarr; `for i := range sessionCount {`
- `internal/store/connect_test.go:96`: `for r := 0; r < numReaders; r++ {` &rarr; `for r := range numReaders {`
- `internal/store/connect_test.go:100`: `for j := 0; j < 50; j++ {` &rarr; `for range 50 {`

#### 2.4 Use `slices` and `maps` Packages *(Go 1.21+ / Go 1.23+)*
- **Sort and Slice Operations:**
  - `internal/agentproto/agentproto.go:1426`: `sort.SliceStable(cands, ...)` &rarr; `slices.SortStableFunc(cands, func(a, b retrieve.Anchor) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/agentproto/agentproto.go:1436`: `sort.SliceStable(cands, ...)` &rarr; `slices.SortStableFunc(cands, func(a, b retrieve.Anchor) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/agentproto/agentproto.go:1446`: `sort.SliceStable(cands, ...)` &rarr; `slices.SortStableFunc(cands, func(a, b retrieve.Anchor) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/agentproto/agentproto.go:2477`: `sort.SliceStable(hits, ...)` &rarr; `slices.SortStableFunc(hits, func(a, b view.AnchorScore) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/agentproto/agentproto.go:2584`: `sort.SliceStable(hits, ...)` &rarr; `slices.SortStableFunc(hits, func(a, b scoredSession) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/agentproto/agentproto.go:2647`: `sort.Strings(projects)` &rarr; `slices.Sort(projects)`
  - `internal/archive/scopes.go:262`: `sort.Strings(cwds)` &rarr; `slices.Sort(cwds)`
  - `internal/archive/scopes.go:342`: `sort.Strings(cwds)` &rarr; `slices.Sort(cwds)`
  - `internal/archive/tagconflict.go:38`: `sort.Strings(sorted)` &rarr; `slices.Sort(sorted)`
  - `internal/archive/tagingest.go:128`: `sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartUUID < sorted[j].StartUUID })` &rarr; `slices.SortFunc(sorted, func(a, b TagSegment) int { return cmp.Compare(a.StartUUID, b.StartUUID) })`
  - `internal/cli/cli.go:1446`: `sort.SliceStable(rows, ...)` &rarr; `slices.SortStableFunc(rows, func(a, b searchRow) int { return cmp.Compare(...) })`
  - `internal/cli/cli.go:1853`: `sort.SliceStable(rows, ...)` &rarr; `slices.SortStableFunc(rows, func(a, b topicRow) int { return cmp.Compare(...) })`
  - `internal/durable/durable.go:424`: `sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })` &rarr; `slices.SortFunc(out, func(a, b StoredSession) int { return cmp.Compare(a.ID, b.ID) })`
  - `internal/index/retained.go:64`: `sort.Strings(entries)` &rarr; `slices.Sort(entries)`
  - `internal/lifecycle/lifecycle.go:281`: `sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })` &rarr; `slices.SortFunc(out, func(a, b SessionCandidate) int { return cmp.Compare(a.Path, b.Path) })`
  - `internal/lifecycle/lifecycle.go:292`: `sort.Strings(files)` &rarr; `slices.Sort(files)`
  - `internal/lifecycle/lifecycle.go:372`: `sort.Strings(out)` &rarr; `slices.Sort(out)`
  - `internal/lifecycle/lifecycle_test.go:38`: `sort.Strings(out)` &rarr; `slices.Sort(out)`
  - `internal/live/serve.go:52`: `sort.SliceStable(rows, func(i, j int) bool { return rows[i].mtime.After(rows[j].mtime) })` &rarr; `slices.SortStableFunc(rows, func(a, b liveRow) int { return cmp.Compare(b.mtime.UnixNano(), a.mtime.UnixNano()) })`
  - `internal/paths/paths.go:150`: `sort.Strings(entries)` &rarr; `slices.Sort(entries)`
  - `internal/paths/paths.go:156`: `sort.Strings(files)` &rarr; `slices.Sort(files)`
  - `internal/paths/paths.go:276`: `sort.Strings(entries)` &rarr; `slices.Sort(entries)`
  - `internal/paths/paths.go:396`: `sort.Strings(files)` &rarr; `slices.Sort(files)`
  - `internal/paths/paths.go:453`: `sort.Strings(files)` &rarr; `slices.Sort(files)`
  - `internal/paths/paths.go:471`: `sort.Strings(out)` &rarr; `slices.Sort(out)`
  - `internal/retrieve/retrieve.go:479`: `sort.SliceStable(scored, ...)` &rarr; `slices.SortStableFunc(scored, func(a, b Anchor) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/retrieve/retrieve.go:585`: `sort.SliceStable(hits, ...)` &rarr; `slices.SortStableFunc(hits, func(a, b Anchor) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/retrieve/retrieve.go:638`: `sort.SliceStable(out, ...)` &rarr; `slices.SortStableFunc(out, func(a, b Anchor) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/retrieve/retrieve.go:730`: `sort.SliceStable(out, ...)` &rarr; `slices.SortStableFunc(out, func(a, b Anchor) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/scopes/antigravity.go:35`: `sort.Strings(cwds)` &rarr; `slices.Sort(cwds)`
  - `internal/scopes/antigravity.go:82`: `sort.Strings(entries)` &rarr; `slices.Sort(entries)`
  - `internal/scopes/goose.go:35`: `sort.Strings(cwds)` &rarr; `slices.Sort(cwds)`
  - `internal/scopes/goose.go:82`: `sort.Strings(entries)` &rarr; `slices.Sort(entries)`
  - `internal/scopes/scopes.go:130`: `sort.Strings(entries)` &rarr; `slices.Sort(entries)`
  - `internal/scopes/scopes.go:209`: `sort.Strings(cwds)` &rarr; `slices.Sort(cwds)`
  - `internal/scopes/scopes.go:270`: `sort.Strings(entries)` &rarr; `slices.Sort(entries)`
  - `internal/semantic/semantic.go:366`: `sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })` &rarr; `slices.SortFunc(out, func(a, b ScoredChunk) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/semantic/semantic.go:461`: `sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })` &rarr; `slices.Sort(ids)`
  - `internal/view/view.go:321`: `sort.SliceStable(cands, ...)` &rarr; `slices.SortStableFunc(cands, func(a, b retrieve.Anchor) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/view/view.go:331`: `sort.SliceStable(cands, ...)` &rarr; `slices.SortStableFunc(cands, func(a, b retrieve.Anchor) int { return cmp.Compare(b.Score, a.Score) })`
  - `internal/view/view.go:341`: `sort.SliceStable(cands, ...)` &rarr; `slices.SortStableFunc(cands, func(a, b retrieve.Anchor) int { return cmp.Compare(b.Score, a.Score) })`
- **Slices Contains & ContainsFunc:**
  - `internal/agentproto/agentproto_store_test.go:427`: `for _, x := range xs { if x == val { ... } }` &rarr; `if slices.Contains(xs, val) { ... }`
  - `internal/cli/cli.go:1931`: `for _, o := range opts { if o == opt { ... } }` &rarr; `if slices.Contains(opts, opt) { ... }`
  - `internal/cli/cmd_completion_test.go:194`: `for _, got := range gotItems { if got == item { ... } }` &rarr; `if slices.Contains(gotItems, item) { ... }`
  - `internal/cli/setup.go:533`: `for _, vv := range val { if vv == hookCmd { ... } }` &rarr; `if slices.Contains(val, hookCmd) { ... }`
  - `internal/scopes/scopes_test.go:142`: `for _, s := range scs { if s.Label == "..." { ... } }` &rarr; `if slices.ContainsFunc(scs, func(s view.Scope) bool { return s.Label == "..." }) { ... }`
- **Slices Backward:**
  - `internal/cli/cmd_tag.go:192`: `for i := len(displayable) - 1; i >= 0; i-- {` &rarr; `for i, item := range slices.Backward(displayable) {`
  - `internal/cli/cmd_tag.go:323`: `for j := len(displayable) - 1; j >= 0; j-- {` &rarr; `for j, item := range slices.Backward(displayable) {`
  - `internal/cli/cmd_tag.go:746`: `for j := len(chunk.displayable) - 1; j >= 0; j-- {` &rarr; `for j, item := range slices.Backward(chunk.displayable) {`
- **Slices Clone:**
  - `internal/agentproto/agentproto_test.go:227`: `append([]view.ViewMsg(nil), tt.window...)` &rarr; `slices.Clone(tt.window)`
  - `internal/cli/tagrefresh_test.go:29`: `append([]source.Container(nil), s.containers...)` &rarr; `slices.Clone(s.containers)`
  - `internal/cli/tagrefresh_test.go:34`: `append([]model.Message(nil), s.messages...)` &rarr; `slices.Clone(s.messages)`
  - `internal/retrieve/retrieve.go:326`: `append([]string(nil), in.Terms...)` &rarr; `slices.Clone(in.Terms)`
  - `internal/retrieve/retrieve.go:337`: `append([]string(nil), terms...)` &rarr; `slices.Clone(terms)`
  - `internal/view/view_test.go:352`: `append([]retrieve.Anchor(nil), tt.in...)` &rarr; `slices.Clone(tt.in)`

#### 2.5 Use `cmp.Or` for Default Values *(Go 1.22+)*
- `internal/adapters/adapters.go:336-339`: `model := os.Getenv(...); if model == "" { model = DefaultEmbedModel }` &rarr; `model := cmp.Or(os.Getenv("RAWCLAW_EMBED_MODEL"), DefaultEmbedModel)`
- `internal/adapters/adapters.go:341-344`: `wire := os.Getenv(...); if wire == "" { wire = detectWire(ep) }` &rarr; `wire := cmp.Or(os.Getenv("RAWCLAW_EMBED_WIRE"), detectWire(ep))`
- `internal/paths/paths.go:45-48`: `if cc := os.Getenv("CLAUDE_CONFIG_DIR"); cc != "" { return cc }; return expandHome("~/.claude")` &rarr; `return cmp.Or(os.Getenv("CLAUDE_CONFIG_DIR"), expandHome("~/.claude"))`
- `internal/paths/paths.go:60-63`: `if x := os.Getenv("XDG_DATA_HOME"); x != "" { return filepath.Join(x, "rawclaw", "transcripts") }; return expandHome("~/.local/share/rawclaw/transcripts")` &rarr; `return filepath.Join(cmp.Or(os.Getenv("XDG_DATA_HOME"), expandHome("~/.local/share")), "rawclaw", "transcripts")`

#### 2.6 Use `sync.OnceValue` / `sync.OnceFunc` *(Go 1.21+)*
- `internal/provenance/provenance.go:60-73`: `var ( machineIDOnce sync.Once; machineIDValue string ); func MachineID() string { machineIDOnce.Do(...); return machineIDValue }` &rarr; `var MachineID = sync.OnceValue(loadOrMintMachineID)`
- `internal/agentproto/agentproto.go:523-532`: `var ( once sync.Once; vec []float64 ); qvecFn = func() []float64 { once.Do(...); return vec }` &rarr; `qvecFn = sync.OnceValue(func() []float64 { return embedder.Embed(context.Background(), rawQuery) })`

#### 2.7 Use `sync.WaitGroup.Go` *(Go 1.25+)*
- `internal/semantic/semantic.go:248-251`: `wg.Add(1); go func() { defer wg.Done(); ... }()` &rarr; `wg.Go(func() { ... })`
- `internal/archive/lock_test.go:138-143`: `wg.Add(1); go func() { defer wg.Done(); ... }()` &rarr; `wg.Go(func() { ... })`
- `internal/cli/cmd_ingest_test.go:338-347`: `wg.Add(1); go func(sid string) { defer wg.Done(); ... }(targetSID)` &rarr; `wg.Go(func() { ... })`
- `internal/index/tail_edge_test.go:1042-1044`: `wg.Add(1); go func() { defer wg.Done(); ... }()` &rarr; `wg.Go(func() { ... })`
- `internal/source/source_test.go:77-125`: `wg.Add(goroutines * 4); go func(...) { defer wg.Done(); ... }` &rarr; `wg.Go(func() { ... })`
- `internal/store/connect_test.go:63-65`: `wg.Add(1); go func() { defer wg.Done(); ... }()` &rarr; `wg.Go(func() { ... })`
- `internal/store/connect_test.go:96-102`: `wg.Add(1); go func() { defer wg.Done(); ... }()` &rarr; `wg.Go(func() { ... })`

#### 2.8 Use `t.Context()` in Tests *(Go 1.24+)*
- Over 180 test call sites currently pass manual `context.Background()` instead of test-scoped `t.Context()`:
  - `internal/archive/recovery_test.go:87`: `a.PushLocal(context.Background())` &rarr; `a.PushLocal(t.Context())`
  - `internal/archive/scopes_test.go:46`: `Init(context.Background(), bare, "machine-a")` &rarr; `Init(t.Context(), bare, "machine-a")`
  - `internal/archive/status_test.go:60`: `Init(context.Background(), bare, "machine-a")` &rarr; `Init(t.Context(), bare, "machine-a")`
  - `internal/cli/cmd_upgrade_test.go:310`: `latestReleaseTag(context.Background(), client, ...)` &rarr; `latestReleaseTag(t.Context(), client, ...)`
  - `internal/cli/timer_test.go:96`: `installTimer(context.Background(), ...)` &rarr; `installTimer(t.Context(), ...)`
  - `internal/live/client_test.go:42`: `c.List(context.Background(), ...)` &rarr; `c.List(t.Context(), ...)`
  - `internal/semantic/semantic_test.go:96`: `VecIndex(context.Background(), ...)` &rarr; `VecIndex(t.Context(), ...)`
  - `internal/semantic/topup_test.go:105`: `VecIndex(context.Background(), ...)` &rarr; `VecIndex(t.Context(), ...)`

#### 2.9 Use `b.Loop()` in Benchmarks *(Go 1.24+)*
- **Status:** Clean & fully modernized. All benchmark targets in `internal/index/index_bench_test.go`, `internal/semantic/semantic_bench_test.go`, `internal/source/antigravity/antigravity_bench_test.go`, and `internal/store/connect_bench_test.go` already use `for b.Loop() { ... }`.

---

### 3. Lower Priority (Gradual Improvement & Tooling)

#### 3.1 Use `strings.CutPrefix`, `strings.CutSuffix`, and `strings.Cut` *(Go 1.20+)*
- `internal/agentproto/warnings_test.go:252`: `if strings.HasPrefix(line, "note: ") { noteLines = append(noteLines, strings.TrimPrefix(line, "note: ")) }` &rarr; `if rest, ok := strings.CutPrefix(line, "note: "); ok { noteLines = append(noteLines, rest) }`
- `internal/archive/gitrun_test.go:40`: `if strings.HasPrefix(e, "GIT_SSH_COMMAND=") { ssh = strings.TrimPrefix(e, "GIT_SSH_COMMAND=") }` &rarr; `if val, ok := strings.CutPrefix(e, "GIT_SSH_COMMAND="); ok { ssh = val }`
- `internal/archive/gitrun_test.go:61`: `if strings.HasPrefix(e, "GIT_SSH_COMMAND=") { ssh = strings.TrimPrefix(e, "GIT_SSH_COMMAND=") }` &rarr; `if val, ok := strings.CutPrefix(e, "GIT_SSH_COMMAND="); ok { ssh = val }`
- `internal/archive/gitrun_test.go:106`: `if strings.HasPrefix(e, key+"=") { v = strings.TrimPrefix(e, key+"=") }` &rarr; `if val, ok := strings.CutPrefix(e, key+"="); ok { v = val }`
- `internal/cli/cmd_ingest.go:312`: `if idx := strings.IndexByte(p, '#'); idx >= 0 { p = p[:idx] }` &rarr; `p, _, _ = strings.Cut(p, "#")`
- `internal/index/containers.go:88`: `if idx := strings.IndexByte(p, '#'); idx >= 0 { p = p[:idx] }` &rarr; `p, _, _ = strings.Cut(p, "#")`
- `internal/index/tail.go:451-454`: `start := strings.Index(s, startTag); if end := strings.Index(sub, endTag); end >= 0 { ... }` &rarr; `_, after, ok := strings.Cut(s, startTag); if ok { inside, _, ok2 := strings.Cut(after, endTag); ... }`
- `internal/source/antigravity/antigravity.go:365`: `start := strings.Index(s, startTag)` &rarr; `_, after, ok := strings.Cut(s, startTag)`

#### 3.2 Use Iterator Variants (`strings.SplitSeq`, `strings.FieldsSeq`, `strings.Lines`) *(Go 1.24+)*
- `internal/agentproto/agentproto_test.go:102`: `for _, line := range strings.Split(buf.String(), "\n") {` &rarr; `for line := range strings.SplitSeq(buf.String(), "\n") {`
- `internal/agentproto/agentproto_test.go:429`: `for _, line := range strings.Split(out, "\n") {` &rarr; `for line := range strings.SplitSeq(out, "\n") {`
- `internal/agentproto/warnings_test.go:251`: `for _, line := range strings.Split(buf.String(), "\n") {` &rarr; `for line := range strings.SplitSeq(buf.String(), "\n") {`
- `internal/archive/tagconflict.go:68`: `for _, line := range strings.Split(string(b), "\n") {` &rarr; `for line := range strings.SplitSeq(string(b), "\n") {`
- `internal/cli/cli.go:546`: `for _, part := range strings.Split(w, ",") {` &rarr; `for part := range strings.SplitSeq(w, ",") {`
- `internal/cli/cmd_setup_live.go:182`: `for _, line := range strings.Split(string(out), "\n") {` &rarr; `for line := range strings.SplitSeq(string(out), "\n") {`
- `internal/cli/cmd_tag_test.go:62`: `for _, line := range strings.Split(strings.TrimSpace(out), "\n") {` &rarr; `for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {`
- `internal/cli/cmd_upgrade.go:400`: `for _, line := range strings.Split(string(checksums), "\n") {` &rarr; `for line := range strings.SplitSeq(string(checksums), "\n") {`
- `internal/query/query.go:65`: `for _, t := range strings.Fields(rest) {` &rarr; `for t := range strings.FieldsSeq(rest) {`
- `internal/query/query.go:198`: `for _, t := range strings.Fields(holes) {` &rarr; `for t := range strings.FieldsSeq(holes) {`
- `internal/query/query.go:221`: `for _, t := range strings.Fields(q) {` &rarr; `for t := range strings.FieldsSeq(q) {`
- `internal/source/antigravity/antigravity.go:373`: `for _, line := range strings.Split(sub, "\n") {` &rarr; `for line := range strings.SplitSeq(sub, "\n") {`
- `internal/source/antigravity/antigravity.go:433`: `for _, line := range strings.Split(string(data), "\n") {` &rarr; `for line := range strings.SplitSeq(string(data), "\n") {`
- `internal/source/claude/claude.go:85`: `for _, line := range strings.Split(string(data), "\n") {` &rarr; `for line := range strings.SplitSeq(string(data), "\n") {`
- `internal/source/codex/codex.go:140`: `for _, line := range strings.Split(string(data), "\n") {` &rarr; `for line := range strings.SplitSeq(string(data), "\n") {`
- `internal/store/topics.go:284`: `for _, t := range strings.Fields(query) {` &rarr; `for t := range strings.FieldsSeq(query) {`

#### 3.3 Zero-Allocation `fmt.Appendf` *(Go 1.19+)*
- `internal/index/tail.go:343`: `h := sha1.Sum([]byte(fmt.Sprintf("%s:%d", sessionID, ordinal)))` &rarr; `h := sha1.Sum(fmt.Appendf(nil, "%s:%d", sessionID, ordinal))`
- `internal/index/tail.go:494`: `h := sha1.Sum([]byte(fmt.Sprintf("%s:%d:%d", sessionID, stepIndex, ordinal)))` &rarr; `h := sha1.Sum(fmt.Appendf(nil, "%s:%d:%d", sessionID, stepIndex, ordinal))`
- `internal/provenance/provenance.go:100`: `return hex.EncodeToString([]byte(fmt.Sprintf("pid%d-%d", os.Getpid(), time.Now().UnixNano())))` &rarr; `return hex.EncodeToString(fmt.Appendf(nil, "pid%d-%d", os.Getpid(), time.Now().UnixNano()))`
- `internal/source/antigravity/antigravity.go:584`: `h := sha1.Sum([]byte(fmt.Sprintf("%s:%d:%d", sessionID, stepIndex, ordinal)))` &rarr; `h := sha1.Sum(fmt.Appendf(nil, "%s:%d:%d", sessionID, stepIndex, ordinal))`
- `internal/source/codex/codex.go:321`: `h := sha1.Sum([]byte(fmt.Sprintf("%s:%d", sessionID, ordinal)))` &rarr; `h := sha1.Sum(fmt.Appendf(nil, "%s:%d", sessionID, ordinal))`

#### 3.4 Inefficient Loop String Concatenation *(Go 1.10+ / Go 1.20+)*
- `internal/index/index_bench_test.go:19`: `content += l + "\n"` &rarr; `var sb strings.Builder; sb.WriteString(l); sb.WriteByte('\n')`
- `internal/index/index_test.go:415`: `content += l + "\n"` &rarr; `var sb strings.Builder; sb.WriteString(l); sb.WriteByte('\n')`
- `internal/store/sessions.go:65-68`: `whereSQL := where[0]; for _, w := range where[1:] { whereSQL += " AND " + w }` &rarr; `whereSQL := strings.Join(where, " AND ")`

#### 3.5 Tool Directives in `go.mod` *(Go 1.24+)*
- In `go.mod`, declare build and audit tools directly using `tool (...)` rather than requiring loose external binary installations:
  ```go.mod
  tool (
      github.com/golangci/golangci-lint/v2/cmd/golangci-lint
      golang.org/x/vuln/cmd/govulncheck
  )
  ```

---

## 5 Highest-Value One-Commit Fixes

These five self-contained refactors represent the highest engineering ROI across correctness, performance, safety, and modern standard library alignment:

### 1. Fix 1: Migrate `math/rand` to `math/rand/v2`
- **Scope:** `internal/cli/cmd_ingest.go:8,288`, `internal/semantic/semantic_bench_test.go:7,31`
- **Value:** Replaces legacy global seed/lock `math/rand` with auto-seeded, non-blocking ChaCha8/PCG `math/rand/v2` and idiomatic `rand.IntN(20)`. Completely eliminates deprecated pre-Go 1.22 RNG APIs from the binary.

### 2. Fix 2: Migrate All Sorting to `slices.Sort` / `slices.SortFunc` / `slices.SortStableFunc`
- **Scope:** `internal/paths/paths.go`, `internal/scopes/scopes.go`, `internal/lifecycle/lifecycle.go`, `internal/agentproto/agentproto.go`, `internal/retrieve/retrieve.go`, `internal/view/view.go`, `internal/durable/durable.go`, `internal/semantic/semantic.go`
- **Value:** Drops reflective `sort.Slice` and interface-boxed `sort.Strings` across 40+ call sites. Delivers zero-allocation, type-safe, generic in-place sorting and clearer `cmp.Compare` comparator lambdas throughout all search ranking and directory traversal hot paths.

### 3. Fix 3: Standardize Lazy Initialization with `sync.OnceValue`
- **Scope:** `internal/provenance/provenance.go:60-73`, `internal/agentproto/agentproto.go:523-532`
- **Value:** Replaces verbose manual `sync.Once` + outer mutable variable boilerplate with safe, idiomatic, single-expression `sync.OnceValue` functions that guarantee thread-safe lazy calculation without mutable package-level globals.

### 4. Fix 4: Modernize Counting Loops to Go 1.22+ `range` over Integer
- **Scope:** `internal/agentproto/`, `internal/index/`, `internal/semantic/`, `internal/store/`, `internal/source/`, `internal/cli/`
- **Value:** Replaces 38+ error-prone 3-clause `for i := 0; i < n; i++` index loops with readable `for i := range n` and `for range n` forms, eliminating off-by-one errors and aligning with Go 1.22+ loop semantics.

### 5. Fix 5: Adopt Standard Library Iterators (`strings.SplitSeq` / `strings.FieldsSeq`) on Tokenizing Hot Paths
- **Scope:** `internal/query/query.go:65,198,221`, `internal/store/topics.go:284`, `internal/source/antigravity/antigravity.go:373,433`, `internal/source/claude/claude.go:85`, `internal/source/codex/codex.go:140`
- **Value:** Eliminates heap allocations from intermediate `[]string` slices during search query token parsing, topic search string tokenization, and line-by-line transcript ingestion by ranging directly over Go 1.24+ standard library sequence iterators.
