# Conor PR35 Wave 3 hostile audit

Scope: `/Users/jay-m4/code/rawclaw-luna-conor-pr35`, `-resolve`, and
`-containers`, compared with integration baseline `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.
The measured integration context supplied by the supervisor is
`bd8346c` (clean, full race gate PASS); this audit did not alter any rival
worktree. Audited heads are immutable `c88bc4664c4050082abfa635ee8b7600107b2e1f`,
`4b32d95e04fc8fc093d9ad1a1445e88a5a780727`, and
`54bf2b03d3b32bf639924ff0a1f8f6885772eb81`.

## Verdict: HOLD

The current code fixes exercised by the three workers pass their focused
tests, but the committed audit artifacts are not truthful descriptions of
their own current heads. Two worker diffs also reproduce earlier patch IDs.
The report artifacts require correction before ACCEPT.

## Findings

### F1 — HOLD: hooks report is stale after its own fix

`FINDINGS-PR35-HOOKS.md:26-29` says the detached ingest is launched before the
dedup check and claims the second invocation still receives an ingest. At the
audited head, `internal/cli/setup.go:64-84` and `:150-170` check
`[ -f "$entry" ]`, write the marker, and only then launch `nohup ... ingest`.
The worker's own regression `internal/cli/cmd_ingest_test.go:136-201` passes.
This is an unsupported current-head claim, not an unresolved production bug.

Delta for the worker range: production `+2/-2` (net 0), tests `+73`, report
`+53`; `net: -53 lines possible` by deleting/replacing the stale report.
Ruling: HOLD report until rewritten against `c88bc466...`; code ACCEPT narrowed.

### F2 — HOLD: resolution report describes pre-fix behavior and duplicates prior art

`FINDINGS-PR35-RESOLUTION.md:39-42` describes the unsafe directory fallback as
current behavior, while audited `internal/agentproto/agentproto.go:1810-1817`
now skips `ProjectDirOf` misses. The two follow-up tests pass, so the report
must distinguish historical reproduction from current-head status.

Patch-id evidence: Conor `8dfa1ca95cc4fb719ee07b2135fc10814740230d` has stable
patch-id `4b310ec5516b651c43cfecef4dca4124d061b8bf`, identical to prior-art
`54afa70` (same source fix). Delta: production `+4/-2` (net +2), tests `+98`,
report `+74`; `net: -74 lines possible` by correcting/removing stale report
material. Ruling: HOLD artifact; code ACCEPT only as duplicate-attribution
reviewed work.

### F3 — HOLD: containers report is factually impossible at audited HEAD and
its deletion patch is duplicated

`FINDINGS-PR35-CONTAINERS.md:10-14` names nonexistent `85cf480` as the kept
change. `:23-25`, `:36-55`, and `:75-83` cite `pruneStaleRefreshDBs` and line
locations that no longer exist: audited `internal/index/containers.go:1-105`
contains no sweeper, and `internal/index/containers_test.go` no longer has
`TestEnsureFreshContainer_PruneStaleLeftovers`. The report's command at
`:89` still passes only because the missing test name is silently omitted by
`-run`; it does not test the claimed prune behavior. The report's `:90`
"Net code change: 0" is false: the audited commit removes 42 production and
119 test lines (net `-161`).

Patch-id evidence: Conor `54bf2b03d3b32bf639924ff0a1f8f6885772eb81` has stable
patch-id `d7c22ba9b5bf9b41eb8b473bd1e48227e4fe3a28`, identical to prior-art
`25a43ea` and `21ece6f`. Delta: production `-42`, tests `-119`, report `+92`;
`net: -92 lines possible` by replacing the stale artifact with a short,
current-head receipt. Ruling: HOLD report; deletion itself ACCEPT pending
independent integration review.

## Observed gates

- `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestPrimeScripts_SessionStartDeduplicatesDetachedIngest$'` — PASS, 2.563s.
- `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestGuardedSessionLookup(DoesNotTreatForeignCatalogPathAsClaude|UsesForeignPreResolvedScope)$'` — PASS, 1.994s.
- `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'TestPrepareFreshContainer_ProvesFreshnessWithoutConsolidatedSync$'` — PASS, 1.514s.
- `CGO_ENABLED=0 go test -race -count=3 ./internal/cli/... ./internal/index/...` — PASS in each rival tree; cli/index package timings were 283.992s/311.289s (hooks), 282.081s/306.800s (resolve), and 282.229s/310.748s (containers).
- Exact containers report gate with `-count=5 -shuffle=on` — PASS, 2.892s, but the deleted prune test matched zero cases and therefore does not validate F3's claim.

No Go files were touched. Rival trees remained untouched.

## Net accounting

Audited production deltas are 0, +2, and -42; test deltas are +73, +98, and
-119. Report artifacts add 53, 74, and 92 lines while containing stale claims.
Overall report cleanup opportunity: `net: -219 lines possible` (remove stale
report ballast; no production code deletion is requested by this audit).
