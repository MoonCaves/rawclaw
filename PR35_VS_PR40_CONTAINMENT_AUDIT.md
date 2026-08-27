# PR #35 vs PR #40 Containment Audit

**Audit Context:** Evaluating claim that PR #40 (bc16820) exactly contains PR #35 (a33ab02), necessitating rejection of PR #35 as redundant.

## Verified Findings

### 1. Ancestry Analysis
- Both PRs diverged from common base 0d1da19 (verified via `git merge-base --is-ancestor`)
- Neither a33ab02 nor bc16820 is an ancestor of the other
- PR #40 built on newer main (c818ea1), PR #35 on older main (86c5ce0)
- **Verdict:** Different lineages, not pure containment.

### 2. File Identity Coverage
- **17 of 25 files identical** (byte-identical across both tips)
  - internal/agentproto/agentproto.go, internal/cli/antigravityhook_test.go, catalog_hook_test.go, cmd_ingest_test.go, cmd_prewarm.go, cmd_setup_test.go, codexhook_test.go, setup.go, setup_test.go, internal/durable/durable.go, internal/index/consolidated_fence.go, consolidated_fence_test.go, containers.go, containers_test.go, internal/store/connect_bench_test.go, stats.go, topics.go
- **8 files differ** with net +535/-79 changes:
  - docs/design/tombstone-consolidation-contract.md, internal/cli/cli.go, cmd_tag.go, cmd_tag_onestore_test.go, tagrefresh.go, tagrefresh_test.go, internal/index/consolidated.go, consolidated_test.go

### 3. Behavioral Substitution (Critical)
PR #40 **replaces** PR #35's synchronous fold semantics with asynchronous publish semantics:

**In cmd_tag.go (runTagWriteCmd):**
- PR #35: `index.SyncConsolidatedFrom(dbp)` — inline, synchronous fold
- PR #40: `spawnTagPublish(dbp, fullSID)` — detached, async spawn

**In tagrefresh.go (runTagPrepCmdWithSources loop):**
- PR #35: `index.SyncConsolidatedFrom(refreshDB)` — synchronous per-refresh-DB
- PR #40: `maybeSpawnIngest(fullSID)` — async spawn

**In tagrefresh.go (topic resolution):**
- PR #35: `readConsolidatedTopics(...)` — consolidated store query
- PR #40: `readAuthoritativeTagTopics(...)` — per-session DB direct query

**Verdict:** This is genuine behavioral substitution, not pure addition. PR #40 does not merely add new code; it replaces PR #35's design decision (sync-inline-fold → async-detached-publish).

### 4. Missing Commit d918706
- d918706 ("fix: unblock nil-scope consolidated tag-write") **not present** in PR #40 by SHA ancestry
- Checked exhaustively against PR #40's 47 unique commits via `git rev-list`
- No patch-id match or payload equivalence
- Consistent with stated rejection ("lost-write safety")

### 5. PR #40 Author Intent
PR #40's own PR body explicitly states: **"This supersedes PR #35 in scope; PR #35 is intentionally left open."**
- Author does NOT claim PR #35 should be closed/merged-away
- Contradicts rival claim that "must not merge separately"

### 6. PR #40 CI Status
- Build (1.24.0) / build(stable) / lint: all **SUCCESS**
- mergeStateStatus: **CLEAN**, mergeable: **MERGEABLE** (via `gh pr checks 40`)

## Gate Receipt

**Audit Worktree:** /Users/jay-m4/code/rawclaw-wt-pr35-vs-pr40-audit at bc16820

**gofmt Check:**
```
gofmt -l internal/cli/cmd_tag.go internal/cli/tagrefresh.go internal/index/consolidated.go
# Result: (no output) — all files clean
```

**Focused Test Suite:**
```
CGO_ENABLED=0 go test -race -run 'TestTagWrite|TestTagPrep|TestConsolidate' -count=1 ./internal/cli/... ./internal/index/...
# Result: 
#   ok  	github.com/MoonCaves/rawclaw/internal/cli	10.031s
#   ok  	github.com/MoonCaves/rawclaw/internal/index	28.652s
# All tag-write, tag-prep, and consolidate test suites PASS
```

**Graphify Orientation:**
- Updated graphify-out/ (3505 nodes, 10589 edges, 162 communities)
- Confirmed presence of cmd_tag.go, tagrefresh.go, consolidated.go in graph
- Key nodes: runTagWriteCmd, runTagPrepCmdWithSources, locateTagWriteFast, spawnTagPublishChild, readAuthoritativeTagTopics

## Verdict

**CONTAINMENT VERDICT: REJECT**

PR #40 (bc16820) does **not** exactly contain PR #35 (a33ab02). 

- 17/25 files are identical ✓
- But 8 differing files include a **genuine behavioral substitution** (synchronous inline consolidated fold in PR #35 replaced by asynchronous detached publish in PR #40), not a pure superset.
- d918706 is confirmed absent from PR #40 by both ancestry and patch-id.
- PR #40's own author explicitly states PR #35 is intentionally left open, contradicting the "must not merge separately" framing.

**Recommendation:** PR #35 and PR #40 represent different design choices (sync-fold vs. async-publish) on overlapping concerns. Both merit independent review; closure of #35 based on claimed exact containment is not warranted by the evidence.
