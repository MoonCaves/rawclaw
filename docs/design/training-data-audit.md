# Prior-art vs. training-data contamination audit

Date: 2026-09-05 (WITA)

Scope: `7eba11f..98ade78`, including the exact-tier lift and the RRF
integration; the design files and `exact-tier-lift` messages 290–309; and the
named `internal/retrieve`, `internal/query`, `internal/store`, and
`internal/index` areas.

## Executive ruling

**PARTIAL / HOLD.** The central search mechanisms have identifiable prior art,
but the branch is not fully lineage-clean. The surviving implementation mixes
source-grounded mechanisms with local heuristics and undocumented adaptations.
The latter must either acquire an exact external implementation citation or be
explicitly treated as RawClaw-owned policy and justified by tests.

The requested `docs/design/exact-tier-build-notes.md` is absent. The surviving
file is `docs/design/exact-tier-notes.md`, created by the final merge.

The coordination record is useful evidence, but it is not itself prior art:
messages 290–309 show proposals, adversarial tests, rulings, and the merge; the
external lineage must come from the cited open-source files.

## Evidence and method

- Reviewed the commit history and patches in `git log -p 7eba11f..HEAD`.
- Read `decision-references.md`, `steal-code.md`, `prior-art-steal-list.md`, and
  the surviving `exact-tier-notes.md`.
- Read agent-mail messages 290–309 from the HTTP Agent Mail database.
- Inspected current code and the parent revisions to distinguish new work from
  pre-existing mechanisms.
- `gofmt -l internal/` is empty. The historical range has one whitespace
  warning from `git diff --check` at `docs/design/exact-tier-notes.md:172`;
  this audit does not alter that file or any Go file.

For this report, “grounded” means that the cited external file and line range
covers the implemented mechanism, not merely a similar idea. A local test or a
design document can validate behavior, but cannot establish external lineage.

## (a) Caught and purged

### C1. CASS custom `tokenchars` copied into prose search

Status: **Caught and purged.**

The initial exact table copied CASS's
`unicode61 tokenchars '-_./:@#$%\\'` from `tests/pages_fts.rs:174`. The
coordination record then tested it against prose transcripts and found that a
sentence-final `auth.` became one token, so `auth` did not match; the same
problem affected `store.go` inside a path (message 296). Commit `b581fce`
changed the table to plain `unicode61`, and `a0c0737` retained that correction.

Receipts:

- CASS source citation: `docs/design/decision-references.md:20-28` and
  `docs/design/steal-code.md:34-38`.
- Rejection and empirical proof: `docs/design/exact-tier-notes.md:65-75`;
  message 296; implementation in `internal/store/store.go:142-148`.

The source was real, but applying its tokenization domain to natural-language
agent transcripts was the contamination failure. The final tokenizer choice is
not contaminated by the CASS tokenchars snippet.

### C2. CASS query-shape router

Status: **Caught and purged.**

`0a26535` added `detectSearchMode`, camelCase/kebab-case detection, and a
`cass-router` mode based on CASS `src/pages/fts.rs:84-179`. The locked benchmark
showed that the prose query `where did we land on auth` was routed to the
stemmed table and reproduced the `land` noise. Message 306 ordered deletion;
`a0c0737` removed the router, helpers, mode, and tests.

Receipts: `docs/design/exact-tier-notes.md:49-63` and `:155-169`; messages
293, 305, 306, and 307. No router survives in `internal/` at `HEAD`.

### C3. Exact-first fallback

Status: **Caught and purged.**

The exact-first mechanism was source-informed but its cutoff policy was not
validated before implementation: any exact hit stopped the stemmed query. On
`handling credential leaks in commits`, two exact hits suppressed 28 useful
stemmed matches. The benchmark and referee ruling selected RRF instead, and
`a0c0737` removed the mode and tests.

Receipts: `docs/design/exact-tier-notes.md:134-140` and `:159-162`; messages
300, 304, 305, and 306.

This is a good example of a plausible retrieval heuristic that survived initial
review only until an adversarial corpus exposed its recall failure.

### C4. Navidrome row-weighting proposal

Status: **False alarm / withdrawn before shipping.**

Navidrome's per-column BM25 weighting was considered for role weighting, but
message 296 correctly noted that RawClaw's FTS table has one text column and
that a row-level multiplier would be an invention. No such role multiplier
survives in this range.

Evidence: `docs/design/decision-references.md:56-66`; message 296.

## (b) Survived into `HEAD`

### S1. External-content exact FTS5 table and synchronization triggers

Status: **Grounded core; stale comments remain.**

The surviving table in `internal/store/store.go:142-158` uses external content,
`content_rowid`, and FTS5 insert/delete/update triggers. This is covered by
ccrider `internal/core/db/schema.go:86-122`, especially its plain `unicode61`
second table and FTS5 `'delete'` trigger protocol. The rebuild command is also
explicitly present in that cited block (`:122`) and is used by
`internal/index/index.go`.

The implementation comments are not fully clean: `store.go:135-137` says
“custom tokenchars”, and `internal/store/fts.go:73-74` calls the table “exact
tokenchars”, although the final SQL is plain `unicode61`. Those comments are
surviving residue from the rejected CASS design and should be corrected.

External lineage: ccrider `internal/core/db/schema.go:86-122` (MIT), recorded at
`docs/design/decision-references.md:7-18` and
`docs/design/steal-code.md:7-32`.

### S2. Exact-table store wrappers

Status: **Grounded as a direct adaptation.**

`SearchHitsExact` and `SearchAnchorsExact` in `internal/store/fts.go:204-207`
and `:293-296` reuse the existing SQL body with a second table name. The
two-table shape is covered by ccrider `schema.go:86-122`; the wrapper names and
RawClaw-specific result structs are local glue, not an independently invented
ranking mechanism.

### S3. FTS query conversion

Status: **Grounded core, with local deviations that are not independently
grounded.**

`internal/query/query.go:190-241` implements the character-level parser for
quoted phrases, boolean operators, prefix `*`, `-` as NOT, and `|` as OR. The
corresponding source is zk `internal/util/fts5/fts5.go:6-104`, cited at
`docs/design/decision-references.md:30-41` and reproduced at
`docs/design/steal-code.md:69-127`.

Two details exceed that citation and therefore remain ungrounded adaptations:

1. `buildMatch` and `LinearFallback` call `StripStopwords` before conversion
   (`internal/retrieve/retrieve.go:153-180` and the later fallback). The
   ordering is a RawClaw rule; no external file is cited for it.
2. `EscapeFTS5Query` (`internal/query/query.go:149-187`) is also copied from
   ccrider `internal/core/search/search.go:356-425`, but it is not the active
   sanitizer and its whole-query phrase special case does not cover the mixed
   phrase behavior that motivated zk's parser. Treat it as dead duplicated
   code, not as evidence for the active path.

### S4. Default RRF over exact and stemmed ranked lists

Status: **Grounded core; ungrounded key/tie-break adaptation.**

The reciprocal-rank calculation and `k=60` in
`internal/retrieve/retrieve.go:399-472` and its use at `:517-533` are covered
by grepai `search/hybrid.go:57-89` (MIT), as recorded at
`docs/design/decision-references.md:43-52` and
`docs/design/steal-code.md:184-209`.

The following surviving details are not covered by that citation and must not
be described as “lifted verbatim”:

- `rrfHits` keys hits by `SessionID:ISO:Role` (`:447-450`), while
  `rrfAnchors` prefers UUID and falls back to `SessionID:ISO`
  (`:405-411`). The source citation does not establish these RawClaw identity
  rules.
- Equal-score ordering by ISO descending and ID/session ascending
  (`:418-466`) is a local tie-break policy. It needs an external exact
  precedent or an explicit RawClaw policy/test rationale.
- The two-query AND-then-OR behavior around each list (`:519-529` and
  `:800-810`) is RawClaw control flow. The cited RRF implementation does not
  establish it.

Therefore the RRF formula is accepted as grounded, but the complete ranking
mechanism is not lineage-clean.

### S5. `--exact` behavior

Status: **Grounded core; local fallback detail.**

The flag routes to the exact table in `internal/retrieve/retrieve.go:509-516`
and `:790-797`. Calibre's table-selection behavior is cited at
`src/calibre/db/fts/connect.py:164-165,193-195`, recorded at
`docs/design/decision-references.md:43-54` and
`docs/design/steal-code.md:211-230`.

RawClaw's same-table OR retry for a multi-term query is local glue. Calibre's
flag citation supports selecting a table, not this exact fallback sequence.

### S6. SQLite read-only tuning

Status: **Survived ungrounded invention.**

Commit `494a875` changed `ROMmapSize` to `0x7fff0000` and added
`cache_size(-64000)` (`internal/store/store.go:345-358`). The design references
contain no authoritative open-source implementation for these exact values or
for the claim that the former is the appropriate ceiling. The added test at
`internal/store/connect_test.go:34-43` only locks in the local choice; it does
not establish lineage.

Required disposition: cite an exact SQLite/driver source and explain the
platform/driver constraints, or revert the magic values to a documented,
measured RawClaw policy.

### S7. Index freshness and settle policy

Status: **Survived ungrounded adaptation.**

Commit `701569a` changed `CheckIndexFreshness` so that catalog-entry mtime,
transcript mtime, a `60s` settle window, a `0.001` timestamp epsilon, and
missing transcript handling determine freshness (`internal/index/consolidated.go:1464-1577`).

The references cite hpcloud/tail `watch/polling.go:83-109` for file identity,
size, shrink, growth, and mtime changes (`decision-references.md:96-105`), but
RawClaw's final code does not implement that detector. Nor does the cited file
establish the directory-mtime shortcut, the 60-second value, the epsilon, or
the policy that a purged transcript is not stale. Those are local inventions
or adaptations and must be labeled and justified as such.

### S8. Overfetch and post-query coverage policy

Status: **Survived; mostly pre-existing, not lineage-cleared by this range.**

The active code uses `limit*4`, `limit*8`, and `limit*2` in
`internal/retrieve/retrieve.go:490-504`, then applies distinct-term coverage
and routine partitioning at `:567-580`. The design notes document the `20000`
store window and `limit*8` observation at `exact-tier-notes.md:143-151`, but
those are measurements of RawClaw behavior, not external prior art. No exact
open-source file is cited for these constants or for this particular
post-ranking policy.

This is not evidence that the exact-tier commits invented the policy: much of
it predates the range. It is nevertheless not lineage-clean under the audit's
broader “any code in the named areas” requirement.

### S9. Local acceptance thresholds

Status: **Survived in design documents as ungrounded project policy.**

The P@1, R@3, `<=100ms`, two-run timing, and eight-query corpus gates at
`exact-tier-notes.md:82-87` are reasonable local test criteria, but no external
implementation can be expected to provide them. They should be labeled
RawClaw acceptance policy, with corpus provenance and rationale, rather than
presented alongside prior-art lineage.

## (c) False alarms and non-findings

### F1. The final RRF choice was not an intuition-only invention

Messages 303–309 show a measured recall failure, an independent referee ruling,
and a race-gated merge. The formula itself has the grepai citation. The
remaining concern is limited to the RawClaw key and tie-break adaptations, as
described in S4.

### F2. Plain `unicode61` was a correction, not uncited invention

The final choice is explicitly tied to ccrider's plain `unicode61` table and
was validated against the two transcript examples in message 296. The CASS
snippet was rejected because its domain constraints did not transfer.

### F3. Intermediate batch backfill was not a surviving contamination

The temporary watermark/batch implementation was removed by `3309c1c` in favor
of the native FTS5 rebuild. The surviving `INSERT INTO ... VALUES('rebuild')`
has ccrider/agentsview references in the notes; the stronger directly-read
source is ccrider `schema.go:122`.

### F4. Full test and formatting claims in the mail thread are evidence, not
lineage

Message 309 reports `CGO_ENABLED=0 go test -race -count=1 ./...` as 32 passing
and gofmt clean on the merge candidate. That supports the merge gate but does
not turn any heuristic into prior art. This audit did not modify Go files.

## Required cleanup before claiming a lineage-clean design

1. Correct the stale “custom tokenchars” comments in `store.go` and `fts.go`.
2. Add exact citations or explicitly mark as RawClaw-owned policy for S3's
   stopword ordering, S4's identity/tie-break rules and AND/OR orchestration,
   S5's same-table OR retry, S6's SQLite tuning, S7's freshness policy, and
   S8's overfetch/coverage constants.
3. Remove or quarantine the unused ccrider `EscapeFTS5Query` implementation,
   or document why both sanitizers must exist.
4. Keep the rejected CASS tokenchars/router and exact-first experiments clearly
   marked as historical failures; do not reuse their source snippets without
   the transcript-domain tests that caught the defect.
