# Architectural and Philosophical Decision Audit

Date: 2026-09-03 (WITA, UTC+8)  
Revision audited: `a85ddc4`  
Scope: all production code under `internal/` (27 packages, 33,019 production Go lines)

## Executive verdict

The codebase has a coherent product philosophy: a local, keyword-first, SQLite-backed
archive with optional semantic recall; explicit freshness and partial-result reporting;
source adapters; durable transcript copies; and fail-closed deletion/archive paths.
Those choices are broadly consistent with the external systems cited below.

The main architectural debt is concentration and duplicated policy, not a missing
framework. The highest-value simplifications are:

1. Extract one canonical normalized-message ingestion path. Claude parsing is duplicated
   in `internal/index/index.go` and `internal/source/claude/claude.go`, while the other
   adapters repeat the same whole-file materialization pattern.
2. Split `internal/agentproto/agentproto.go` (2,826 production lines) and
   `internal/cli/cli.go` (2,283 lines) by responsibility. They currently own protocol,
   retrieval, scope fallback, enrichment, rendering, and command orchestration together.
3. Remove or implement `embed.VectorStore`. It is a public-looking port with no
   production implementation or call site; semantic storage is hard-wired to SQLite.
4. Consolidate repeated tiny helpers (`isWordByte`, `first10`, `capRunes`, `sid8`, and
   `runeLen`) where doing so reduces policy drift.
5. Keep brute-force KNN only as the small-corpus default, but define a measured corpus
   threshold and an ANN/vector-index seam before growth makes query cost linear in all
   stored vectors.

No source changes are recommended by this report. Findings are report-only and ranked by
architectural leverage. Correctness, security, and performance observations are included
only where they expose an architectural decision or an unmeasured trade-off; they are not
presented as proven bugs.

## Audit receipts and limits

- Required Graphify-first query was attempted after `graphify reflect --if-stale`.
- This checkout had no `graphify-out/graph.json`; the nearby recovery-worktree graph was
  from a different checkout and returned no nodes for `store semantic retrieve cli index
  adapter`. Graph coverage is therefore **unavailable**, not evidence of absent symbols.
- Ponytail audit/review guidance and the requested Go how-to, modernize, lint, and
  performance guidance were read before source inspection.
- Source inventory used `rg --files`, package import listings from `go list`, line-numbered
  source inspection, and targeted searches for interfaces, duplicated helpers, SQLite
  configuration, whole-file reads, goroutines, and external-process boundaries.
- Baseline gate observed: `go version` = `go1.26.3 darwin/arm64`; the required
  `CGO_ENABLED=0 go test -race -count=1 ./...` exited non-zero. Passing packages included
  `cmd/rawclaw`, `internal/adapters`, `internal/agentproto`, `internal/durable`,
  `internal/embed`, `internal/index`, `internal/lifecycle`, `internal/live`,
  `internal/model`, `internal/parse`, `internal/paths`, `internal/provenance`,
  `internal/query`, `internal/render`, `internal/retention`, `internal/retrieve`,
  `internal/scopes`, `internal/semantic`, all source packages, `internal/store`,
  `internal/text`, `internal/timefmt`, and `internal/view`. Failures were observed in
  `internal/archive` (`TestBundle_ExportAndInitRoundTrip`, `TestArchiveBundle_EndToEnd`:
  `git bundle verify` reported “need a repository to verify a bundle”) and `internal/cli`
  (`TestMachineStream`, stale-search note, empty-query coaching, include-path session-ID
  hint, and flexible-date tests). This is a baseline failure, not a claim caused by this
  report.
- `AUDIT_MISSION.md` and `luna.log` were pre-existing untracked user files and were not
  modified or staged.
- `$HOME/go/bin/golangci-lint run ./...` completed with `0 issues`; the runner emitted a
  warning while processing a stale generated-file path from a sibling worktree, but did
  not report a lint finding for this checkout.

## System shape observed

```text
source adapters -> model.Message -> index -> store(SQLite + FTS5)
                                      |             |
                                      |             +-> retrieve/query -> agentproto -> cli/render/view
                                      |
                                      +-> durable vault / archive / consolidated store

optional HTTP embedder -> semantic (SQLite BLOB vectors + brute-force KNN + RRF)
```

The layering is real but porous. `agentproto` imports nine internal packages, `cli` imports
nearly every product package, and `scopes` imports concrete source adapters. The result is
easy to ship as one binary, but policy changes cross package boundaries and are difficult to
review as independent architectural units.

## Ranked findings

### A1. Canonical message normalization is duplicated

**Evidence:** `internal/index/index.go:733-784` defines `parseTranscript` and its own
`indexable`; `internal/source/claude/claude.go:99-161` independently reads, splits, decodes,
filters, extracts, timestamps, and deduplicates the same Claude JSONL. Codex and Antigravity
also materialize complete files in `internal/source/codex/codex.go:191-225` and
`internal/source/antigravity/antigravity.go:476-520`.

**Decision being audited:** each source adapter owns normalization, while the legacy Claude
index path retains a private parser for the fast/special path.

**Assessment:** this creates two contracts for the same source. A change to malformed-line
handling, UUID deduplication, cwd extraction, or role extraction can update one path and not
the other. It also makes the source interface less authoritative than its name suggests.

**External cross-examination:** ripgrep keeps searcher mechanics separate from format-aware
parsing and uses narrow traits/types at the boundary; Zoekt similarly separates document
ingestion from index/search execution. The applicable principle is one canonical record
normalizer feeding multiple consumers, not two semantically equivalent parsers.

**Ruling:** `yagni/shrink` — keep one source-owned normalized-message reader and let the
indexer consume it, or explicitly document and benchmark a genuinely distinct streaming
fast path. The current duplicate is not justified by a measured contract. This is the
highest-leverage deletion/refactor candidate.

### A2. `agentproto.go` is a protocol god package

**Evidence:** `internal/agentproto/agentproto.go:1-36` imports embed, index, paths, query,
retrieve, scopes, semantic, store, time formatting, and view. It owns search policy
(`484-678`), candidate collection (`1154-1525`), read resolution and budgeting
(`1590-1774`), session location (`1843-2072`), outline (`2087-2222`), and topic search
(`2492-2818`). The file is 2,826 production lines.

**Decision being audited:** centralize the agent-facing search/read/outline protocol in one
place so the CLI and future callers share exactly the same behavior.

**Assessment:** the goal is sound, but the unit is too broad. Ranking, freshness state,
session lookup, enrichment, and output serialization change for different reasons. The
current structure makes it hard to test or replace one policy without loading the others.

**External cross-examination:** `cli/cli` separates command construction, command execution,
and API clients; Cobra's command tree is intentionally a tree of small command nodes. A
protocol facade can remain stable while delegating to `search`, `resolve`, `read`, `outline`,
and `render` components.

**Ruling:** `shrink` — retain `agentproto` as a small public facade and move pure scoring,
scope reports, session resolution, enrichment, and render helpers into separate packages or
files with narrow inputs. Do not introduce interfaces merely to split files.

### A3. `cli.go` and `setup.go` concentrate unrelated policy

**Evidence:** `internal/cli/cli.go` is 2,283 production lines and imports almost every
internal package. `internal/cli/setup.go` is 1,265 lines and embeds multiple shell-hook
templates, JSON extraction, catalog writing, platform setup, and eject behavior. The
top-level CLI also contains browse fallback, search routing, timeout behavior, and upgrade
logic.

**Decision being audited:** keep the product's full UX and all command wiring in one CLI
package to preserve a single static binary and shared flag semantics.

**Assessment:** package-level cohesion is low. The code is not dependency-heavy, but the
review surface is large and command behavior is coupled to protocol internals. The setup
templates are especially sensitive: one template change can affect several runtimes.

**External cross-examination:** GitHub CLI uses command packages with shared root/config
services; Cobra provides the tree and flag parsing but does not require business logic in
the root command. Lipgloss keeps rendering primitives independent from command behavior.

**Ruling:** `shrink` — split by command family (`search/read/browse`, `archive`, `setup`,
`lifecycle`, `upgrade`) while preserving one `cli.Execute` entry point. Keep generated hook
templates in a dedicated package with fixture tests. Avoid a dependency-injection framework.

### A4. `embed.VectorStore` is an unused abstraction

**Evidence:** `internal/embed/embed.go:66-78` declares `VectorStore`, but production search
and indexing call `internal/store` vector functions directly (`internal/semantic/semantic.go:
112-529`). `rg` finds no production implementation or use of `VectorStore`; only
`Embedder` and optional `BatchEmbedder` are wired.

**Decision being audited:** define both embedding and vector-storage ports in advance so
Ollama/OpenAI/Voyage and sqlite-vec/other stores can be plugged in later.

**Assessment:** the embedder port is active and valuable. The vector-store port is a
speculative promise whose contract is weaker than the actual semantic implementation: it
does not expose dimensions, errors, lifecycle, filtering, deletion, or metadata needed by
`VecKNN` and `Fuse`. It gives readers the impression that storage is replaceable when it is
not.

**External cross-examination:** FAISS and Lance expose explicit index/storage capabilities,
metadata and lifecycle rather than a three-method opaque port. SQLite FTS5 is also an
explicit table/index contract, not a generic interface hiding all query semantics.

**Ruling:** `delete` the unused port until a second vector backend exists, or replace it with
a real semantic-index boundary whose contract is driven by two implementations. Do not
keep an aspirational interface solely for future flexibility.

### A5. Source extensibility is only partially centralized

**Evidence:** `internal/sources/sources.go:23-32` centrally registers seven adapters, but
`internal/scopes/scopes.go:54-262` directly imports and constructs concrete adapters, and
`internal/archive` imports concrete Antigravity/Codex packages for archive behavior.

**Decision being audited:** use a source registry for discovery while allowing scope-specific
special cases for source layouts.

**Assessment:** adding a source requires edits in the registry plus source-specific scope,
archive, or freshness paths. The `source.Source` interface itself is small and sensible
(`internal/source/source.go:21-59`), but the system-wide extension seam is not actually one
seam.

**External cross-examination:** Zoekt's source/index boundary and ripgrep's searcher
abstractions keep format-specific behavior behind a small implementation boundary. The
lesson is not “use plugins”; it is “one registration record should carry discovery,
indexing, and capability metadata where possible.”

**Ruling:** `yagni` for a runtime plugin system, but `shrink` the built-in wiring: put
source-specific scope/archive capabilities on registration or a small capability struct;
then make generic callers consume registrations. Keep compile-time built-ins for the static
binary.

### A6. Vector recall is deliberately linear, but the growth policy is undefined

**Evidence:** `internal/semantic/semantic.go:341-418` scans every stored vector, even though
it uses a heap and an optimized float32/unrolled dot product. The code explicitly states the
strategy is brute force and linear (`367-370`). `VecKNN` reads all vector rows before
existence checks (`420-457`).

**Decision being audited:** use plain Go and SQLite BLOBs, with no ANN dependency, because
the optional feature should preserve the single-binary/no-cgo default.

**Assessment:** reasonable for a modest local corpus and consistent with the product's
optional posture. It is not a complete scaling strategy. The current benchmark evidence
supports a local optimization, not a corpus-size ceiling or recall/latency SLO.

**External cross-examination:** FAISS provides multiple exact and approximate indexes;
Lance separates columnar storage from vector indexing; RoaringBitmap demonstrates compact
set representations for candidate filtering. These systems support a staged policy: exact
scan for small corpora, measured ANN or narrowed candidate sets for large corpora.

**Ruling:** `performance gap, not immediate defect`. Add a benchmarked threshold and a
future seam in the design notes. Do not add FAISS/Lance or cgo to the default path. Measure
query latency, allocations, vector count, dimensions, and recall before selecting an ANN
implementation.

### A7. Whole-file materialization is repeated across ingestion

**Evidence:** Claude, Codex, and Antigravity `Messages` implementations use `os.ReadFile`
and `strings.Split` (`claude.go:103-111`, `codex.go:191-200`, `antigravity.go:476-486`).
The Claude index parser also uses `os.ReadFile` and `bytes.SplitSeq` (`index.go:745-754`).
`durable.StoreFile` copies the full source with `os.ReadFile` (`durable.go:103-110`).

**Decision being audited:** materialize one session so parsing, deduplication, atomic
replacement, and durable vaulting are simple and consistent.

**Assessment:** the atomic replacement decision is good, but memory use scales with the
largest transcript and is paid multiple times for some paths. A scanner/streaming decoder
would reduce peak memory, but must preserve malformed-line tolerance and the ability to
atomically replace the session.

**External cross-examination:** ripgrep is explicitly streaming and bounded at the search
boundary; SQLite's bulk-ingest guidance favors transactions and incremental batches; RocksDB
uses write batches and iterators rather than whole-dataset materialization.

**Ruling:** `performance gap`. Use a bounded scanner/decoder for parsing and batch inserts,
while retaining one transaction and atomic vault writes. First benchmark peak RSS and
throughput on representative large transcripts; do not optimize based on intuition alone.

### A8. Single-connection SQLite discipline is safe but globally fragile

**Evidence:** `internal/store/store.go:314-352` caps both read and write pools at one
connection and documents that callers must drain/close rows before issuing another query.
The same constraint is repeated in semantic and view comments. Source readers independently
configure SQLite connections with their own busy timeouts and mmap settings.

**Decision being audited:** use one connection per database because SQLite is a single-writer
local store and this avoids interleaving/deadlock classes.

**Assessment:** the decision is understandable and keeps lock behavior predictable. It
turns resource lifetime into an implicit cross-package protocol: a missed `Rows.Close` can
block unrelated work. It also makes future parallel read optimization impossible without
changing the contract.

**External cross-examination:** SQLite documents WAL's reader/writer behavior and transaction
boundaries; LMDB exposes a transaction/reader model explicitly; RocksDB uses iterators and
write batches with clearer lifetime boundaries. The common principle is explicit handle
ownership, not a hidden pool invariant.

**Ruling:** `accepted with documentation debt`. Keep one connection for writes and small
local stores. Consider a small read-connection factory or explicit query helpers if profiling
shows contention. Centralize DSN/pragma policy so Goose, Hermes, OpenCode, and RawClaw do not
drift.

### A9. Consolidation is a useful read optimization with a large consistency protocol

**Evidence:** `internal/index/consolidated.go:167-230` records per-source provenance and
merges contributions; `491-546` performs rebuild/fold/watermark phases; `553-609` performs
write-through synchronization. `agentproto.Search` documents consolidated-vs-fan-out
semantics at `470-483` and reports partial scope states at `97-115`.

**Decision being audited:** answer first from one consolidated database, fall back to
per-project stores when absent/stale/unavailable, and expose incompleteness as data.

**Assessment:** the consistency model is unusually explicit and avoids silently dropping
scopes. The cost is a second index representation plus watermark/provenance state. Every
new source or metadata field must be merged correctly in both the per-project and
consolidated paths.

**External cross-examination:** RocksDB's WAL/manifest model and SQLite's transaction model
show the value of durable, ordered state transitions; Tantivy uses segment publication and
explicit commit points. The comparable lesson is to make freshness/version state first-class
and mechanically testable, which this design does, but keep the number of representations
small.

**Ruling:** `accepted product trade-off`. Preserve the consolidated read path. Reduce policy
duplication by centralizing merge field definitions and source capability metadata. Treat
`ScopeReport` as a stable public contract.

### A10. Query sanitation is a hand-rolled mini-language

**Evidence:** `internal/query/query.go:29-48` defines eleven regular expressions; it protects
phrases with a NUL sentinel, rewrites booleans, quotes dotted identifiers, and restores
phrases (`151-256`). `MakeSnippet` compiles a dynamic regular expression per term (`91-145`).
`PathPredicate` silently converts an invalid regex to literal substring matching (`259-301`).

**Decision being audited:** make natural-language queries safe for FTS5 while preserving
phrases, paths, wildcards, and a small boolean syntax without exposing raw MATCH syntax.

**Assessment:** the explicit language boundary is better than concatenating raw FTS5 input.
The sentinel/regex pipeline is complex and has multiple semantic modes. Silent invalid-regex
fallback is friendly but can surprise automation that believes it requested a regex.

**External cross-examination:** SQLite FTS5 documents a precise MATCH grammar; Tantivy and
Zoekt expose structured query parsers rather than repeated regex rewrites. A small tokenizer
or typed query AST would make supported syntax and errors more explicit.

**Ruling:** `accepted for now; shrink later`. Keep the supported language intentionally
small. Prefer an explicit parse result containing terms, phrases, operators, and invalid
input state over a sequence of string rewrites if new syntax is added. Do not replace it
with a general parser dependency.

### A11. Durable vault and archive correctly separate truth, cache, and transport

**Evidence:** `internal/durable/durable.go:1-11` defines the vault as rebuild truth while
`internal/paths/paths.go:58-67` places it in data storage, not cache. Durable writes stage,
fsync, rename, and fsync the directory (`durable.go:220-261`). Archive push uses a per-machine
tree and a lock/retry/rebase protocol (`archive/push.go:19-117`, `358-409`).

**Decision being audited:** store durable local copies in a readable Claude-shaped format;
use any private git remote as optional transport; never make the archive a server or a
mandatory dependency.

**Assessment:** this is a strong philosophical decision for a zero-runtime-dependency tool.
The copy-on-rename and stranded-commit guards are proportionate to the data-loss risk.
Git is not a database or a confidentiality boundary, so the private-remote requirement must
remain prominent.

**External cross-examination:** LMDB and SQLite emphasize durable commit semantics; RocksDB
uses WAL/manifest recovery; Git itself supplies content-addressed transport but not secret
management. The design should continue to treat transport, durability, and privacy as
separate concerns.

**Ruling:** `accepted`. Keep the vault format and explicit private-remote warning. Avoid
adding encryption or a server to the core unless the product scope changes.

### A12. Fail-soft optional semantics are coherent, but observability is thin

**Evidence:** `internal/embed/embed.go:38-64` defines nil as the no-embedding signal;
`internal/adapters/adapters.go:47-57` collapses all HTTP failures to nil. Semantic top-up
is detached and throttled (`internal/semantic/topup.go:101-135`). Search reports vector
coverage and scope status through the agent envelope (`agentproto.go:97-115`, `181-220`).

**Decision being audited:** an unavailable optional embedder must never make keyword search
fail or slow unnecessarily.

**Assessment:** this is exactly right for the sovereign keyword core. The trade-off is that
backend outage, malformed response, dimension mismatch, and deliberate no-op all look alike
to the caller. Search has coverage information, but the adapter itself offers no structured
diagnostic channel.

**External cross-examination:** production clients in GitHub CLI and llama.cpp distinguish
configuration, transport, and response failures while allowing optional features to degrade.
The same separation would preserve fail-soft behavior without hiding operational causes.

**Ruling:** `accepted behavior; observability gap`. Keep the nil contract for the search path;
add bounded debug/metrics/receipt information outside the result contract if operators need
to distinguish disabled from failing.

## Package-by-package decision inventory

This inventory records the principal architectural choices found in every internal package.
“Keep” means the choice is coherent; it does not mean the implementation cannot improve.

| Package | Principal decisions | Audit ruling |
|---|---|---|
| `adapters` | Environment-driven HTTP embedding; Ollama/OpenAI wire shapes; nil on any failure; optional batching. | Keep the optional seam. Consider typed config validation and bounded diagnostics. |
| `agentproto` | One agent protocol for search/read/outline; bounded reads; explicit partial-result envelope; post-ranking enrichment. | Keep the contract; split the god package (A2). |
| `archive` | Git remote as transport; per-machine namespace; lock; bounded pull-rebase-push; tombstone propagation; recovery before destructive rebuild. | Keep. Centralize repeated git/state transitions if future features grow. |
| `cli` | Cobra command tree; search as default verb; answer-first consolidated reads; detached background work; platform-specific hooks/timers. | Keep UX direction; split command families (A3). |
| `durable` | RawClaw-owned vault in data dir; Claude-shaped JSONL; sidecar metadata; staged fsync+rename. | Keep. Benchmark/stream large writes only if needed. |
| `embed` | Minimal `Embedder` and optional `BatchEmbedder`; nil means no vector. | Keep active ports; remove unused `VectorStore` (A4). |
| `index` | SQLite FTS5 cache; incremental file watermarks; atomic per-session replacement; consolidated secondary store; durable vaulting. | Keep core; remove duplicate Claude parser (A1); make merge schema declarative. |
| `lifecycle` | Filter-gated, dry-run-first deletion; tombstone sidecar; direct filesystem operation independent of index. | Keep safety boundary. Reconsider duplicate JSONL counting if index metadata can be trusted for a future fast path. |
| `live` | SSH transport; remote computes ages; bounded response buffers; source registry for remote enumeration. | Keep. It is correctly a transport seam, not archive coupling. |
| `model` | Small normalized message value type; source-specific provenance stays in `Container`. | Keep; this is a good boundary. |
| `parse` | Shared text extraction, generated/tool stripping, timestamp and role normalization. | Expand this seam carefully to absorb A1 without making it a universal parser. |
| `paths` | XDG/config-root resolution; catalog; containment-aware JSONL enumeration; catalog-first session resolution. | Keep. Avoid adding more global path policy here. |
| `provenance` | Stable machine/session IDs and file fingerprints. | Keep. Document hash truncation assumptions if IDs become security-sensitive. |
| `query` | Pure standard-library query sanitizer and tiny boolean language. | Keep current language; replace rewrite pipeline only when syntax expands (A10). |
| `render` | Writer-injected pure output formatting; honest score explanations; JSON projection DTOs. | Keep; good separation from CLI. |
| `retrieve` | Store-backed keyword retrieval; substring fallback; coverage/recency sorting; linear transcript fallback. | Keep behavior; consolidate helper duplication and measure fallback cost. |
| `retention` | Pure retention decision policy used by index/archive/lifecycle. | Keep; this is a good example of extracting policy into a small package. |
| `scopes` | Scope discovery/filtering, source-specific container handling, orphan stores. | Keep concept; stop importing concrete adapters in multiple places (A5). |
| `semantic` | Optional vectors in same SQLite DB; content-hash identity; batch embedding; heap-bounded brute-force cosine; RRF. | Keep optionality; define growth threshold and remove unused storage port (A4/A6). |
| `source` | Two-method source interface plus registration metadata and container value. | Keep interface; move more source capabilities into registration metadata (A5). |
| `sources` | Built-in registration of all adapters. | Keep static registration; make it the only wiring list. |
| `store` | SQLite schema owner; FTS5 word/trigram tables; one-connection DSN; typed query functions; topic/vector tables. | Keep SQLite choice; centralize source-reader DSNs and consider explicit connection lifetimes (A8). |
| `text` | Regex-based code-identifier splitting. | Keep if vocabulary is intentionally ASCII-ish; benchmark or document Unicode behavior before broadening. |
| `timefmt` | Centralized UTC/local display formatting. | Keep; small and appropriately scoped. |
| `view` | Display filtering, anchored windows, bookends, previews, caps, no direct rendering. | Keep; it is one of the cleaner seams. |

## Ponytail semantic findings

These are complexity-only findings, separate from correctness and performance review.

- `internal/embed/embed.go:66-78`: **delete:** unused `VectorStore` port. Replacement:
  direct semantic/store calls until a second backend exists.
- `internal/index/index.go:733-847` plus `internal/source/claude/claude.go:99-161`:
  **yagni/shrink:** two Claude normalizers. Replacement: one canonical source reader.
- `internal/query/query.go:433-440` and `internal/retrieve/retrieve.go:39-56`:
  **shrink:** duplicate ASCII `isWordByte`; replacement: one private helper in a shared
  query/text utility only if it truly has two production consumers.
- `internal/store/stats.go:86-88` and `internal/retrieve/retrieve.go:749-753`:
  **shrink:** duplicate `first10`; replacement: one timestamp helper, or inline the two
  short slices if the shared helper would create a worse dependency.
- `internal/parse/parse.go:28-46` and `internal/view/view.go:191-196`:
  **shrink:** duplicate `capRunes`; replacement: one display/text helper if both packages
  can share it without a layering violation.
- `internal/agentproto/agentproto.go:336-350` and `internal/render/render.go:18-24`:
  **shrink:** duplicate `sid8`; replacement: one render-format helper.
- `internal/agentproto/agentproto.go:444-450` and `internal/query/query.go:361-367`:
  **shrink:** duplicate rune-length loops; use `utf8.RuneCountInString` where the exact
  semantics match, or keep local code rather than add a utility package for two lines.
- `internal/source/goose/goose.go:431-464`: **yagni:** `rowsIterator` exists only to
  abstract `*sql.Rows` for one production use and a test fake. Replacement: pass the
  concrete rows type unless more row providers appear; the test can use a helper or a
  narrow test-only adapter.
- `internal/agentproto/agentproto.go:1368-1388`: **keep:** `dbConnCache` is not flagged;
  it has a real cross-database resource-lifetime purpose and avoids repeated opens.

Net simplification potential is material but intentionally not estimated in lines: the
largest reductions come from structural extraction and parser deduplication, where a raw
line count could reward merely moving code.

## Optimization and edge-case backlog

1. Establish a representative corpus benchmark for: indexing peak RSS, incremental append
   latency, consolidated fold time, keyword search p50/p95, vector search p50/p95, and
   vector count/dimension. The current semantic benchmark proves a local loop optimization,
   not system-scale behavior.
2. Measure the cost of `store.AllMessages` and `store.VecAll` before changing their full-scan
   contracts. If large corpora become normal, use streaming rows plus bounded batches.
3. Define an exact-to-ANN migration policy: corpus threshold, acceptable recall loss, index
   rebuild behavior, dimension changes, and optional dependency policy. Compare FAISS/Lance
   style indexes only in an opt-in seam.
4. Add a source-registration completeness test that fails when a built-in source is in the
   registry but absent from scope/live/archive capability routing.
5. Make invalid `--include-path` behavior machine-visible: distinguish “regex accepted”
   from “literal fallback” in structured output, or reject invalid regexes in strict JSON
   mode while preserving the friendly human fallback if product requirements demand it.
6. Add a single source-normalization golden corpus exercised through every caller that can
   ingest that source. This guards parser convergence more directly than package-local
   tests.
7. Centralize SQLite DSN pragma construction for RawClaw-owned and read-only foreign source
   databases. Keep foreign database compatibility rules explicit rather than sharing a
   single overly strict DSN.

## External authority used

The following references are the architectural comparators, not RawClaw. Repository paths
are included so the claims are inspectable and can be revisited as upstreams evolve.

- BurntSushi/ripgrep searcher and streaming search architecture:
  https://github.com/BurntSushi/ripgrep/blob/master/crates/searcher/src/searcher/mod.rs
- quickwit-oss/tantivy index writer/segment publication:
  https://github.com/quickwit-oss/tantivy/blob/main/src/indexer/index_writer.rs
- sourcegraph/zoekt source/index/search implementation:
  https://github.com/sourcegraph/zoekt
- SQLite FTS5 grammar, tokenizers, ranking, and external-content tables:
  https://www.sqlite.org/fts5.html
- SQLite WAL and transaction behavior:
  https://www.sqlite.org/wal.html
- LMDB transaction-oriented embedded storage:
  https://www.symas.com/lmdb
- RocksDB write batches, WAL, compaction, and iterators:
  https://github.com/facebook/rocksdb/wiki/RocksDB-Basics
- GitHub CLI command/config architecture:
  https://github.com/cli/cli/blob/trunk/pkg/cmd/root/root.go
- Cobra command tree and flag model:
  https://github.com/spf13/cobra/blob/main/command.go
- Lipgloss terminal rendering primitives:
  https://github.com/charmbracelet/lipgloss
- viterin/vek Go SIMD/vector operations:
  https://github.com/viterin/vek
- klauspost/cpuid CPU feature detection:
  https://github.com/klauspost/cpuid
- Roaring Bitmap compressed set representation and paper links:
  https://github.com/RoaringBitmap/roaring
  https://arxiv.org/abs/1709.07821
- FAISS similarity-search indexes and paper:
  https://github.com/facebookresearch/faiss
  https://arxiv.org/abs/1702.08734
- Lance vector/columnar storage:
  https://github.com/lancedb/lance
- llama.cpp optional local inference/runtime architecture:
  https://github.com/ggerganov/llama.cpp
- Robertson and Zaragoza, *The Probabilistic Relevance Framework: BM25 and Beyond*:
  https://doi.org/10.1561/1500000019
- Cormack, Clarke, and Buettcher, *Reciprocal Rank Fusion outperforms Condorcet and
  individual Rank Learning Methods*:
  https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf

## Final disposition

`PARTIAL`: the architectural findings and requested report are complete, but the required
race suite is not green at `a85ddc4`; the failures above predate this report and were not
modified. No recommendation in this document authorizes source edits.
