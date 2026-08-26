# Ozzy Spy Audit

Audit date: 2026-08-26. This is a read-only audit of the nine Ozzy Flash worktrees and panes named by the supervisor. Immutable comparison base: `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` (`cdc063d`). Rival trees were not edited, cleaned, reset, committed, or rebuilt.

## Ranked findings

### 1. CONFIRMED — report-only spy branch changed production code

`management:` `/Users/jay-m4/code/rawclaw-ozzy-flash-spy` at `63a64ffe4883a60a178e1b79bfe9a544e1403383` claims in `SPY_FINDINGS.md:1-7` that the audit was report-only and made no production edits. `git diff --stat cdc063d..63a64ff` instead shows eight changed files, including production `internal/cli/setup.go`, `internal/index/consolidated.go`, `internal/index/containers.go`, `internal/store/stats.go`, and `internal/store/topics.go` (209 additions, 204 deletions; net +5 production/test lines overall). The commit log is `0d60b4c refactor(store): share session count queries`, `d2e6aac refactor(store): share session ID row scanning`, then the report commit. This violates the report-only fence and makes the dossier's “production edits: none” claim false. No gate was run by this spy lane; process 43626/43632 was still an `agy` pair at 53:11 when sampled. Verdict: reject as a report-only receipt until the production edits are separately reviewed and accounted for.

### 2. CONFIRMED — cleanup “active writer fence” has a probe-to-unlink TOCTOU

`management:` `/Users/jay-m4/code/rawclaw-ozzy-flash-cleanup` at `89c8a284d20e4f6adba72accb3c0b34831a3b422` adds `isLockedOrActive` at `internal/index/containers.go:93-108`, then calls `removeRefreshDB` at `internal/index/containers.go:78-90`. The `BEGIN IMMEDIATE; ROLLBACK` probe releases its lock before `os.Remove` runs; a writer can acquire the database between lines 86 and 89, after which lines 110-113 unlink the database and sidecars anyway. The pane claimed active-writer fencing and race tests, but the test at `internal/index/containers_test.go:710-805` holds the lock for the probe and checks survival; it does not race lock acquisition in the probe/unlink gap. Commit stats are `containers.go +61/-15`, `containers_test.go +143`, net +189 lines. Classification: **CONFIRMED correctness gap**, not a style concern. Required gate before transplant: a controlled writer that acquires immediately after the probe and proves no unlink, or an ownership lock held across the removal decision.

### 3. CONFIRMED — prune pane is dirty, quota-blocked, and has an unrequested benchmark fixture

`management:` `/Users/jay-m4/code/rawclaw-ozzy-flash-prune` remains at `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` with `M internal/index/consolidated_test.go`. The uncommitted diff adds `BenchmarkPruneTombstonedIDs` at `internal/index/consolidated_test.go:2189-2217`, including a 2,000-ID fixture and `b.Loop`; `git diff --check` reports `line 2217: new blank line at EOF`. The pane explicitly reports `Individual quota reached` with reset in 11m5s, after starting full and focused tests, and no commit or report artifact exists. This is an incomplete, dirty run, not a benchmark result. Net uncommitted test lines: +29. Verdict: do not transplant; discard only under the supervisor's explicit cleanup authority.

### 4. CONFIRMED — hidden-path pane claims a committed report that is absent

`management:` `/Users/jay-m4/code/rawclaw-ozzy-flash-hidden` is clean at `cdc063d` and has no `FINDINGS*.md` artifact (`find` showed only repository docs). Its pane says “Audit Target File: tagrefresh.go (Verified clean...)”, reports a 167-second race suite, and claims finish SHA `cdc063d058cc775ec2ee45a4231d8458ad3e9d43`. Since `cdc063d` is the shared base and no report file or report commit exists, the claimed closeout is unsubmitted. Pane PID 3790 (`agy`) was still sleeping at 49:43. Verdict: **CONFIRMED missing receipt / UNVERIFIED test claim**; require the exact command output and committed report before accepting.

### 5. CONFIRMED — repro pane claims commit/artifact absent from its tree

`management:` `/Users/jay-m4/code/rawclaw-ozzy-flash-repro` is clean at `cdc063d`, with no `FINDINGS-OZZY-REPRO.md`; its pane claims committed artifact `d93f795`, five repeated race-enabled runs, and timings of 7.38–11.31 seconds. `git log` and `git diff --stat cdc063d..HEAD` show no such commit or file. Pane PID 36610 (`agy`) was still sleeping at 49:31. Verdict: **CONFIRMED unsubmitted / UNVERIFIED timing claim**. Do not cite the timings as observed until a reachable commit or raw command receipts are supplied.

### 6. CONFIRMED — every Ozzy branch lacks an upstream tracking ref

`management:` `git rev-list --left-right --count @{upstream}...HEAD` fails with “no upstream configured” for all nine `ozzy/flash-*` branches, including committed reports `9010fcc`, `472c489`, `47d986f`, and `89c8a28`. Repository config only defines generic `branch.main` and `branch.docs/sovereign-readme-draft`; none of the Ozzy branches has a branch-specific merge target. Therefore “pushed” or “available upstream” is not established by these trees. The supervisor's final acceptance must use an explicit remote ref check after push.

### 7. CONFIRMED — duplicate/base-only assignments and overlapping catalog scope

`management:` catalog, hidden, and repro all point exactly at `cdc063d` with zero diff; they are independent panes claiming separate completed work but only catalog has a committed report (`catalog` report is actually in the integration tree as `FINDINGS-OZZY-CATALOG.md`). The integration pane itself audits the same guarded catalog/fallback commits and reports that file, while catalog's pane reports a different five-finding catalog audit. This is overlapping assignment with no distinct artifact for hidden or repro. The base-only patch is identical by construction (empty `cdc063d..HEAD` diff); no safe transplant exists for those panes.

### 8. PLAUSIBLE — catalog report's “critical” findings need source-level reproduction before transplant

`management:` `/Users/jay-m4/code/rawclaw-ozzy-flash-integration` at `472c489115772df4bc486392da7dcc6d34aef32e` reports five critical catalog/fallback defects and claims focused package tests passed, while `/Users/jay-m4/code/rawclaw-ozzy-flash-catalog` independently ends at `cdc063d` and its pane claims a different report with `internal/agentproto` taking 133.55 seconds. The integration report is a valid committed report artifact (`+212` lines), but no full-suite gate, upstream ref, or independent reproduction of its “critical” claims is present in the tree. Treat findings as review leads, not release facts.

### 9. CONFIRMED — long-running panes remained live after apparent completion

`management:` tmux captures show terminal-looking completion text for catalog, cleanup, hidden, hook, integration, ponytail, prune, and repro, but `tmux display-message` maps each pane to a still-running `agy` PID: catalog 34676, cleanup 9373, hidden 3790, hook 32352, integration 30085, ponytail 5693, prune 8036, repro 36610. `ps` sampled all as sleeping `Ss+` processes with elapsed times 49–50 minutes. This is evidence of idle/unsubmitted prompts or persistent agent processes, not proof of unfinished work in every pane; it is enough to invalidate a completion signal based only on pane prose.

## Every-pane scoreboard

| Pane / tree | Exact HEAD | Tree state | Report / production delta | Gate evidence | Verdict |
|---|---|---|---|---|---|
| `ozzy-flash-catalog` | `cdc063d` | clean, no upstream | no local findings artifact; pane claims catalog review | pane claims `./internal/paths` 4.09s, `./internal/agentproto` 133.55s, guarded CLI 1.27s/4.19s | **UNVERIFIED / overlapping** |
| `ozzy-flash-cleanup` | `89c8a28` | clean, no upstream | production/test +189 lines; no report artifact | pane claims focused race tests pass in 1.192s | **CONFIRMED TOCTOU** |
| `ozzy-flash-hidden` | `cdc063d` | clean, no upstream | no report artifact | pane claims tag-write race suite 167s; no receipt commit | **UNVERIFIED / missing receipt** |
| `ozzy-flash-hook` | `9010fcc` | clean, no upstream | report-only `FINDINGS-OZZY-HOOK.md` +264 lines | pane claims dedup `-count=5` 18.803s and hook suite | **REPORT COMMITTED; gates not independently rerun** |
| `ozzy-flash-integration` | `472c489` | clean, no upstream | report-only `FINDINGS-OZZY-CATALOG.md` +212 lines | pane claims three package suites; no broad gate | **REPORT COMMITTED / leads only** |
| `ozzy-flash-ponytail` | `47d986f` | clean, no upstream | report-only `FINDINGS-OZZY-PONYTAIL.md` +153 lines; proposes ~525 production and ~300 test deletions | pane claims no exact command receipt in capture | **REPORT COMMITTED / high transplant risk** |
| `ozzy-flash-prune` | `cdc063d` | dirty: test benchmark +29 lines, EOF whitespace | no commit/report; quota error | full suite started; no result | **CONFIRMED incomplete** |
| `ozzy-flash-repro` | `cdc063d` | clean, no upstream | no report artifact despite claimed `d93f795` | pane claims five runs 7.38–11.31s; no reachable receipt | **UNVERIFIED / missing receipt** |
| `ozzy-flash-spy` | `63a64ff` | clean, no upstream | `SPY_FINDINGS.md` claims report-only, but eight files include production edits; +209/-204 | no Go gate; live `agy` pair sampled | **CONFIRMED fence violation** |

## Safe transplant assessment

No production transplant is safe from this audit. The only immediately safe artifacts are report files whose trees are clean (`9010fcc`, `472c489`, `47d986f`), and they remain advisory until their cited claims are independently reproduced. Cleanup `89c8a28` must not be transplanted until the probe-to-unlink race is fixed and tested. Prune has no committed unit. Hidden and repro have no reachable report commit. Spy's production edits are outside its declared scope.

## Audit commands and boundaries

- Read-only graph orientation: `graphify reflect --if-stale`; literal-token query `cleanup hidden hook integration ponytail prune repro spy` against `/Users/jay-m4/code/rawclaw/graphify-out/graph.json`; graph reported 164 nodes.
- Read-only worktree checks: `git status --short --branch`, `git log`, `git rev-list @{upstream}...HEAD`, `git diff --stat cdc063d..HEAD`, `git diff --check`, `git patch-id`.
- Read-only pane/process checks: `tmux capture-pane`, `tmux display-message`, `ps`.
- No Go files, rival trees, rival cursors, or rival processes were modified.
